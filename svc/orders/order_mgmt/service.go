package order_mgmt

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

type Paginate struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`
}

type OrderSummary struct {
	ID           int64   `json:"id"`
	Status       string  `json:"status"`
	GrandTotal   float64 `json:"grand_total"`
	CreatedAtISO string  `json:"created_at"`
}

type OrdersResponse struct {
	Items []OrderSummary `json:"items"`
}

//encore:api auth method=GET path=/orders
func ListMyOrders(ctx context.Context, q *Paginate) (*OrdersResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	uid, _ := strconv.ParseInt(string(uidStr), 10, 64)
	page, limit := q.Page, q.Limit
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit
	rows, err := db.Stdlib().QueryContext(ctx, `
		SELECT id, status::text, grand_total, to_char(created_at at time zone 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM orders WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, uid, limit, offset)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل الاستعلام"}
	}
	defer rows.Close()
	var items []OrderSummary
	for rows.Next() {
		var it OrderSummary
		if err := rows.Scan(&it.ID, &it.Status, &it.GrandTotal, &it.CreatedAtISO); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل القراءة"}
		}
		items = append(items, it)
	}
	return &OrdersResponse{Items: items}, nil
}

type OrderDetail struct {
	ID              int64            `json:"id"`
	Status          string           `json:"status"`
	Items           []OrderItem      `json:"items"`
	Totals          OrderTotals      `json:"totals"`
	UserID          int64            `json:"user_id,omitempty"`
	UserName        string           `json:"user_name,omitempty"`
	UserEmail       string           `json:"user_email,omitempty"`
	UserPhone       string           `json:"user_phone,omitempty"`
	ShippingAddress *ShippingAddress `json:"shipping_address,omitempty"`
}

type ShippingAddress struct {
	CityName string `json:"city_name,omitempty"`
	Label    string `json:"label,omitempty"`
	Line1    string `json:"line1,omitempty"`
	Line2    string `json:"line2,omitempty"`
}

type OrderItem struct {
	ProductID int64   `json:"product_id"`
	Qty       int     `json:"qty"`
	UnitGross float64 `json:"unit_price_gross"`
	LineGross float64 `json:"line_total_gross"`
}

type OrderTotals struct {
	SubtotalGross float64 `json:"subtotal_gross"`
	VATAmount     float64 `json:"vat_amount"`
	ShippingGross float64 `json:"shipping_fee_gross"`
	GrandTotal    float64 `json:"grand_total"`
}

//encore:api auth method=GET path=/orders/:id
func GetOrder(ctx context.Context, id string) (*OrderDetail, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	uid, _ := strconv.ParseInt(string(uidStr), 10, 64)
	var oid int64
	if v, err := strconv.ParseInt(id, 10, 64); err == nil {
		oid = v
	} else {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف غير صالح"}
	}
	// Ownership or admin
	var ownerID int64
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT user_id FROM orders WHERE id=$1`, oid).Scan(&ownerID); err != nil {
		return nil, &errs.Error{Code: "ORD_NOT_FOUND", Message: "الطلب غير موجود"}
	}
	if ownerID != uid {
		var role string
		_ = db.Stdlib().QueryRowContext(ctx, `SELECT role::text FROM users WHERE id=$1`, uid).Scan(&role)
		if strings.ToLower(role) != "admin" {
			return nil, &errs.Error{Code: errs.Forbidden, Message: "غير مصرح"}
		}
	}
	var det OrderDetail
	det.ID = oid
	
	// Fetch order with user and address information
	var addressID sql.NullInt64
	var userName, userEmail, userPhone sql.NullString
	var cityName, addressLabel, addressLine1, addressLine2 sql.NullString
	
	err := db.Stdlib().QueryRowContext(ctx, `
		SELECT 
			o.status::text, 
			o.subtotal_gross, 
			o.vat_amount, 
			o.shipping_fee_gross, 
			o.grand_total,
			o.user_id,
			u.name,
			u.email,
			u.phone,
			o.address_id,
			c.name_ar,
			a.label,
			a.line1,
			a.line2
		FROM orders o
		JOIN users u ON u.id = o.user_id
		LEFT JOIN addresses a ON a.id = o.address_id
		LEFT JOIN cities c ON c.id = a.city_id
		WHERE o.id = $1
	`, oid).Scan(
		&det.Status, 
		&det.Totals.SubtotalGross, 
		&det.Totals.VATAmount, 
		&det.Totals.ShippingGross, 
		&det.Totals.GrandTotal,
		&det.UserID,
		&userName,
		&userEmail,
		&userPhone,
		&addressID,
		&cityName,
		&addressLabel,
		&addressLine1,
		&addressLine2,
	)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة الطلب"}
	}
	
	// Set user info
	if userName.Valid {
		det.UserName = userName.String
	}
	if userEmail.Valid {
		det.UserEmail = userEmail.String
	}
	if userPhone.Valid {
		det.UserPhone = userPhone.String
	}
	
	// Set address info if exists
	if addressID.Valid && addressID.Int64 > 0 {
		det.ShippingAddress = &ShippingAddress{}
		if cityName.Valid {
			det.ShippingAddress.CityName = cityName.String
		}
		if addressLabel.Valid {
			det.ShippingAddress.Label = addressLabel.String
		}
		if addressLine1.Valid {
			det.ShippingAddress.Line1 = addressLine1.String
		}
		if addressLine2.Valid {
			det.ShippingAddress.Line2 = addressLine2.String
		}
	} else {
		// Fallback: Get user's default address if order has no address_id
		var defCityName, defLabel, defLine1, defLine2 sql.NullString
		err := db.Stdlib().QueryRowContext(ctx, `
			SELECT c.name_ar, a.label, a.line1, a.line2
			FROM addresses a
			JOIN cities c ON c.id = a.city_id
			WHERE a.user_id = $1 AND a.is_default = true AND a.archived_at IS NULL
			LIMIT 1
		`, det.UserID).Scan(&defCityName, &defLabel, &defLine1, &defLine2)
		
		if err == nil {
			det.ShippingAddress = &ShippingAddress{}
			if defCityName.Valid {
				det.ShippingAddress.CityName = defCityName.String
			}
			if defLabel.Valid {
				det.ShippingAddress.Label = defLabel.String
			}
			if defLine1.Valid {
				det.ShippingAddress.Line1 = defLine1.String
			}
			if defLine2.Valid {
				det.ShippingAddress.Line2 = defLine2.String
			}
		}
	}
	rows, err := db.Stdlib().QueryContext(ctx, `SELECT product_id, qty, unit_price_gross, line_total_gross FROM order_items WHERE order_id=$1`, oid)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة العناصر"}
	}
	defer rows.Close()
	for rows.Next() {
		var it OrderItem
		if err := rows.Scan(&it.ProductID, &it.Qty, &it.UnitGross, &it.LineGross); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة عنصر"}
		}
		det.Items = append(det.Items, it)
	}
	return &det, nil
}

type InvoiceSummary struct {
	ID     int64  `json:"id"`
	Number string `json:"number"`
	Status string `json:"status"`
}

type InvoicesResponse struct {
	Items []InvoiceSummary `json:"items"`
}

//encore:api auth method=GET path=/orders/:id/invoice
func GetOrderInvoice(ctx context.Context, id string) (*InvoiceSummary, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	uid, _ := strconv.ParseInt(string(uidStr), 10, 64)
	var oid int64
	if v, err := strconv.ParseInt(id, 10, 64); err == nil {
		oid = v
	} else {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف غير صالح"}
	}
	// Ownership or admin
	var ownerID int64
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT user_id FROM orders WHERE id=$1`, oid).Scan(&ownerID); err != nil {
		return nil, &errs.Error{Code: "ORD_NOT_FOUND", Message: "الطلب غير موجود"}
	}
	if ownerID != uid {
		var role string
		_ = db.Stdlib().QueryRowContext(ctx, `SELECT role::text FROM users WHERE id=$1`, uid).Scan(&role)
		if strings.ToLower(role) != "admin" {
			return nil, &errs.Error{Code: errs.Forbidden, Message: "غير مصرح"}
		}
	}
	var res InvoiceSummary
	// Each order has a single invoice (unique)
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT id, number, status::text FROM invoices WHERE order_id=$1`, oid).Scan(&res.ID, &res.Number, &res.Status); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة الفاتورة"}
	}
	return &res, nil
}

//encore:api auth method=GET path=/invoices
func ListMyInvoices(ctx context.Context, q *Paginate) (*InvoicesResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	uid, _ := strconv.ParseInt(string(uidStr), 10, 64)
	page, limit := q.Page, q.Limit
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit
	rows, err := db.Stdlib().QueryContext(ctx, `
		SELECT i.id, i.number, i.status::text
		FROM invoices i JOIN orders o ON o.id=i.order_id
		WHERE o.user_id=$1 ORDER BY i.created_at DESC LIMIT $2 OFFSET $3`, uid, limit, offset)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل الاستعلام"}
	}
	defer rows.Close()
	var items []InvoiceSummary
	for rows.Next() {
		var it InvoiceSummary
		if err := rows.Scan(&it.ID, &it.Number, &it.Status); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "فشل القراءة"}
		}
		items = append(items, it)
	}
	return &InvoicesResponse{Items: items}, nil
}

//encore:api auth method=GET path=/invoices/:id
func GetInvoice(ctx context.Context, id string) (*InvoiceSummary, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	uid, _ := strconv.ParseInt(string(uidStr), 10, 64)
	var iid int64
	if v, err := strconv.ParseInt(id, 10, 64); err == nil {
		iid = v
	} else {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "معرّف غير صالح"}
	}
	var ownerID int64
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT o.user_id FROM invoices i JOIN orders o ON o.id=i.order_id WHERE i.id=$1`, iid).Scan(&ownerID); err != nil {
		return nil, &errs.Error{Code: "INV_NOT_FOUND", Message: "الفاتورة غير موجودة"}
	}
	if ownerID != uid {
		var role string
		_ = db.Stdlib().QueryRowContext(ctx, `SELECT role::text FROM users WHERE id=$1`, uid).Scan(&role)
		if strings.ToLower(role) != "admin" {
			return nil, &errs.Error{Code: errs.Forbidden, Message: "غير مصرح"}
		}
	}
	var res InvoiceSummary
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT id, number, status::text FROM invoices WHERE id=$1`, iid).Scan(&res.ID, &res.Number, &res.Status); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل القراءة"}
	}
	return &res, nil
}

type CreateAuctionWinnerParams struct {
	AuctionID          int64   `json:"auction_id"`
	ProductID          int64   `json:"product_id"`
	WinnerUserID       int64   `json:"winner_user_id"`
	WinningAmountGross float64 `json:"winning_amount_gross"`
}

type CreateAuctionWinnerResponse struct {
	OrderID   int64 `json:"order_id"`
	InvoiceID int64 `json:"invoice_id"`
}

//encore:api private
func CreateAuctionWinnerOrder(ctx context.Context, p *CreateAuctionWinnerParams) (*CreateAuctionWinnerResponse, error) {
	if p == nil || p.AuctionID == 0 || p.ProductID == 0 || p.WinnerUserID == 0 || p.WinningAmountGross < 0 {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "بيانات غير مكتملة"}
	}
	// Validate product is pigeon
	var ptype string
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT type::text FROM products WHERE id=$1`, p.ProductID).Scan(&ptype); err != nil {
		return nil, &errs.Error{Code: errs.NotFound, Message: "المنتج غير موجود"}
	}
	if ptype != "pigeon" {
		return nil, &errs.Error{Code: errs.Conflict, Message: "المنتج ليس حمامة"}
	}

	tx, err := db.Stdlib().BeginTx(ctx, nil)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء معاملة"}
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Move product to auction_hold
	if _, err = tx.ExecContext(ctx, `UPDATE products SET status='auction_hold', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, p.ProductID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث حالة المنتج"}
	}

	// Create order with source=auction
	var orderID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO orders (user_id, source) VALUES ($1,'auction') RETURNING id`, p.WinnerUserID).Scan(&orderID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء الطلب"}
	}
	// Add single item (qty=1) at winning gross
	if _, err = tx.ExecContext(ctx, `INSERT INTO order_items (order_id, product_id, qty, unit_price_gross, line_total_gross) VALUES ($1,$2,1,$3,$3)`, orderID, p.ProductID, p.WinningAmountGross); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إدراج عنصر الطلب"}
	}
	// Touch to recalc totals via trigger
	if _, err = tx.ExecContext(ctx, `UPDATE orders SET updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, orderID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل احتساب الإجماليات"}
	}

	// Snapshot vat rate
	s := config.GetSettings()
	vatRate := 0.0
	if s != nil && s.VATEnabled {
		vatRate = s.VATRate
	}

	// Create invoice with next number
	year := time.Now().UTC().Year()
	var invoiceNumber string
	if err = tx.QueryRowContext(ctx, `SELECT next_invoice_number($1)`, year).Scan(&invoiceNumber); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل ترقيم الفاتورة"}
	}
	var invoiceID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO invoices (order_id, number, vat_rate_snapshot) VALUES ($1,$2,$3) RETURNING id`, orderID, invoiceNumber, vatRate).Scan(&invoiceID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء الفاتورة"}
	}

	if err = tx.Commit(); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل الحفظ"}
	}
	return &CreateAuctionWinnerResponse{OrderID: orderID, InvoiceID: invoiceID}, nil
}
