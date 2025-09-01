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
	"encore.app/pkg/moneysar"
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

	// Guard: ORD_PIGEON_ALREADY_PENDING only for same pigeon currently reserved by user
	var conflict bool
	_ = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
		  SELECT 1
		  FROM order_items oi
		  JOIN orders o ON o.id = oi.order_id
		  WHERE o.user_id = $1
		    AND o.status = 'pending_payment'
		    AND oi.product_id IN (
		      SELECT id FROM products
		      WHERE type = 'pigeon'
		        AND reserved_by = $1
		        AND status IN ('reserved','payment_in_progress')
		    )
		)
	`, userID).Scan(&conflict)
	if conflict {
		return nil, &errs.Error{Code: "ORD_PIGEON_ALREADY_PENDING", Message: "لديك طلب حمامة قيد الدفع"}
	}

	// Create order
	var orderID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO orders (user_id, source) VALUES ($1,'direct') RETURNING id`, userID).Scan(&orderID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء الطلب"}
	}

	// Build items from current holds
	// Pigeons
	pRows, err := tx.QueryContext(ctx, `
		SELECT p.id, p.price_net
		FROM products p
		WHERE p.type='pigeon' AND p.status='reserved' AND p.reserved_by=$1 AND p.reserved_expires_at > (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
	`, userID)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة عناصر الحمام"}
	}
	for pRows.Next() {
		var pid int64
		var priceNet float64
		if err = pRows.Scan(&pid, &priceNet); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "خطأ قراءة الحمام"}
		}
		unitGross := moneysar.GrossFromNet(priceNet, vatRate)
		if _, err = tx.ExecContext(ctx, `INSERT INTO order_items (order_id, product_id, qty, unit_price_gross, line_total_gross) VALUES ($1,$2,1,$3,$3)`, orderID, pid, unitGross); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل إدراج عنصر الحمام"}
		}
	}
	_ = pRows.Close()

	// Supplies
	sRows, err := tx.QueryContext(ctx, `
		SELECT sr.product_id, p.price_net, sr.qty
		FROM stock_reservations sr
		JOIN products p ON p.id=sr.product_id
		WHERE sr.user_id=$1 AND (sr.expires_at > (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') OR sr.invoice_id IS NOT NULL)
	`, userID)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة المستلزمات"}
	}
	for sRows.Next() {
		var pid int64
		var priceNet float64
		var qty int
		if err = sRows.Scan(&pid, &priceNet, &qty); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "خطأ قراءة المستلزمات"}
		}
		unitGross := moneysar.GrossFromNet(priceNet, vatRate)
		lineGross := unitGross * float64(qty)
		if _, err = tx.ExecContext(ctx, `INSERT INTO order_items (order_id, product_id, qty, unit_price_gross, line_total_gross) VALUES ($1,$2,$3,$4,$5)`, orderID, pid, qty, unitGross, lineGross); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل إدراج عنصر مستلزم"}
		}
	}
	_ = sRows.Close()

	// Recalculate totals via trigger by touching row
	if _, err = tx.ExecContext(ctx, `UPDATE orders SET updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, orderID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل احتساب الإجماليات"}
	}

	// Create invoice
	year := time.Now().UTC().Year()
	var invoiceNumber string
	if err = tx.QueryRowContext(ctx, `SELECT next_invoice_number($1)`, year).Scan(&invoiceNumber); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل ترقيم الفاتورة"}
	}
	var invoiceID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO invoices (order_id, number, vat_rate_snapshot, totals) VALUES ($1,$2,$3, jsonb_build_object('idem_key',$4)) RETURNING id`, orderID, invoiceNumber, vatRate, key).Scan(&invoiceID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء الفاتورة"}
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
