package worker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"encore.dev"
	"encore.dev/beta/auth"
	"encore.dev/pubsub"
	"encore.dev/storage/sqldb"

	"encore.app/pkg/config"
	"encore.app/pkg/errs"
	"encore.app/pkg/logger"
	"encore.app/pkg/mailer"
	"encore.app/pkg/metrics"
	"encore.app/pkg/moyasar"
	"encore.app/pkg/ratelimit"
	"encore.app/pkg/templates"
	"encore.app/svc/notifications"
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

	// ========= اختيار الدومين للـ URLs (يفضّل www.dughairiloft.com) =========
	frontendBase := "http://localhost:3000"

	if s := config.GetSettings(); s != nil && len(s.CORSAllowedOrigins) > 0 {
		// 1) فضّل www.dughairiloft.com إن وُجد
		for _, o := range s.CORSAllowedOrigins {
			o = strings.TrimSpace(o)
			if o != "" && o != "*" && strings.Contains(o, "www.dughairiloft.com") {
				frontendBase = strings.TrimRight(o, "/")
				break
			}
		}
		// 2) وإلا أي نطاق يحتوي dughairiloft.com
		if !strings.Contains(frontendBase, "dughairiloft.com") {
			for _, o := range s.CORSAllowedOrigins {
				o = strings.TrimSpace(o)
				if o != "" && o != "*" && strings.Contains(o, "dughairiloft.com") {
					frontendBase = strings.TrimRight(o, "/")
					break
				}
			}
		}
		// 3) إن ما لقينا، خذ أول non-admin صالح (للتجارب)
		if strings.HasPrefix(frontendBase, "http://localhost") || strings.HasPrefix(frontendBase, "http://127.") {
			for _, o := range s.CORSAllowedOrigins {
				o = strings.TrimSpace(o)
				if o != "" && o != "*" && !strings.Contains(o, "admin.") {
					frontendBase = strings.TrimRight(o, "/")
					break
				}
			}
		}
	}

	// في بيئات غير التطوير/المحلي: امنع localhost و vercel.app واستخدم دومين الإنتاج + https
	if encore.Meta().Environment.Type != encore.EnvLocal && encore.Meta().Environment.Type != encore.EnvDevelopment {
		if strings.HasPrefix(frontendBase, "http://localhost") || strings.HasPrefix(frontendBase, "http://127.") || strings.Contains(frontendBase, "vercel.app") {
			frontendBase = "https://www.dughairiloft.com"
		}
		if strings.HasPrefix(frontendBase, "http://www.dughairiloft.com") {
			frontendBase = "https://www.dughairiloft.com"
		}
	}

	// returnURL & callbackURL بعد الدفع
	returnURL := fmt.Sprintf("%s/checkout/callback?invoice_id=%d", frontendBase, req.InvoiceID)
	callbackURL := returnURL

	// webhookURL: محلي فقط (الإنتاج مضبوط من لوحة مزوّد الدفع)
	webhookURL := ""
	if encore.Meta().Environment.Type == encore.EnvLocal {
		webhookURL = "http://127.0.0.1:4000/payments/webhook/moyasar"
	}

	gatewayRef, sessionURL, err := moyasar.CreateInvoice(
		halalas, currency, fmt.Sprintf("invoice:%d", req.InvoiceID),
		callbackURL, returnURL, webhookURL,
		map[string]string{"invoice_id": fmt.Sprint(req.InvoiceID)},
	)
	if err != nil {
		logger.LogError(ctx, err, "moyasar create invoice failed", logger.Fields{
			"invoice_id":       req.InvoiceID,
			"amount_halalas":   halalas,
			"currency":         currency,
			"payments_enabled": config.GetSettings() != nil && config.GetSettings().PaymentsEnabled,
			"provider": func() string {
				if s := config.GetSettings(); s != nil {
					return s.PaymentsProvider
				}
				return ""
			}(),
		})
		return nil, &errs.Error{Code: errs.ServiceUnavailable, Message: "تعذر إنشاء جلسة الدفع"}
	}

	// Create payment row (gateway=moyasar)
	var paymentID int64
	if err := db.Stdlib().QueryRowContext(ctx, `INSERT INTO payments (invoice_id, gateway, gateway_ref, status, currency, amount_authorized, raw_response) VALUES ($1,'moyasar',$2,'initiated',$3,$4,'{}') RETURNING id`, req.InvoiceID, gatewayRef, currency, float64(halalas)/100.0).Scan(&paymentID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل إنشاء سجل الدفع"}
	}

	// Update invoice to payment_in_progress and store session metadata (incl. start time)
	nowUTC := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Stdlib().ExecContext(ctx, `
		UPDATE invoices
		SET status='payment_in_progress',
			totals = COALESCE(totals,'{}'::jsonb)
					|| jsonb_build_object(
						'pay_idem_key', $1::text,
						'pay_method',   $2::text,
						'pay_session',  $3::text,
						'payment_id',   $4::bigint,
						'pay_started_at',$5::text,
						'pay_currency', $6::text,
						'pay_amount',   $7::numeric
					)
		WHERE id=$8
	`, key, method, sessionURL, paymentID, nowUTC, currency, float64(halalas)/100.0, req.InvoiceID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "فشل تحديث الفاتورة: " + err.Error()}
	}

	// In local/test mode simulate a successful payment via internal pubsub to unblock the flow
	if s := config.GetSettings(); (s != nil && s.PaymentsTestMode) || encore.Meta().Environment.Type == encore.EnvLocal {
		go func(gwRef string, invID int64, amt int, curr string) {
			ctx2 := context.Background()
			evt := &PaymentEvent{
				GatewayRef: gwRef,
				Status:     "paid",
				Amount:     int64(amt),
				Captured:   int64(amt),
				Currency:   curr,
				ReceivedAt: time.Now().UTC().Format(time.RFC3339),
				InvoiceID:  invID,
			}
			if _, err := PaymentWebhookEvents.Publish(ctx2, evt); err != nil {
				logger.LogError(ctx2, err, "publish test payment event failed", logger.Fields{"gateway_ref": gwRef})
			}
		}(gatewayRef, req.InvoiceID, halalas, currency)
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
	InvoiceID  int64  `json:"invoice_id"`
}

var PaymentWebhookEvents = pubsub.NewTopic[*PaymentEvent]("payment-webhook-events", pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce})

var _ = pubsub.NewSubscription(PaymentWebhookEvents, "payments-webhook-worker", pubsub.SubscriptionConfig[*PaymentEvent]{
	Handler: handlePaymentEvent,
})

func handlePaymentEvent(ctx context.Context, evt *PaymentEvent) error {
	logger.Info(ctx, "handlePaymentEvent begin", logger.Fields{
		"gateway_ref": evt.GatewayRef,
		"invoice_id":  evt.InvoiceID,
		"status":      evt.Status,
		"amount":      evt.Amount,
		"captured":    evt.Captured,
	})
	receivedAt, _ := time.Parse(time.RFC3339, evt.ReceivedAt)
	err := processWebhook(ctx, evt.GatewayRef, evt.InvoiceID, strings.ToLower(evt.Status), evt.Amount, evt.Captured, evt.Currency, receivedAt)
	if err != nil {
		logger.LogError(ctx, err, "processWebhook failed", logger.Fields{
			"gateway_ref": evt.GatewayRef,
			"invoice_id":  evt.InvoiceID,
		})
	} else {
		logger.Info(ctx, "handlePaymentEvent completed successfully", logger.Fields{
			"gateway_ref": evt.GatewayRef,
			"invoice_id":  evt.InvoiceID,
		})
	}
	return err
}

//encore:api public raw method=POST path=/payments/webhook/moyasar
func MoyasarWebhook(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	// Reset body so we can ParseForm() after reading raw
	r.Body = io.NopCloser(bytes.NewReader(raw))
	// Accept multiple possible signature header names
	sig := r.Header.Get("X-Moyasar-Signature")
	if sig == "" {
		sig = r.Header.Get("X-Signature")
	}
	if sig == "" {
		sig = r.Header.Get("Signature")
	}
	if sig == "" {
		sig = r.Header.Get("X-Webhook-Signature")
	}
	if sig == "" {
		sig = r.Header.Get("Moyasar-Signature")
	}
	if sig == "" {
		sig = r.Header.Get("Moyasar-Webhook-Signature")
	}
	// In test mode, bypass verification entirely (useful for local dev and sandbox).
	// Otherwise, accept either header-based HMAC OR payload `secret_token` (per Moyasar docs).
	if s := config.GetSettings(); !(s != nil && s.PaymentsTestMode) {
		verified := false
		// Try header-based HMAC first
		if strings.TrimSpace(sig) != "" && moyasar.VerifySignature(raw, sig) {
			verified = true
		}
		// If no valid signature header, try payload secret_token
		if !verified {
			ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
			var token string
			if strings.Contains(ct, "application/x-www-form-urlencoded") {
				if err := r.ParseForm(); err == nil {
					token = strings.TrimSpace(r.FormValue("secret_token"))
					if token == "" {
						token = strings.TrimSpace(r.FormValue("webhook[secret_token]"))
					}
				}
			} else {
				var obj map[string]any
				if err := json.Unmarshal(raw, &obj); err == nil {
					if v, ok := obj["secret_token"].(string); ok {
						token = strings.TrimSpace(v)
					}
				}
			}
			if token != "" && moyasar.VerifySecretToken(token) {
				verified = true
			}
		}
		if !verified {
			logger.Info(r.Context(), "moyasar webhook invalid signature", logger.Fields{
				"sig_present": sig != "",
				"sig_len":     len(sig),
				"content_len": len(raw),
			})
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"PAY_WEBHOOK_INVALID_SIGNATURE","message":"invalid signature"}`))
			return
		}
	}

	// Parse payload: support both JSON and application/x-www-form-urlencoded
	var evt moyasarEvent
	ct := r.Header.Get("Content-Type")
	parsed := false
	var invoiceIDFromMeta int64
	if strings.Contains(strings.ToLower(ct), "application/x-www-form-urlencoded") {
		// Form-encoded payload (common from Moyasar webhooks)
		if err := r.ParseForm(); err == nil {
			evt.ID = r.FormValue("id")
			evt.Status = r.FormValue("status")
			evt.Currency = r.FormValue("currency")
			if a := strings.TrimSpace(r.FormValue("amount")); a != "" {
				if v, err := strconv.ParseInt(a, 10, 64); err == nil {
					evt.Amount = v
				}
			}
			if c := strings.TrimSpace(r.FormValue("captured")); c != "" {
				if v, err := strconv.ParseInt(c, 10, 64); err == nil {
					evt.Captured = v
				}
			}
			// Try nested data[...] keys
			if evt.ID == "" {
				evt.ID = r.FormValue("data[id]")
			}
			if evt.Status == "" {
				evt.Status = r.FormValue("data[status]")
			}
			if evt.Currency == "" {
				evt.Currency = r.FormValue("data[currency]")
			}
			if evt.Amount == 0 {
				if a := strings.TrimSpace(r.FormValue("data[amount]")); a != "" {
					if v, err := strconv.ParseInt(a, 10, 64); err == nil {
						evt.Amount = v
					}
				}
			}
			if evt.Captured == 0 {
				if c := strings.TrimSpace(r.FormValue("data[captured]")); c != "" {
					if v, err := strconv.ParseInt(c, 10, 64); err == nil {
						evt.Captured = v
					}
				}
			}
			// Extract metadata invoice_id if provided as metadata[invoice_id] or invoice_id
			if inv := strings.TrimSpace(r.FormValue("metadata[invoice_id]")); inv != "" {
				if v, err := strconv.ParseInt(inv, 10, 64); err == nil {
					invoiceIDFromMeta = v
				}
			}
			if invoiceIDFromMeta == 0 {
				if inv := strings.TrimSpace(r.FormValue("invoice_id")); inv != "" {
					if v, err := strconv.ParseInt(inv, 10, 64); err == nil {
						invoiceIDFromMeta = v
					}
				}
			}
			if invoiceIDFromMeta == 0 {
				if inv := strings.TrimSpace(r.FormValue("data[metadata][invoice_id]")); inv != "" {
					if v, err := strconv.ParseInt(inv, 10, 64); err == nil {
						invoiceIDFromMeta = v
					}
				}
			}
			// Derive invoice id from description "invoice:<id>" or "Invoice #INV-YYYY-NNNNNN" if metadata missing
			if invoiceIDFromMeta == 0 {
				desc := strings.TrimSpace(r.FormValue("description"))
				if desc == "" {
					desc = strings.TrimSpace(r.FormValue("data[description]"))
				}
				if strings.HasPrefix(strings.ToLower(desc), "invoice:") {
					if v, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(desc, "invoice:")), 10, 64); err == nil {
						invoiceIDFromMeta = v
					}
				}
				// Try to extract from "Invoice #INV-YYYY-NNNNNN" format
				if invoiceIDFromMeta == 0 && strings.Contains(desc, "Invoice #INV-") {
					parts := strings.Split(desc, "Invoice #INV-")
					if len(parts) > 1 {
						invNum := strings.TrimSpace(parts[1])
						// invNum is like "2025-000030" or "2025-000030..."
						invNum = strings.Split(invNum, " ")[0] // Take first part before space
						// Query database to find invoice by number
						var invID int64
						if err := db.Stdlib().QueryRowContext(r.Context(), `SELECT id FROM invoices WHERE number=$1`, "INV-"+invNum).Scan(&invID); err == nil {
							invoiceIDFromMeta = invID
						}
					}
				}
			}
			// Some providers wrap JSON in a 'payload' field; try to parse it
			if p := strings.TrimSpace(r.FormValue("payload")); p != "" && evt.ID == "" {
				var t map[string]any
				if err := json.Unmarshal([]byte(p), &t); err == nil {
					if dm, ok := t["data"].(map[string]any); ok {
						if v, ok := dm["id"].(string); ok {
							evt.ID = v
						}
						if v, ok := dm["status"].(string); ok && evt.Status == "" {
							evt.Status = v
						}
						if v, ok := dm["currency"].(string); ok && evt.Currency == "" {
							evt.Currency = v
						}
						if v, ok := dm["amount"].(float64); ok && evt.Amount == 0 {
							evt.Amount = int64(v)
						}
						if v, ok := dm["captured"].(float64); ok && evt.Captured == 0 {
							evt.Captured = int64(v)
						}
						if m, ok := dm["metadata"].(map[string]any); ok && invoiceIDFromMeta == 0 {
							if inv, ok := m["invoice_id"]; ok {
								switch t := inv.(type) {
								case string:
									if v, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
										invoiceIDFromMeta = v
									}
								case float64:
									invoiceIDFromMeta = int64(t)
								}
							}
						}
					}
				}
			}
			parsed = true
		}
	}
	if !parsed {
		_ = json.Unmarshal(raw, &evt)
		// Try to parse metadata.invoice_id from JSON
		var rawMap map[string]any
		if err := json.Unmarshal(raw, &rawMap); err == nil {
			// If event envelope has data{...}
			if dm, ok := rawMap["data"].(map[string]any); ok {
				if evt.ID == "" {
					if v, ok := dm["id"].(string); ok {
						evt.ID = v
					}
				}
				if evt.Status == "" {
					if v, ok := dm["status"].(string); ok {
						evt.Status = v
					}
				}
				if evt.Currency == "" {
					if v, ok := dm["currency"].(string); ok {
						evt.Currency = v
					}
				}
				if evt.Amount == 0 {
					if v, ok := dm["amount"].(float64); ok {
						evt.Amount = int64(v)
					}
				}
				if evt.Captured == 0 {
					if v, ok := dm["captured"].(float64); ok {
						evt.Captured = int64(v)
					}
				}
				if m, ok := dm["metadata"].(map[string]any); ok {
					if invoiceIDFromMeta == 0 {
						if inv, ok := m["invoice_id"]; ok {
							switch t := inv.(type) {
							case string:
								if v, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
									invoiceIDFromMeta = v
								}
							case float64:
								invoiceIDFromMeta = int64(t)
							}
						}
					}
				}
				if invoiceIDFromMeta == 0 {
					if d, ok := dm["description"].(string); ok {
						if strings.HasPrefix(strings.ToLower(d), "invoice:") {
							if v, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(d, "invoice:")), 10, 64); err == nil {
								invoiceIDFromMeta = v
							}
						}
						// Try to extract from "Invoice #INV-YYYY-NNNNNN" format
						if invoiceIDFromMeta == 0 && strings.Contains(d, "Invoice #INV-") {
							parts := strings.Split(d, "Invoice #INV-")
							if len(parts) > 1 {
								invNum := strings.TrimSpace(parts[1])
								invNum = strings.Split(invNum, " ")[0]
								var invID int64
								if err := db.Stdlib().QueryRowContext(r.Context(), `SELECT id FROM invoices WHERE number=$1`, "INV-"+invNum).Scan(&invID); err == nil {
									invoiceIDFromMeta = invID
								}
							}
						}
					}
				}
			}
			// look for metadata.invoice_id or invoice_id at top-level
			if m, ok := rawMap["metadata"].(map[string]any); ok {
				if inv, ok := m["invoice_id"]; ok {
					switch t := inv.(type) {
					case string:
						if v, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
							invoiceIDFromMeta = v
						}
					case float64:
						invoiceIDFromMeta = int64(t)
					}
				}
			}
			if invoiceIDFromMeta == 0 {
				if inv, ok := rawMap["invoice_id"]; ok {
					switch t := inv.(type) {
					case string:
						if v, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
							invoiceIDFromMeta = v
						}
					case float64:
						invoiceIDFromMeta = int64(t)
					}
				}
			}
			if invoiceIDFromMeta == 0 {
				if d, ok := rawMap["description"].(string); ok {
					if strings.HasPrefix(strings.ToLower(d), "invoice:") {
						if v, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(d, "invoice:")), 10, 64); err == nil {
							invoiceIDFromMeta = v
						}
					}
					// Try to extract from "Invoice #INV-YYYY-NNNNNN" format
					if invoiceIDFromMeta == 0 && strings.Contains(d, "Invoice #INV-") {
						parts := strings.Split(d, "Invoice #INV-")
						if len(parts) > 1 {
							invNum := strings.TrimSpace(parts[1])
							invNum = strings.Split(invNum, " ")[0]
							var invID int64
							if err := db.Stdlib().QueryRowContext(r.Context(), `SELECT id FROM invoices WHERE number=$1`, "INV-"+invNum).Scan(&invID); err == nil {
								invoiceIDFromMeta = invID
							}
						}
					}
				}
			}
			// If status still empty, try mapping from event 'type'
			if evt.Status == "" {
				if typ, ok := rawMap["type"].(string); ok {
					lt := strings.ToLower(typ)
					switch {
					case strings.Contains(lt, "captured"), strings.Contains(lt, "paid"), strings.Contains(lt, "succeeded"):
						evt.Status = "paid"
					case strings.Contains(lt, "authorized"):
						evt.Status = "authorized"
					case strings.Contains(lt, "failed"), strings.Contains(lt, "void"):
						evt.Status = "failed"
					}
				}
			}
		}
	}

	gatewayRef := strings.TrimSpace(evt.ID)

	// Persist raw payload for diagnostics (store as JSONB object with raw text & content-type)
	if gatewayRef != "" {
		_, _ = db.Stdlib().ExecContext(r.Context(),
			`UPDATE payments SET raw_response = jsonb_build_object('raw', $1::text, 'ct', $2::text), updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE gateway_ref=$3`,
			string(raw), ct, gatewayRef,
		)
	}
	if gatewayRef == "" && invoiceIDFromMeta > 0 {
		// Fallback: attach raw_response to the most recent pending/initiated payment for this invoice (via subquery)
		_, _ = db.Stdlib().ExecContext(r.Context(),
			`UPDATE payments SET raw_response = jsonb_build_object('raw', $1::text, 'ct', $2::text), updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
             WHERE id = (
               SELECT id FROM payments
               WHERE invoice_id=$3 AND status IN ('initiated','pending') AND gateway='moyasar'
               ORDER BY created_at DESC
               LIMIT 1
             )`,
			string(raw), ct, invoiceIDFromMeta,
		)
	}

	if gatewayRef == "" {
		// No actionable id – acknowledge to avoid retries but do nothing
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
		return
	}

	// If no payment row has this gateway_ref yet, try to claim the pending payment for invoice and set its gateway_ref now
	if invoiceIDFromMeta > 0 {
		logger.Info(r.Context(), "attach gateway_ref by invoice fallback attempt", logger.Fields{
			"invoice_id":  invoiceIDFromMeta,
			"gateway_ref": gatewayRef,
		})
		res, err := db.Stdlib().ExecContext(r.Context(),
			`UPDATE payments SET gateway_ref=$1, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
             WHERE id = (
               SELECT id FROM payments
               WHERE invoice_id=$2 AND gateway='moyasar' AND status IN ('initiated','pending') AND (gateway_ref IS NULL OR gateway_ref='')
               ORDER BY created_at DESC
               LIMIT 1
             )`,
			gatewayRef, invoiceIDFromMeta,
		)
		if err != nil {
			logger.LogError(r.Context(), err, "attach gateway_ref by invoice fallback failed", logger.Fields{
				"invoice_id":  invoiceIDFromMeta,
				"gateway_ref": gatewayRef,
			})
			return
		}
		rows, _ := res.RowsAffected()
		if rows > 0 {
			logger.Info(r.Context(), "attached gateway_ref by invoice fallback", logger.Fields{
				"invoice_id":  invoiceIDFromMeta,
				"gateway_ref": gatewayRef,
			})
		}
	}

	// Log parsed summary for diagnostics (no secrets)
	logger.Info(r.Context(), "moyasar webhook parsed", logger.Fields{
		"ct":       ct,
		"evt_id":   evt.ID,
		"status":   evt.Status,
		"amount":   evt.Amount,
		"captured": evt.Captured,
		"currency": evt.Currency,
		"inv_meta": invoiceIDFromMeta,
		"raw_body_preview": func() string {
			if len(raw) > 500 {
				return string(raw[:500]) + "..."
			}
			return string(raw)
		}(),
	})

	pe := &PaymentEvent{
		GatewayRef: gatewayRef,
		Status:     strings.ToLower(strings.TrimSpace(evt.Status)),
		Amount:     evt.Amount,
		Captured:   evt.Captured,
		Currency:   strings.ToUpper(strings.TrimSpace(evt.Currency)),
		ReceivedAt: time.Now().UTC().Format(time.RFC3339),
		InvoiceID:  invoiceIDFromMeta,
	}
	_, _ = PaymentWebhookEvents.Publish(r.Context(), pe)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

func processWebhook(ctx context.Context, gatewayRef string, invoiceIDHint int64, status string, amount, captured int64, currency string, now time.Time) error {
	logger.Info(ctx, "processWebhook begin", logger.Fields{
		"gateway_ref":  gatewayRef,
		"invoice_hint": invoiceIDHint,
		"status":       status,
		"amount":       amount,
		"captured":     captured,
		"currency":     currency,
	})
	// Post-commit notification data (if order transitions to paid)
	var notifyPaid bool
	var notifOrderID int64
	var notifInvoiceID int64
	var notifGrandTotal float64
	var notifBuyerName, notifBuyerEmail string

	err := withTx(ctx, func(tx *sql.Tx) error {
		var paymentID, invoiceID int64
		var currentStatus string
		err := tx.QueryRowContext(ctx, `SELECT id, invoice_id, status::text FROM payments WHERE gateway_ref=$1 FOR UPDATE`, gatewayRef).Scan(&paymentID, &invoiceID, &currentStatus)
		if err == sql.ErrNoRows {
			if invoiceIDHint > 0 {
				// Fallback: claim latest initiated/pending payment by invoice id
				err = tx.QueryRowContext(ctx, `
                    SELECT id, invoice_id, status::text
                    FROM payments
                    WHERE invoice_id=$1 AND gateway='moyasar' AND status IN ('initiated','pending')
                    ORDER BY created_at DESC
                    LIMIT 1
                    FOR UPDATE
                `, invoiceIDHint).Scan(&paymentID, &invoiceID, &currentStatus)
				if err == sql.ErrNoRows {
					logger.Info(ctx, "no payment found for invoice (early exit)", logger.Fields{
						"invoice_id": invoiceIDHint,
					})
					return nil
				}
				if err != nil {
					return err
				}
				// Attach gateway_ref now to ensure idempotency for subsequent events
				logger.Info(ctx, "attach gateway_ref by invoice fallback attempt", logger.Fields{
					"invoice_id":  invoiceIDHint,
					"gateway_ref": gatewayRef,
				})
				_, err = tx.ExecContext(ctx, `UPDATE payments SET gateway_ref=$1, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$2 AND (gateway_ref IS NULL OR gateway_ref='')`, gatewayRef, paymentID)
				if err != nil {
					logger.LogError(ctx, err, "attach gateway_ref by invoice fallback failed", logger.Fields{
						"payment_id":  paymentID,
						"invoice_id":  invoiceID,
						"gateway_ref": gatewayRef,
					})
					return err
				}
			} else {
				// Final fallback: match by amount/currency among latest initiated/pending payments
				// Convert amount halalas -> SAR float
				sar := float64(amount) / 100.0
				err = tx.QueryRowContext(ctx, `
                    SELECT p.id, p.invoice_id, p.status::text
                    FROM payments p
                    JOIN invoices i ON i.id = p.invoice_id
                    WHERE p.gateway='moyasar'
                      AND p.status IN ('initiated','pending')
                      AND i.status IN ('payment_in_progress')
                      AND p.currency = $1
                      AND ABS(p.amount_authorized - $2) < 0.01
                    ORDER BY p.created_at DESC
                    LIMIT 1
                    FOR UPDATE
                `, currency, sar).Scan(&paymentID, &invoiceID, &currentStatus)
				if err == sql.ErrNoRows {
					return nil
				}
				if err != nil {
					return err
				}
				// Attach gateway_ref now to ensure idempotency for subsequent events
				logger.Info(ctx, "attach gateway_ref by amount fallback attempt", logger.Fields{
					"payment_id":  paymentID,
					"invoice_id":  invoiceID,
					"gateway_ref": gatewayRef,
				})
				_, err = tx.ExecContext(ctx, `UPDATE payments SET gateway_ref=$1, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$2 AND (gateway_ref IS NULL OR gateway_ref='')`, gatewayRef, paymentID)
				if err != nil {
					logger.LogError(ctx, err, "attach gateway_ref by amount fallback failed", logger.Fields{
						"payment_id":  paymentID,
						"invoice_id":  invoiceID,
						"gateway_ref": gatewayRef,
					})
					return err
				}
			}
		}
		if err != nil {
			return err
		}

		logger.Info(ctx, "payment found", logger.Fields{
			"payment_id":     paymentID,
			"invoice_id":     invoiceID,
			"current_status": currentStatus,
		})

		// Validate currency (and optionally amount) against invoice totals snapshot.
		// Avoid casting in SQL to prevent transaction aborts on bad JSON; parse in Go instead.
		var invCurrency sql.NullString
		var invAmountStr sql.NullString
		_ = tx.QueryRowContext(ctx, `SELECT totals->>'pay_currency', totals->>'pay_amount' FROM invoices WHERE id=$1`, invoiceID).Scan(&invCurrency, &invAmountStr)
		if invCurrency.Valid {
			if !strings.EqualFold(invCurrency.String, currency) {
				return nil // ignore mismatched currency (avoid state change)
			}
		}
		var invAmount sql.NullFloat64
		if invAmountStr.Valid {
			if f, perr := strconv.ParseFloat(strings.TrimSpace(invAmountStr.String), 64); perr == nil {
				invAmount.Float64 = f
				invAmount.Valid = true
			}
		}

		// Read pay_started_at and session TTL
		var payStartedAtStr sql.NullString
		_ = tx.QueryRowContext(ctx, `SELECT totals->>'pay_started_at' FROM invoices WHERE id=$1`, invoiceID).Scan(&payStartedAtStr)
		// Use a safe default TTL if settings are unavailable
		settings := config.GetSettings()
		sessionTTL := 15
		if settings != nil && settings.PaymentsSessionTTL > 0 {
			sessionTTL = settings.PaymentsSessionTTL
		}
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

		logger.Info(ctx, "webhook status check", logger.Fields{
			"status":            status,
			"successAuthorized": successAuthorized,
			"successCaptured":   successCaptured,
			"failed":            failed,
			"sessionExpired":    sessionExpired,
		})

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
			} else {
				// Keep payment pending until capture; snapshot authorized amount/currency
				if _, err := tx.ExecContext(ctx, `UPDATE payments SET status='pending', amount_authorized=GREATEST(amount_authorized,$1), currency=COALESCE($2,currency), updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$3`, float64(amount)/100.0, currency, paymentID); err != nil {
					return err
				}
			}
			return nil
		}

		if successCaptured {
			logger.Info(ctx, "webhook: success captured", logger.Fields{"payment_id": paymentID, "invoice_id": invoiceID, "captured": captured, "amount": amount})
			// Use captured amount if available, otherwise use total amount (Moyasar sometimes sends captured=0 for paid status)
			capturedAmount := captured
			if capturedAmount == 0 && (status == "paid" || status == "succeeded") {
				capturedAmount = amount
			}
			if _, err := tx.ExecContext(ctx, `UPDATE payments SET status='paid', amount_captured=GREATEST(amount_captured,$1), currency=COALESCE($2,currency), updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$3`, float64(capturedAmount)/100.0, currency, paymentID); err != nil {
				return err
			}
			if sessionExpired {
				// Late success with capture → invoice refund_required; order awaiting_admin_refund (via trigger tweak not present, so set order explicitly)
				if _, err := tx.ExecContext(ctx, `UPDATE invoices SET status='refund_required', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1 AND status!='refund_required'`, invoiceID); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE orders SET status='awaiting_admin_refund', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=(SELECT order_id FROM invoices WHERE id=$1)`, invoiceID); err != nil {
					logger.LogError(ctx, err, "update order awaiting_admin_refund failed", logger.Fields{"invoice_id": invoiceID})
					return err
				}
			} else {
				// Normal success path
				// Finalize the order atomically and idempotently (update order FIRST to avoid trigger race)
				var orderID int64
				if err := tx.QueryRowContext(ctx, `SELECT order_id FROM invoices WHERE id=$1`, invoiceID).Scan(&orderID); err != nil {
					return err
				}
				logger.Info(ctx, "webhook: mark order paid attempt", logger.Fields{"order_id": orderID})
				res, err := tx.ExecContext(ctx, `UPDATE orders SET status='paid', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1 AND status!='paid'`, orderID)
				if err != nil {
					return err
				}
				rows, _ := res.RowsAffected()
				if rows > 0 {
					notifyPaid = true
					notifOrderID = orderID
					notifInvoiceID = invoiceID
					// Fetch order details for notification
					var buyerName, buyerEmail string
					if err := tx.QueryRowContext(ctx, `SELECT COALESCE(u.name,''), COALESCE(u.email,'') FROM orders o JOIN users u ON u.id = o.user_id WHERE o.id=$1`, orderID).Scan(&buyerName, &buyerEmail); err != nil {
						return err
					}
					notifBuyerName = buyerName
					notifBuyerEmail = buyerEmail
					// Fetch order grand total for notification
					var grandTotal float64
					if err := tx.QueryRowContext(ctx, `SELECT COALESCE(grand_total,0) FROM orders WHERE id=$1`, orderID).Scan(&grandTotal); err != nil {
						return err
					}
					notifGrandTotal = grandTotal
					// First time we mark order paid → perform atomic stock/product transitions with conflict detection
					// Count expected pigeon lines
					var pigeonsTotal int64
					_ = tx.QueryRowContext(ctx, `
                        SELECT COUNT(*)
                        FROM order_items oi
                        JOIN products p ON p.id=oi.product_id
                        WHERE oi.order_id=$1 AND p.type='pigeon'
                    `, orderID).Scan(&pigeonsTotal)
					// Sell pigeons only if currently available
					resP, err := tx.ExecContext(ctx, `
                        UPDATE products p
                        SET status='sold', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
                        FROM order_items oi
                        WHERE oi.order_id=$1 AND oi.product_id=p.id AND p.type='pigeon' AND p.status IN ('available','auction_hold')
                    `, orderID)
					if err != nil {
						return err
					}
					soldPigeons, _ := resP.RowsAffected()
					logger.Info(ctx, "webhook: sold pigeons", logger.Fields{"expected": pigeonsTotal, "updated": soldPigeons})

					// Count expected supply lines
					var suppliesTotal int64
					_ = tx.QueryRowContext(ctx, `
                        SELECT COUNT(*)
                        FROM order_items oi
                        JOIN products p ON p.id=oi.product_id
                        WHERE oi.order_id=$1 AND p.type='supply'
                    `, orderID).Scan(&suppliesTotal)
					// Deduct supplies only when enough stock
					resS, err := tx.ExecContext(ctx, `
                        UPDATE supplies s
                        SET stock_qty = s.stock_qty - oi.qty
                        FROM order_items oi
                        JOIN products p ON p.id = oi.product_id AND p.type='supply'
                        WHERE oi.order_id=$1 AND s.product_id = oi.product_id AND s.stock_qty >= oi.qty
                    `, orderID)
					if err != nil {
						return err
					}
					deductedSupplies, _ := resS.RowsAffected()
					logger.Info(ctx, "webhook: deducted supplies", logger.Fields{"expected": suppliesTotal, "updated": deductedSupplies})

					// If any conflict (couldn't sell or deduct), mark refund_required and admin follow-up
					if (pigeonsTotal > 0 && soldPigeons < pigeonsTotal) || (suppliesTotal > 0 && deductedSupplies < suppliesTotal) {
						if _, err := tx.ExecContext(ctx, `UPDATE invoices SET status='refund_required', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, invoiceID); err != nil {
							return err
						}
						if _, err := tx.ExecContext(ctx, `UPDATE orders SET status='awaiting_admin_refund', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, orderID); err != nil {
							return err
						}
						if _, err := tx.ExecContext(ctx, `UPDATE payments SET refund_partial=TRUE, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$1`, paymentID); err != nil {
							return err
						}
					} else {
						// Optional: clear user's cart after successful fulfillment
						if _, err := tx.ExecContext(ctx, `DELETE FROM cart_items WHERE user_id=(SELECT user_id FROM orders WHERE id=$1)`, orderID); err != nil {
							return err
						}
					}

					// Auto-create a pickup shipment for pigeon orders (no courier)
					var hasPigeons bool
					_ = tx.QueryRowContext(ctx, `
                        SELECT EXISTS(
                            SELECT 1 FROM order_items oi
                            JOIN products p ON p.id=oi.product_id
                            WHERE oi.order_id=$1 AND p.type='pigeon'
                        )
                    `, orderID).Scan(&hasPigeons)
					if hasPigeons {
						// Insert one pickup shipment if not already present for this order
						_, _ = tx.ExecContext(ctx, `
                            INSERT INTO shipments (order_id, delivery_method, status, tracking_ref)
                            SELECT $1, 'pickup', 'pending', $2
                            WHERE NOT EXISTS (
                                SELECT 1 FROM shipments WHERE order_id=$1 AND delivery_method='pickup'
                            )
                        `, orderID, "لوفت الدغيري - بريدة، القصيم")
					}
				}
				// Mark invoice paid at the end to ensure order gating above succeeds
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
			return nil
		}

		return nil
	})
	if err != nil {
		return err
	}

	// After commit, optionally notify the buyer their order was paid
	if notifyPaid {
		var buyerID int64
		_ = db.Stdlib().QueryRowContext(ctx, `SELECT user_id FROM orders WHERE id=$1`, notifOrderID).Scan(&buyerID)
		if buyerID > 0 {
			payload := map[string]any{
				"order_id":    notifOrderID,
				"invoice_id":  notifInvoiceID,
				"grand_total": fmt.Sprintf("%.2f", notifGrandTotal),
				"name":        notifBuyerName,
				"email":       notifBuyerEmail,
				"currency":    currency,
			}
			// Fire-and-forget notification; ignore errors
			_, _ = notifications.EnqueueInternal(ctx, buyerID, "order_paid", payload)

			// Send confirmation email to buyer
			go func() {
				// Check if order has pigeons for pickup info
				var hasPigeons bool
				_ = db.Stdlib().QueryRowContext(context.Background(), `
                    SELECT EXISTS(
                        SELECT 1 FROM order_items oi
                        JOIN products p ON p.id = oi.product_id
                        WHERE oi.order_id = $1 AND p.type = 'pigeon'
                    )
                `, notifOrderID).Scan(&hasPigeons)

				// Frontend URL للايميل
				frontendURL := "https://www.dughairiloft.com"
				if encore.Meta().Environment.Type == encore.EnvDevelopment || encore.Meta().Environment.Type == encore.EnvLocal {
					frontendURL = "http://localhost:3000"
				}

				emailData := templates.TemplateData{
					"name":        notifBuyerName,
					"order_id":    notifOrderID,
					"invoice_id":  notifInvoiceID,
					"grand_total": fmt.Sprintf("%.2f", notifGrandTotal),
					"has_pigeons": hasPigeons,
					"order_url":   fmt.Sprintf("%s/account/orders/%d", frontendURL, notifOrderID),
				}

				subject, htmlBody, textBody, err := templates.RenderTemplate("order_confirmation", "ar", emailData)
				if err != nil {
					fmt.Printf("ERROR: Failed to render order confirmation template: %v\n", err)
					return
				}

				mailClient := mailer.NewClient()
				err = mailClient.Send(context.Background(), mailer.Mail{
					// FromName and FromEmail will use defaults from mailer.Client secrets
					ToName:  notifBuyerName,
					ToEmail: notifBuyerEmail,
					Subject: subject,
					HTML:    htmlBody,
					Text:    textBody,
				})
				if err != nil {
					fmt.Printf("ERROR: Failed to send order confirmation email to %s: %v\n", notifBuyerEmail, err)
				} else {
					fmt.Printf("INFO: Order confirmation email sent successfully to %s (Order #%d)\n", notifBuyerEmail, notifOrderID)
				}
			}()
		}
	}

	logger.Info(ctx, "processWebhook completed successfully", logger.Fields{
		"gateway_ref":  gatewayRef,
		"invoice_hint": invoiceIDHint,
		"notify_paid":  notifyPaid,
	})
	return nil
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
	// Some historical rows may contain malformed pay_started_at strings; filter candidates defensively.
	invRes, err := db.Stdlib().ExecContext(ctx, `
        WITH candidates AS (
            SELECT id FROM invoices
            WHERE status='payment_in_progress'
              AND (totals->>'pay_started_at') IS NOT NULL
              AND (totals->>'pay_started_at') ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T'
              AND (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') > ((totals->>'pay_started_at')::timestamptz + make_interval(mins => $1))
        )
        UPDATE invoices
        SET status='failed', updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')
        WHERE id IN (SELECT id FROM candidates)
    `, ttl)
	if err != nil {
		logger.LogError(ctx, err, "cleanup expired payment sessions failed", nil)
		return nil, &errs.Error{Code: errs.Internal, Message: "تعذر تحديث الفواتير المنتهية"}
	}
	failedCount := 0
	if invRes != nil {
		if n, e := invRes.RowsAffected(); e == nil {
			failedCount = int(n)
		}
	}

	_ = nowUTC // reserved for potential future logging/metrics

	return &CleanupResponse{FailedInvoices: failedCount, ProductsReleased: 0}, nil
}
