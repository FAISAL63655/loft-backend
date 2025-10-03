package checkout

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"encore.dev/beta/auth"
	"encore.dev/storage/sqldb"

	"encore.app/pkg/config"
	"encore.app/pkg/errs"
)

var db = sqldb.Named("coredb")

//encore:service
type Service struct{}

func initService() (*Service, error) { return &Service{}, nil }

type CheckoutHeaders struct {
	IdemKey string `header:"Idempotency-Key"`
}

type CheckoutResponse struct {
	InvoiceID int64  `json:"invoice_id"`
	Status    string `json:"status"`
}

//encore:api auth method=POST path=/checkout
func Checkout(ctx context.Context, h *CheckoutHeaders) (*CheckoutResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	// Parse user id early and use int64 for queries
	userID, perr := strconv.ParseInt(string(uidStr), 10, 64)
	if perr != nil {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف مستخدم غير صالح"}
	}
	// Require email verified
	var verified bool
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT email_verified_at IS NOT NULL FROM users WHERE id=$1`, userID).Scan(&verified); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل التحقق"}
	}
	if !verified {
		return nil, &errs.Error{Code: "AUTH_EMAIL_VERIFY_REQUIRED_AT_CHECKOUT", Message: "فعّل حسابك لإتمام الشراء"}
	}

	key := strings.TrimSpace(h.IdemKey)
	if key == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "مطلوب Idempotency-Key"}
	}

	// Idempotency lock
	hash := int64(hashKey(key))
	if _, err := db.Stdlib().ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, hash); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل القفل"}
	}

	// Return existing unpaid session by idem key
	var existingInvoiceID sql.NullInt64
	_ = db.Stdlib().QueryRowContext(ctx, `SELECT id FROM invoices WHERE totals->>'idem_key' = $1 AND status='unpaid'`, key).Scan(&existingInvoiceID)
	if existingInvoiceID.Valid {
		return &CheckoutResponse{InvoiceID: existingInvoiceID.Int64, Status: "unpaid"}, nil
	}

	// Settings snapshot
	s := config.GetSettings()
	vatRate := 0.0
	if s != nil && s.VATEnabled {
		vatRate = s.VATRate
	}

	// Defensive: ensure DB connection is healthy before starting a transaction
	if err := db.Stdlib().PingContext(ctx); err != nil {
		return nil, &errs.Error{Code: errs.ServiceUnavailable, Message: "تعذر الاتصال بقاعدة البيانات، حاول لاحقاً"}
	}

	// Begin transaction
	tx, err := db.Stdlib().BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء معاملة"}
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Create order
	var orderID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO orders (user_id, source) VALUES ($1,'direct') RETURNING id`, userID).Scan(&orderID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء الطلب"}
	}

	// Build items from cart_items
	// Validate availability first
	var badPigeon sql.NullInt64
	_ = tx.QueryRowContext(ctx, `
		SELECT p.id
		FROM cart_items ci
		JOIN products p ON p.id=ci.product_id
		WHERE ci.user_id=$1 AND p.type='pigeon' AND p.status!='available'
		LIMIT 1
	`, userID).Scan(&badPigeon)
	if badPigeon.Valid {
		return nil, &errs.Error{Code: errs.Conflict, Message: "العنصر لم يعد متاح"}
	}
	var badSupply sql.NullInt64
	_ = tx.QueryRowContext(ctx, `
		SELECT p.id
		FROM cart_items ci
		JOIN products p ON p.id=ci.product_id
		JOIN supplies s ON s.product_id=p.id
		WHERE ci.user_id=$1 AND p.type='supply' AND (ci.qty <= 0 OR s.stock_qty < ci.qty)
		LIMIT 1
	`, userID).Scan(&badSupply)
	if badSupply.Valid {
		return nil, &errs.Error{Code: errs.Conflict, Message: "العنصر لم يعد متاح"}
	}

	// Insert pigeons (bulk)
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO order_items (order_id, product_id, qty, unit_price_gross, line_total_gross)
		SELECT $1 AS order_id, p.id AS product_id, 1 AS qty,
			   ROUND(p.price_net * (1 + $2::numeric), 2) AS unit_price_gross,
			   ROUND(p.price_net * (1 + $2::numeric), 2) AS line_total_gross
		FROM cart_items ci
		JOIN products p ON p.id=ci.product_id
		WHERE ci.user_id=$3 AND p.type='pigeon'
	`, orderID, vatRate, userID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إدراج عناصر الحمام: " + err.Error()}
	}

	// Insert supplies (bulk)
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO order_items (order_id, product_id, qty, unit_price_gross, line_total_gross)
		SELECT $1 AS order_id, p.id AS product_id, ci.qty AS qty,
			   ROUND(p.price_net * (1 + $2::numeric), 2) AS unit_price_gross,
			   ROUND(p.price_net * (1 + $2::numeric), 2) * ci.qty AS line_total_gross
		FROM cart_items ci
		JOIN products p ON p.id=ci.product_id
		WHERE ci.user_id=$3 AND p.type='supply' AND ci.qty > 0
	`, orderID, vatRate, userID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إدراج عناصر المستلزمات: " + err.Error()}
	}

	// Recalculate totals via trigger by touching row
	if _, err = tx.ExecContext(ctx, `UPDATE orders SET updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, orderID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل احتساب الإجماليات"}
	}

	// Create invoice
	year := time.Now().UTC().Year()
	var invoiceNumber string
	if err = tx.QueryRowContext(ctx, `SELECT next_invoice_number($1)`, year).Scan(&invoiceNumber); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل ترقيم الفاتورة: " + err.Error()}
	}
	var invoiceID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO invoices (order_id, number, vat_rate_snapshot, totals) VALUES ($1,$2,$3, jsonb_build_object('idem_key', $4::text)) RETURNING id`, orderID, invoiceNumber, vatRate, key).Scan(&invoiceID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء الفاتورة: " + err.Error()}
	}

	if err = tx.Commit(); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل الحفظ"}
	}

	return &CheckoutResponse{InvoiceID: invoiceID, Status: "unpaid"}, nil
}

func hashKey(s string) uint64 {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
