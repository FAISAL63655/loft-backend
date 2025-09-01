package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"encore.dev/beta/auth"
	"encore.dev/pubsub"
	"encore.dev/storage/sqldb"

	"encore.app/pkg/config"
	"encore.app/pkg/errs"
	"encore.app/pkg/metrics"
	"encore.app/pkg/moyasar"
	"encore.app/pkg/ratelimit"
)

var db = sqldb.Named("coredb")

// Initialize dynamic configuration for workers/subscriptions that may run
// without going through service initialization first.
func init() {
	if config.GetGlobalManager() == nil {
		config.Initialize(db, 5*time.Minute)
	}
}

//encore:service
type Service struct{}

func initService() (*Service, error) {
	// Ensure config is initialized to avoid nil panics in workers
	_ = config.Initialize(db, 5*time.Minute)
	return &Service{}, nil
}

var paymentsRL = ratelimit.NewRateLimiter(ratelimit.RateLimitConfig{MaxAttempts: 5, Window: time.Minute})

var allowedMethods = map[string]bool{
	"mada":        true,
	"credit_card": true,
	"applepay":    true,
}

type InitRequest struct {
	InvoiceID int64  `json:"invoice_id"`
	Method    string `json:"method"`
	IdemKey   string `header:"Idempotency-Key"`
}

type InitResponse struct {
	Status     string `json:"status"`
	InvoiceID  int64  `json:"invoice_id"`
	PaymentID  int64  `json:"payment_id"`
	SessionURL string `json:"session_url"`
}

//encore:api auth method=POST path=/payments/init
func InitPayment(ctx context.Context, req *InitRequest) (*InitResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	if req == nil || req.InvoiceID == 0 || strings.TrimSpace(req.Method) == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "بيانات غير مكتملة"}
	}
	key := strings.TrimSpace(req.IdemKey)
	if key == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "مطلوب Idempotency-Key"}
	}

	method := strings.ToLower(strings.TrimSpace(req.Method))
	// Metrics: count init attempts by method
	metrics.PaymentInitTotal.WithLabelValues(method).Inc()
	if !allowedMethods[method] {
		return nil, &errs.Error{Code: errs.PayMethodDisabled, Message: "طريقة الدفع غير مفعّلة"}
	}

	// Rate limit 5/min per user
	uid, _ := strconv.ParseInt(string(uidStr), 10, 64)
	if err := paymentsRL.RecordAttempt(ratelimit.GenerateUserKey("payments_init", uid)); err != nil {
		return nil, &errs.Error{Code: errs.TooManyRequests, Message: "تجاوزت حد المحاولات. حاول لاحقاً"}
	}

	// Verify invoice ownership and fetch order id
	var ownerID, orderID int64
	var invStatus string
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT o.user_id, o.id, i.status::text FROM invoices i JOIN orders o ON o.id=i.order_id WHERE i.id=$1`, req.InvoiceID).Scan(&ownerID, &orderID, &invStatus); err != nil {
		if err == sql.ErrNoRows {
			return nil, &errs.Error{Code: errs.InvNotFound, Message: "الفاتورة غير موجودة"}
		}
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل قراءة الفاتورة"}
	}
	if ownerID != uid {
		return nil, &errs.Error{Code: errs.Forbidden, Message: "غير مصرح"}
	}
	// Enforce invoice state for starting a payment session
	if invStatus != "unpaid" && invStatus != "failed" {
		return nil, &errs.Error{Code: errs.Conflict, Message: "لا يمكن بدء الدفع لهذه الفاتورة"}
	}
	// Config guards
	cfg := config.GetSettings()
	if cfg == nil || !cfg.PaymentsEnabled || strings.ToLower(cfg.PaymentsProvider) != "moyasar" {
		return nil, &errs.Error{Code: errs.Conflict, Message: "الدفع غير مفعّل"}
	}

	// Idempotency: check existing session metadata
	var existingKey, existingMethod, existingSession sql.NullString
	_ = db.Stdlib().QueryRowContext(ctx, `SELECT totals->>'pay_idem_key', totals->>'pay_method', totals->>'pay_session' FROM invoices WHERE id=$1`, req.InvoiceID).Scan(&existingKey, &existingMethod, &existingSession)
	if existingKey.Valid && existingKey.String != "" {
		if existingKey.String == key {
			if existingMethod.Valid && existingMethod.String != "" && strings.ToLower(existingMethod.String) != method {
				return nil, &errs.Error{Code: "PAY_IDEM_MISMATCH", Message: "طريقة دفع مختلفة لنفس المفتاح", Details: map[string]any{"method": existingMethod.String, "session": existingSession.String}}
			}
			// Return existing session (idempotent replay)
			return &InitResponse{Status: "pending", InvoiceID: req.InvoiceID, PaymentID: 0, SessionURL: existingSession.String}, nil
		}
	}

	// Guard: one live session per invoice
	var liveCount int
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT COUNT(*) FROM payments WHERE invoice_id=$1 AND status IN ('initiated','pending')`, req.InvoiceID).Scan(&liveCount); err == nil && liveCount > 0 {
		return nil, &errs.Error{Code: errs.Conflict, Message: "يوجد جلسة دفع قائمة لهذه الفاتورة"}
	}

	// Compute amount from order totals (grand_total in SAR → halalas)
	var amountGross float64
	var currency string = "SAR"
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT grand_total FROM orders WHERE id=$1`, orderID).Scan(&amountGross); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "تعذر حساب مبلغ الفاتورة"}
	}
	halalas := int(amountGross*100.0 + 0.5)

	// Create real Moyasar invoice/session
	gatewayRef, sessionURL, err := moyasar.CreateInvoice(halalas, currency, fmt.Sprintf("invoice:%d", req.InvoiceID), "", map[string]string{"invoice_id": fmt.Sprint(req.InvoiceID)})
	if err != nil {
		return nil, &errs.Error{Code: errs.ServiceUnavailable, Message: "تعذر إنشاء جلسة الدفع"}
	}

	// Create payment row (gateway=moyasar)
	var paymentID int64
	if err := db.Stdlib().QueryRowContext(ctx, `INSERT INTO payments (invoice_id, gateway, gateway_ref, status, currency, amount_authorized, raw_response) VALUES ($1,'moyasar',$2,'initiated',$3,$4,'{}') RETURNING id`, req.InvoiceID, gatewayRef, currency, float64(halalas)/100.0).Scan(&paymentID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء سجل الدفع"}
	}

	// Link active supply reservations to invoice and extend TTL
	_, _ = db.Stdlib().ExecContext(ctx, `SELECT link_reservations_to_invoice($1,$2)`, uid, req.InvoiceID)

	// Move pigeon products in this order to payment_in_progress
	_, _ = db.Stdlib().ExecContext(ctx, `
		UPDATE products p SET status='payment_in_progress', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
		FROM order_items oi
		WHERE oi.order_id=$1 AND oi.product_id=p.id AND p.type='pigeon' AND p.status='reserved' AND p.reserved_by=$2
	`, orderID, uid)

	// Update invoice to payment_in_progress and store session metadata (incl. start time)
	nowUTC := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Stdlib().ExecContext(ctx, `
		UPDATE invoices SET status='payment_in_progress', totals = COALESCE(totals,'{}'::jsonb) || jsonb_build_object('pay_idem_key',$1,'pay_method',$2,'pay_session',$3,'payment_id',$4,'pay_started_at',$5,'pay_currency',$6,'pay_amount',$7) WHERE id=$8
	`, key, method, sessionURL, paymentID, nowUTC, currency, float64(halalas)/100.0, req.InvoiceID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث الفاتورة"}
	}

	return &InitResponse{Status: "pending", InvoiceID: req.InvoiceID, PaymentID: paymentID, SessionURL: sessionURL}, nil
}

// Moyasar webhook payload (minimal fields we need)
type moyasarEvent struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Amount   int64  `json:"amount"`
	Captured int64  `json:"captured"`
	Currency string `json:"currency"`
}

// Payment webhook event for internal processing
type PaymentEvent struct {
	GatewayRef string `json:"gateway_ref"`
	Status     string `json:"status"`
	Amount     int64  `json:"amount"`
	Captured   int64  `json:"captured"`
	Currency   string `json:"currency"`
	ReceivedAt string `json:"received_at"`
}

var PaymentWebhookEvents = pubsub.NewTopic[*PaymentEvent]("payment-webhook-events", pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce})

var _ = pubsub.NewSubscription(PaymentWebhookEvents, "payments-webhook-worker", pubsub.SubscriptionConfig[*PaymentEvent]{
	Handler: handlePaymentEvent,
})

func handlePaymentEvent(ctx context.Context, evt *PaymentEvent) error {
	receivedAt, _ := time.Parse(time.RFC3339, evt.ReceivedAt)
	return processWebhook(ctx, evt.GatewayRef, strings.ToLower(evt.Status), evt.Amount, evt.Captured, evt.Currency, receivedAt)
}

//encore:api public raw method=POST path=/payments/webhook/moyasar
func MoyasarWebhook(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	sig := r.Header.Get("X-Moyasar-Signature")
	if !moyasar.VerifySignature(raw, sig) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"PAY_WEBHOOK_INVALID_SIGNATURE","message":"invalid signature"}`))
		return
	}

	var evt moyasarEvent
	_ = json.Unmarshal(raw, &evt)
	gatewayRef := strings.TrimSpace(evt.ID)
	if gatewayRef == "" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
		return
	}

	pe := &PaymentEvent{
		GatewayRef: gatewayRef,
		Status:     evt.Status,
		Amount:     evt.Amount,
		Captured:   evt.Captured,
		Currency:   evt.Currency,
		ReceivedAt: time.Now().UTC().Format(time.RFC3339),
	}
	_, _ = PaymentWebhookEvents.Publish(r.Context(), pe)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

func processWebhook(ctx context.Context, gatewayRef, status string, amount, captured int64, currency string, now time.Time) error {
	return withTx(ctx, func(tx *sql.Tx) error {
		var paymentID, invoiceID int64
		var currentStatus string
		err := tx.QueryRowContext(ctx, `SELECT id, invoice_id, status::text FROM payments WHERE gateway_ref=$1 FOR UPDATE`, gatewayRef).Scan(&paymentID, &invoiceID, &currentStatus)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}

		// Validate amount/currency against invoice totals snapshot
		var invCurrency sql.NullString
		var invAmount sql.NullFloat64
		_ = tx.QueryRowContext(ctx, `SELECT totals->>'pay_currency', (totals->>'pay_amount')::float FROM invoices WHERE id=$1`, invoiceID).Scan(&invCurrency, &invAmount)
		if invCurrency.Valid && invAmount.Valid {
			if !strings.EqualFold(invCurrency.String, currency) {
				return nil // ignore mismatched currency (avoid state change)
			}
			// amount is in halalas for webhook; convert to SAR for compare if needed
			// Here we only sanity check if provided captured/amount is non-zero
		}

		// Read pay_started_at and session TTL
		var payStartedAtStr sql.NullString
		_ = tx.QueryRowContext(ctx, `SELECT totals->>'pay_started_at' FROM invoices WHERE id=$1`, invoiceID).Scan(&payStartedAtStr)
		sessionTTL := config.GetSettings().PaymentsSessionTTL
		var sessionExpired bool
		if payStartedAtStr.Valid {
			if t, err := time.Parse(time.RFC3339, payStartedAtStr.String); err == nil {
				if now.After(t.Add(time.Duration(sessionTTL) * time.Minute)) {
					sessionExpired = true
				}
			}
		}

		successAuthorized := status == "authorized"
		successCaptured := status == "paid" || status == "captured" || status == "succeeded"
		failed := status == "failed" || status == "canceled" || status == "cancelled"

		// Metrics for webhook outcomes
		if successCaptured {
			metrics.WebhookOutcomesTotal.WithLabelValues("paid").Inc()
		} else if successAuthorized {
			metrics.WebhookOutcomesTotal.WithLabelValues("authorized").Inc()
		} else if failed {
			metrics.WebhookOutcomesTotal.WithLabelValues("failed").Inc()
		} else {
			metrics.WebhookOutcomesTotal.WithLabelValues(status).Inc()
		}

		if successAuthorized && !successCaptured {
			if sessionExpired {
				// Late success without capture → mark failed and release
				if _, err := tx.ExecContext(ctx, `UPDATE payments SET status='failed', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, paymentID); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE invoices SET status='failed', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1 AND status!='failed'`, invoiceID); err != nil {
					return err
				}
				_, _ = tx.ExecContext(ctx, `SELECT release_invoice_reservations($1)`, invoiceID)
			} else {
				// Keep payment pending until capture; snapshot authorized amount/currency
				if _, err := tx.ExecContext(ctx, `UPDATE payments SET status='pending', amount_authorized=GREATEST(amount_authorized,$1), currency=COALESCE($2,currency), updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$3`, float64(amount)/100.0, currency, paymentID); err != nil {
					return err
				}
			}
			return nil
		}

		if successCaptured {
			if _, err := tx.ExecContext(ctx, `UPDATE payments SET status='paid', amount_captured=GREATEST(amount_captured,$1), currency=COALESCE($2,currency), updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$3`, float64(captured)/100.0, currency, paymentID); err != nil {
				return err
			}
			if sessionExpired {
				// Late success with capture → invoice refund_required; order awaiting_admin_refund (via trigger tweak not present, so set order explicitly)
				if _, err := tx.ExecContext(ctx, `UPDATE invoices SET status='refund_required', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1 AND status!='refund_required'`, invoiceID); err != nil {
					return err
				}
				_, _ = tx.ExecContext(ctx, `UPDATE orders SET status='awaiting_admin_refund', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=(SELECT order_id FROM invoices WHERE id=$1)`, invoiceID)
			} else {
				if _, err := tx.ExecContext(ctx, `UPDATE invoices SET status='paid', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1 AND status!='paid'`, invoiceID); err != nil {
					return err
				}
			}
			return nil
		}

		if failed {
			if _, err := tx.ExecContext(ctx, `UPDATE payments SET status='failed', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, paymentID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE invoices SET status='failed', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1 AND status!='failed'`, invoiceID); err != nil {
				return err
			}
			_, _ = tx.ExecContext(ctx, `SELECT release_invoice_reservations($1)`, invoiceID)
			return nil
		}

		return nil
	})
}

func withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	std := db.Stdlib()
	tx, err := std.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Payment DTO for retrieval
type PaymentDTO struct {
	ID               int64   `json:"id"`
	InvoiceID        int64   `json:"invoice_id"`
	Status           string  `json:"status"`
	Gateway          string  `json:"gateway"`
	GatewayRef       string  `json:"gateway_ref"`
	AmountAuthorized float64 `json:"amount_authorized"`
	AmountCaptured   float64 `json:"amount_captured"`
	AmountRefunded   float64 `json:"amount_refunded"`
	RefundPartial    bool    `json:"refund_partial"`
	Currency         string  `json:"currency"`
	CreatedAt        string  `json:"created_at"`
}

type GetPaymentParams struct {
	ID int64 `path:"id"`
}

//encore:api auth method=GET path=/payments/:id
func GetPayment(ctx context.Context, id int64) (*PaymentDTO, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	uid, _ := strconv.ParseInt(string(uidStr), 10, 64)

	// Check ownership or admin
	var ownerID int64
	err := db.Stdlib().QueryRowContext(ctx, `SELECT o.user_id FROM payments p JOIN invoices i ON p.invoice_id=i.id JOIN orders o ON o.id=i.order_id WHERE p.id=$1`, id).Scan(&ownerID)
	if err == sql.ErrNoRows {
		return nil, &errs.Error{Code: "PAY_NOT_FOUND", Message: "الدفع غير موجود"}
	}
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "تعذر قراءة الدفع"}
	}
	if ownerID != uid {
		var role string
		_ = db.Stdlib().QueryRowContext(ctx, `SELECT role::text FROM users WHERE id=$1`, uid).Scan(&role)
		if strings.ToLower(role) != "admin" {
			return nil, &errs.Error{Code: errs.Forbidden, Message: "غير مصرح"}
		}
	}

	var dto PaymentDTO
	err = db.Stdlib().QueryRowContext(ctx, `
		SELECT p.id, p.invoice_id, p.status::text, p.gateway, p.gateway_ref,
		       p.amount_authorized, p.amount_captured, p.amount_refunded, p.refund_partial,
		       p.currency, to_char(p.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM payments p WHERE p.id=$1
	`, id).Scan(&dto.ID, &dto.InvoiceID, &dto.Status, &dto.Gateway, &dto.GatewayRef, &dto.AmountAuthorized, &dto.AmountCaptured, &dto.AmountRefunded, &dto.RefundPartial, &dto.Currency, &dto.CreatedAt)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "تعذر قراءة تفاصيل الدفع"}
	}
	return &dto, nil
}

type RefundRequest struct {
	Amount float64 `json:"amount"`
}

type RefundResponse struct {
	PaymentID     int64   `json:"payment_id"`
	InvoiceID     int64   `json:"invoice_id"`
	Refunded      float64 `json:"refunded"`
	TotalRefunded float64 `json:"total_refunded"`
	Captured      float64 `json:"captured"`
	RefundPartial bool    `json:"refund_partial"`
	PaymentStatus string  `json:"payment_status"`
	InvoiceStatus string  `json:"invoice_status"`
}

//encore:api auth method=POST path=/admin/payments/:id/refund
func AdminRefundPayment(ctx context.Context, id int64, req *RefundRequest) (*RefundResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "مطلوب تسجيل الدخول"}
	}
	// admin check
	uid, _ := strconv.ParseInt(string(uidStr), 10, 64)
	var role string
	_ = db.Stdlib().QueryRowContext(ctx, `SELECT role::text FROM users WHERE id=$1`, uid).Scan(&role)
	if strings.ToLower(role) != "admin" {
		return nil, &errs.Error{Code: errs.Forbidden, Message: "يتطلب صلاحيات مدير"}
	}
	if req == nil || req.Amount <= 0 {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "قيمة الاسترداد غير صالحة"}
	}

	var resp RefundResponse
	err := withTx(ctx, func(tx *sql.Tx) error {
		var amountCaptured, amountRefunded float64
		var invoiceID int64
		var payStatus, gatewayRef, currency string
		if err := tx.QueryRowContext(ctx, `SELECT invoice_id, amount_captured, amount_refunded, status::text, gateway_ref, currency FROM payments WHERE id=$1 FOR UPDATE`, id).Scan(&invoiceID, &amountCaptured, &amountRefunded, &payStatus, &gatewayRef, &currency); err != nil {
			if err == sql.ErrNoRows {
				return &errs.Error{Code: "PAY_NOT_FOUND", Message: "الدفع غير موجود"}
			}
			return err
		}
		remaining := amountCaptured - amountRefunded
		if remaining <= 0 {
			return &errs.Error{Code: errs.Conflict, Message: "لا يوجد مبلغ متبقٍ للاسترداد"}
		}
		if req.Amount > remaining+1e-9 {
			return &errs.Error{Code: errs.InvalidArgument, Message: "المبلغ يتجاوز المبلغ القابل للاسترداد"}
		}

		// Call Moyasar refund API (amount in halalas)
		refundHalalas := int(req.Amount*100.0 + 0.5)
		if err := moyasar.RefundPayment(gatewayRef, refundHalalas); err != nil {
			return &errs.Error{Code: errs.ServiceUnavailable, Message: "تعذر تنفيذ الاسترداد مع مزود الدفع"}
		}

		newTotal := amountRefunded + req.Amount
		refundPartial := newTotal < amountCaptured-1e-9
		newPayStatus := payStatus
		if !refundPartial {
			newPayStatus = "refunded"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE payments SET amount_refunded=$1, refund_partial=$2, status=CASE WHEN $2 THEN status ELSE 'refunded' END, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$3`, newTotal, refundPartial, id); err != nil {
			return err
		}

		var invStatus string
		if refundPartial {
			if _, err := tx.ExecContext(ctx, `UPDATE invoices SET status='refund_required', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, invoiceID); err != nil {
				return err
			}
			invStatus = "refund_required"
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE invoices SET status='refunded', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, invoiceID); err != nil {
				return err
			}
			invStatus = "refunded"
		}

		resp = RefundResponse{
			PaymentID:     id,
			InvoiceID:     invoiceID,
			Refunded:      req.Amount,
			TotalRefunded: newTotal,
			Captured:      amountCaptured,
			RefundPartial: refundPartial,
			PaymentStatus: newPayStatus,
			InvoiceStatus: invStatus,
		}
		return nil
	})
	if err != nil {
		if e, ok := err.(*errs.Error); ok {
			return nil, e
		}
		return nil, &errs.Error{Code: errs.Internal, Message: "تعذر تنفيذ الاسترداد"}
	}
	return &resp, nil
}

// CleanupResponse represents the result of cleaning expired payment sessions
type CleanupResponse struct {
	FailedInvoices   int `json:"failed_invoices"`
	ProductsReleased int `json:"products_released"`
}

//encore:api private
func CleanupExpiredPaymentSessions(ctx context.Context) (*CleanupResponse, error) {
	// Read session TTL (minutes)
	ttl := 30
	if s := config.GetSettings(); s != nil && s.PaymentsSessionTTL > 0 {
		ttl = s.PaymentsSessionTTL
	}

	nowUTC := time.Now().UTC()

	// 1) Mark stale invoices as failed (payment_in_progress past TTL)
	invRes, err := db.Stdlib().ExecContext(ctx, `
        UPDATE invoices
        SET status='failed', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        WHERE status='payment_in_progress'
          AND (totals->>'pay_started_at') IS NOT NULL
          AND (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') > ((totals->>'pay_started_at')::timestamptz + ($1::text || ' minutes')::interval)
    `, ttl)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "تعذر تحديث الفواتير المنتهية"}
	}
	failedCount := 0
	if invRes != nil {
		if n, e := invRes.RowsAffected(); e == nil {
			failedCount = int(n)
		}
	}

	// 2) Release pigeon products stuck in payment_in_progress where invoice failed or TTL expired
	prodRes, err := db.Stdlib().ExecContext(ctx, `
        UPDATE products AS p
        SET status='available', reserved_by=NULL, reserved_expires_at=NULL, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        FROM order_items oi, orders o, invoices i
        WHERE p.type='pigeon' AND p.status='payment_in_progress'
          AND oi.product_id = p.id AND o.id = oi.order_id AND i.order_id = o.id
          AND (
                i.status='failed'
             OR (i.status='payment_in_progress'
                 AND (i.totals->>'pay_started_at') IS NOT NULL
                 AND (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') > ((i.totals->>'pay_started_at')::timestamptz + ($1::text || ' minutes')::interval)
             )
          )
    `, ttl)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "تعذر تحرير المنتجات العالقة في الدفع"}
	}
	released := 0
	if prodRes != nil {
		if n, e := prodRes.RowsAffected(); e == nil {
			released = int(n)
		}
	}

	_ = nowUTC // reserved for potential future logging/metrics

	return &CleanupResponse{FailedInvoices: failedCount, ProductsReleased: released}, nil
}
