package adminsettings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"encore.app/pkg/audit"
	"encore.app/pkg/config"
	"encore.app/pkg/errs"
	"encore.app/pkg/logger"
	"encore.app/svc/users"
	"encore.dev/beta/auth"
	"encore.dev/storage/sqldb"
)

var db = sqldb.Named("coredb")

//encore:service
type Service struct {
	cfg *config.ConfigManager
}

//encore:api auth method=POST path=/admin/cities
func (s *Service) CreateCity(ctx context.Context, req *CreateCityRequest) (*ListCitiesResponse, error) {
    if _, ok := auth.UserID(); !ok {
        return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
    }
    if !isAdmin() {
        return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
    }
    if req == nil || strings.TrimSpace(req.NameAr) == "" || strings.TrimSpace(req.ShippingFeeNet) == "" {
        return nil, errs.New(errs.InvalidArgument, "الاسم العربي وقيمة الشحن مطلوبة")
    }
    // Basic numeric validation for shipping fee
    if _, err := strconv.ParseFloat(strings.TrimSpace(req.ShippingFeeNet), 64); err != nil {
        return nil, errs.New(errs.ValidationFailed, "قيمة رسوم الشحن غير صالحة")
    }

    nameEn := ""
    if req.NameEn != nil {
        nameEn = strings.TrimSpace(*req.NameEn)
    }

    // Insert city
    if _, err := db.Stdlib().ExecContext(ctx,
        `INSERT INTO cities (name_ar, name_en, shipping_fee_net, enabled) VALUES ($1, $2, $3, $4)`,
        strings.TrimSpace(req.NameAr), nameEn, strings.TrimSpace(req.ShippingFeeNet), req.Enabled,
    ); err != nil {
        return nil, errs.New(errs.Internal, "فشل إنشاء المدينة")
    }

    return s.ListCities(ctx)
}

func initService() (*Service, error) {
	// Initialize global config manager (safe to call once via sync.Once)
	cfg := config.Initialize(db, 30*time.Second)
	return &Service{cfg: cfg}, nil
}

// isAdmin يتحقق من أن بيانات المصادقة تحمل دور "admin"
func isAdmin() bool {
	if d := auth.Data(); d != nil {
		// Case 1: map[string]any
		if m, ok := d.(map[string]interface{}); ok {
			if role, ok := m["role"].(string); ok {
				return role == "admin"
			}
		}
		// Case 2: struct with field Role (via reflection)
		rv := reflect.ValueOf(d)
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		if rv.IsValid() && rv.Kind() == reflect.Struct {
			f := rv.FieldByName("Role")
			if f.IsValid() && f.Kind() == reflect.String {
				return f.String() == "admin"
			}
		}
	}
	return false
}

type RawSetting struct {
	Key           string   `json:"key"`
	Value         *string  `json:"value,omitempty"`
	Description   *string  `json:"description,omitempty"`
	AllowedValues []string `json:"allowed_values,omitempty"`
	UpdatedAt     string   `json:"updated_at"`
}

type ListRawSettingsResponse struct {
	Items []RawSetting `json:"items"`
}

//encore:api auth method=GET path=/admin/system-settings
func (s *Service) ListRawSettings(ctx context.Context) (*ListRawSettingsResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	rows, err := db.Stdlib().QueryContext(ctx, `
		SELECT key, value, description,
		       COALESCE(TO_JSON(allowed_values)::text, 'null') AS allowed_json,
		       to_char(updated_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM system_settings
		ORDER BY key
	`)
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل الاستعلام عن الإعدادات")
	}
	defer rows.Close()

	var items []RawSetting
	for rows.Next() {
		var key string
		var val sql.NullString
		var desc sql.NullString
		var allowedJSON sql.NullString
		var updated string
		if err := rows.Scan(&key, &val, &desc, &allowedJSON, &updated); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف الإعداد")
		}
		var allowed []string
		if allowedJSON.Valid && allowedJSON.String != "null" {
			_ = json.Unmarshal([]byte(allowedJSON.String), &allowed)
		}
		var valuePtr *string
		if val.Valid {
			tmp := val.String
			valuePtr = &tmp
		}
		var descPtr *string
		if desc.Valid {
			tmp := desc.String
			descPtr = &tmp
		}
		items = append(items, RawSetting{
			Key:           key,
			Value:         valuePtr,
			Description:   descPtr,
			AllowedValues: allowed,
			UpdatedAt:     updated,
		})
	}
	return &ListRawSettingsResponse{Items: items}, nil
}

type RuntimeSettingsResponse struct {
	Settings *config.SystemSettings `json:"settings"`
}

//encore:api auth method=GET path=/admin/system-settings/runtime
func (s *Service) GetRuntimeSettings(ctx context.Context) (*RuntimeSettingsResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	return &RuntimeSettingsResponse{Settings: s.cfg.GetSettings()}, nil
}

type GetKeyRequest struct {
	Key string `query:"key"`
}

type GetKeyResponse struct {
	Item RawSetting `json:"item"`
}

//encore:api auth method=GET path=/admin/system-settings/get
func (s *Service) GetKey(ctx context.Context, req *GetKeyRequest) (*GetKeyResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	if req == nil || req.Key == "" {
		return nil, errs.New(errs.InvalidArgument, "المفتاح مطلوب")
	}

	row := db.Stdlib().QueryRowContext(ctx, `
		SELECT key, value, description,
		       COALESCE(TO_JSON(allowed_values)::text, 'null') AS allowed_json,
		       to_char(updated_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM system_settings WHERE key=$1
	`, req.Key)

	var key string
	var val sql.NullString
	var desc sql.NullString
	var allowedJSON sql.NullString
	var updated string
	if err := row.Scan(&key, &val, &desc, &allowedJSON, &updated); err != nil {
		if err == sql.ErrNoRows {
			return nil, errs.New(errs.NotFound, "الإعداد غير موجود")
		}
		return nil, errs.New(errs.Internal, "فشل قراءة الإعداد")
	}
	var allowed []string
	if allowedJSON.Valid && allowedJSON.String != "null" {
		_ = json.Unmarshal([]byte(allowedJSON.String), &allowed)
	}
	var valuePtr *string
	if val.Valid {
		tmp := val.String
		valuePtr = &tmp
	}
	var descPtr *string
	if desc.Valid {
		tmp := desc.String
		descPtr = &tmp
	}
	return &GetKeyResponse{Item: RawSetting{
		Key:           key,
		Value:         valuePtr,
		Description:   descPtr,
		AllowedValues: allowed,
		UpdatedAt:     updated,
	}}, nil
}

type UpdateSettingItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type UpdateSettingsRequest struct {
	Items []UpdateSettingItem `json:"items"`
}

type UpdateError struct {
	Key     string `json:"key"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type UpdateSettingsResponse struct {
	Updated int           `json:"updated"`
	Errors  []UpdateError `json:"errors,omitempty"`
}

// ====== History API Types ======
type SettingsHistoryRequest struct {
	Key   string `query:"key"`
	Limit int    `query:"limit"`
}

type SettingsHistoryItem struct {
	Key      string          `json:"key"`
	OldValue *string         `json:"old_value,omitempty"`
	NewValue *string         `json:"new_value,omitempty"`
	ActorID  *int64          `json:"actor_user_id,omitempty"`
	At       string          `json:"at"`
	Meta     json.RawMessage `json:"meta,omitempty"`
}

type SettingsHistoryResponse struct {
	Items []SettingsHistoryItem `json:"items"`
}

//encore:api auth method=PUT path=/admin/system-settings
func (s *Service) UpdateSettings(ctx context.Context, req *UpdateSettingsRequest) (*UpdateSettingsResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	// تحويل معرف المستخدم لاستخدامه في سجل التدقيق
	var actorID *int64
	if id64, err := strconv.ParseInt(string(uidStr), 10, 64); err == nil {
		actorID = &id64
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	if req == nil || len(req.Items) == 0 {
		return nil, errs.New(errs.InvalidArgument, "قائمة التحديثات مطلوبة")
	}

	resp := &UpdateSettingsResponse{}

	for _, it := range req.Items {
		if it.Key == "" {
			resp.Errors = append(resp.Errors, UpdateError{Key: it.Key, Code: errs.InvalidArgument, Message: "مفتاح فارغ"})
			continue
		}
		// Fetch allowed values (if any)
		var allowedJSON sql.NullString
		row := db.Stdlib().QueryRowContext(ctx, `SELECT COALESCE(TO_JSON(allowed_values)::text, 'null') FROM system_settings WHERE key=$1`, it.Key)
		if err := row.Scan(&allowedJSON); err != nil {
			if err == sql.ErrNoRows {
				resp.Errors = append(resp.Errors, UpdateError{Key: it.Key, Code: errs.NotFound, Message: "الإعداد غير موجود"})
				continue
			}
			resp.Errors = append(resp.Errors, UpdateError{Key: it.Key, Code: errs.Internal, Message: "فشل التحقق من الإعداد"})
			continue
		}
		var allowed []string
		if allowedJSON.Valid && allowedJSON.String != "null" {
			_ = json.Unmarshal([]byte(allowedJSON.String), &allowed)
		}
		// Validate against allowed values if present
		if len(allowed) > 0 {
			ok := false
			for _, v := range allowed {
				if v == it.Value {
					ok = true
					break
				}
			}
			if !ok {
				resp.Errors = append(resp.Errors, UpdateError{Key: it.Key, Code: errs.ValidationFailed, Message: "قيمة غير مسموحة"})
				continue
			}
		}

		// JSON array validation for specific keys
		if it.Key == "pay.methods" || it.Key == "cors.allowed_origins" {
			var arr []string
			if err := json.Unmarshal([]byte(it.Value), &arr); err != nil {
				resp.Errors = append(resp.Errors, UpdateError{Key: it.Key, Code: errs.ValidationFailed, Message: "يجب أن تكون قيمة JSON Array صالحة من السلاسل"})
				continue
			}
		}

		// Additional type validations for known numeric keys
		if verr := validateKeyValue(it.Key, it.Value); verr != nil {
			resp.Errors = append(resp.Errors, UpdateError{Key: it.Key, Code: verr.Code, Message: verr.Message})
			continue
		}
		// Capture current value before update for audit (غير مانع في حال الفشل)
		var oldVal sql.NullString
		if err := db.Stdlib().QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key=$1`, it.Key).Scan(&oldVal); err != nil && err != sql.ErrNoRows {
			// غير مانع: نستمر في التحديث حتى لو تعذر جلب القيمة القديمة
		}

		// Perform update (will trigger async reload)
		if err := s.cfg.UpdateSetting(ctx, it.Key, it.Value); err != nil {
			resp.Errors = append(resp.Errors, UpdateError{Key: it.Key, Code: errs.Internal, Message: "فشل تحديث الإعداد"})
			continue
		}

		// Audit logging (non-blocking): سجّل الفرق بين القديم والجديد
		meta := map[string]interface{}{
			"key":       it.Key,
			"new_value": it.Value,
		}
		if oldVal.Valid {
			meta["old_value"] = oldVal.String
		} else {
			meta["old_value"] = nil
		}
		if _, aerr := audit.Log(ctx, db, audit.Entry{
			ActorUserID: actorID,
			Action:      "system_settings.update",
			EntityType:  "system_setting",
			EntityID:    it.Key,
			Meta:        meta,
		}); aerr != nil {
			logger.LogError(ctx, aerr, "failed to write audit log for system setting update", logger.Fields{"key": it.Key})
		}

		resp.Updated++
	}

	return resp, nil
}

//encore:api auth method=GET path=/admin/system-settings/history
func (s *Service) GetSettingsHistory(ctx context.Context, req *SettingsHistoryRequest) (*SettingsHistoryResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	limit := 50
	if req != nil && req.Limit > 0 && req.Limit <= 200 {
		limit = req.Limit
	}

	var rows *sql.Rows
	var err error
	if req != nil && req.Key != "" {
		rows, err = db.Stdlib().QueryContext(ctx, `
			SELECT entity_id AS key,
			       (meta->>'old_value') AS old_value,
			       (meta->>'new_value') AS new_value,
			       actor_user_id,
			       to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as at,
			       meta
			FROM audit_logs
			WHERE action = 'system_settings.update' AND entity_type = 'system_setting' AND entity_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		`, req.Key, limit)
	} else {
		rows, err = db.Stdlib().QueryContext(ctx, `
			SELECT entity_id AS key,
			       (meta->>'old_value') AS old_value,
			       (meta->>'new_value') AS new_value,
			       actor_user_id,
			       to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') as at,
			       meta
			FROM audit_logs
			WHERE action = 'system_settings.update' AND entity_type = 'system_setting'
			ORDER BY created_at DESC
			LIMIT $1
		`, limit)
	}
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل قراءة السجل التدقيقي")
	}
	defer rows.Close()

	items := make([]SettingsHistoryItem, 0, limit)
	for rows.Next() {
		var key string
		var oldVal, newVal sql.NullString
		var actor sql.NullInt64
		var at string
		var metaJSON json.RawMessage
		if err := rows.Scan(&key, &oldVal, &newVal, &actor, &at, &metaJSON); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف السجل")
		}
		var actorPtr *int64
		if actor.Valid {
			v := actor.Int64
			actorPtr = &v
		}
		var oldPtr *string
		if oldVal.Valid {
			v := oldVal.String
			oldPtr = &v
		}
		var newPtr *string
		if newVal.Valid {
			v := newVal.String
			newPtr = &v
		}

		items = append(items, SettingsHistoryItem{
			Key:      key,
			OldValue: oldPtr,
			NewValue: newPtr,
			ActorID:  actorPtr,
			At:       at,
			Meta:     metaJSON,
		})
	}

	return &SettingsHistoryResponse{Items: items}, nil
}

// validateKeyValue يطبّق تحققًا إضافيًا الأنواع/النطاقات للمفاتيح المعروفة
func validateKeyValue(key, value string) *errs.Error {
	// Integer keys (>= 0 or > 0 حسب السياق)
	intKeysGTZero := map[string]bool{
		"ws.max_connections":                 true,
		"ws.heartbeat_interval":              true,
		"ws.max_connections_per_host":        true,
		"ws.msgs_per_minute":                 true,
		"cors.max_age":                       true,
		"security.session_timeout":           true,
		"security.max_login_attempts":        true,
		"security.lockout_duration":          true,
		"auctions.default_duration":          true,
		"auctions.auto_extend_duration":      true,
		"auctions.anti_sniping_minutes":      true,
		"payments.session_ttl_minutes":       true,
		"payments.idempotency_ttl_hours":     true,
		"payments.rate_limit_per_5min":       true,
		"notifications.email.retention_days": true,
		"stock.checkout_hold_minutes":        true,
		"stock.supplies_hold_minutes":        true,
		"stock.max_active_holds_per_user":    true,
		"bids.rate_limit_per_minute":         true,
	}
	intKeysGEZero := map[string]bool{
		"auctions.max_extensions": true,
	}

	// Bounded integer keys per PRD
	bounded := map[string]struct{ Min, Max int }{
		"bids.rate_limit_per_minute":     {Min: 10, Max: 600},
		"payments.session_ttl_minutes":   {Min: 10, Max: 60},
		"payments.idempotency_ttl_hours": {Min: 1, Max: 72},
		"ws.max_connections_per_host":    {Min: 10, Max: 10000},
		"ws.msgs_per_minute":             {Min: 5, Max: 1000},
	}

	if intKeysGTZero[key] || intKeysGEZero[key] {
		i, err := strconv.Atoi(value)
		if err != nil {
			return errs.New(errs.ValidationFailed, "يجب أن تكون قيمة صحيحة")
		}
		if intKeysGTZero[key] && i <= 0 {
			return errs.New(errs.ValidationFailed, "يجب أن تكون أكبر من صفر")
		}
		if intKeysGEZero[key] && i < 0 {
			return errs.New(errs.ValidationFailed, "يجب ألا تكون سالبة")
		}
		if b, ok := bounded[key]; ok {
			if i < b.Min || i > b.Max {
				return errs.New(errs.ValidationFailed, fmt.Sprintf("القيمة يجب أن تكون بين %d و %d", b.Min, b.Max))
			}
		}
		return nil
	}

	// int64 keys
	if key == "media.max_file_size" {
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return errs.New(errs.ValidationFailed, "حجم الملف غير صالح")
		}
		return nil
	}

	// float keys
	switch key {
	case "auctions.min_bid_increment":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return errs.New(errs.ValidationFailed, "قيمة رقمية غير صالحة")
		}
		return nil
	case "vat.rate":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return errs.New(errs.ValidationFailed, "قيمة رقمية غير صالحة")
		}
		if f < 0 || f > 0.25 {
			return errs.New(errs.ValidationFailed, "النسبة يجب أن تكون بين 0 و 0.25")
		}
		return nil
	case "shipping.free_shipping_threshold":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return errs.New(errs.ValidationFailed, "قيمة رقمية غير صالحة")
		}
		return nil
	case "media.watermark.opacity":
		i, err := strconv.Atoi(value)
		if err != nil {
			return errs.New(errs.ValidationFailed, "يجب أن تكون قيمة صحيحة")
		}
		if i < 0 || i > 100 {
			return errs.New(errs.ValidationFailed, "الشفافية يجب أن تكون بين 0 و 100")
		}
		return nil
	}

	// Boolean and string/slice keys إما مقيّدة عبر allowed_values أو لا تحتاج تحقق إضافي
	return nil
}

// ====== Admin Dashboard Stats ======

type AdminDashboardStats struct {
	UsersTotal           int64  `json:"users_total"`
	UsersVerifiedOrAdmin int64  `json:"users_verified_or_admin"`
	ProductsTotal        int64  `json:"products_total"`
	PigeonsTotal         int64  `json:"pigeons_total"`
	SuppliesTotal        int64  `json:"supplies_total"`
	AuctionsLive         int64  `json:"auctions_live"`
	AuctionsScheduled    int64  `json:"auctions_scheduled"`
	AuctionsEnded        int64  `json:"auctions_ended"`
	AuctionsCancelled    int64  `json:"auctions_cancelled"`
	AuctionsWinnerUnpaid int64  `json:"auctions_winner_unpaid"`
	OrdersPendingPayment int64  `json:"orders_pending_payment"`
	OrdersPaid           int64  `json:"orders_paid"`
	OrdersAwaitingRefund int64  `json:"orders_awaiting_admin_refund"`
	OrdersRefundRequired int64  `json:"orders_refund_required"`
	OrdersRefunded       int64  `json:"orders_refunded"`
	InvoicesUnpaid       int64  `json:"invoices_unpaid"`
	InvoicesInProgress   int64  `json:"invoices_payment_in_progress"`
	InvoicesPaid         int64  `json:"invoices_paid"`
	InvoicesFailed       int64  `json:"invoices_failed"`
	PaymentsPending      int64  `json:"payments_pending"`
	PaymentsPaid         int64  `json:"payments_paid"`
	PaymentsFailed       int64  `json:"payments_failed"`
	PaymentsRefunded     int64  `json:"payments_refunded"`
	GeneratedAt          string `json:"generated_at"`
}

//encore:api auth method=GET path=/admin/dashboard/stats
func (s *Service) GetAdminDashboardStats(ctx context.Context) (*AdminDashboardStats, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	query := `
		SELECT
			(SELECT COUNT(*) FROM users) AS users_total,
			(SELECT COUNT(*) FROM users WHERE role IN ('verified','admin')) AS users_verified,
			(SELECT COUNT(*) FROM products) AS products_total,
			(SELECT COUNT(*) FROM products WHERE type='pigeon') AS pigeons_total,
			(SELECT COUNT(*) FROM products WHERE type='supply') AS supplies_total,
			(SELECT COUNT(*) FROM auctions WHERE status='live') AS auctions_live,
			(SELECT COUNT(*) FROM auctions WHERE status='scheduled') AS auctions_scheduled,
			(SELECT COUNT(*) FROM auctions WHERE status='ended') AS auctions_ended,
			(SELECT COUNT(*) FROM auctions WHERE status='cancelled') AS auctions_cancelled,
			(SELECT COUNT(*) FROM auctions WHERE status='winner_unpaid') AS auctions_winner_unpaid,
			(SELECT COUNT(*) FROM orders WHERE status='pending_payment') AS orders_pending_payment,
			(SELECT COUNT(*) FROM orders WHERE status='paid') AS orders_paid,
			(SELECT COUNT(*) FROM orders WHERE status='awaiting_admin_refund') AS orders_awaiting_admin_refund,
			(SELECT COUNT(*) FROM orders WHERE status='refund_required') AS orders_refund_required,
			(SELECT COUNT(*) FROM orders WHERE status='refunded') AS orders_refunded,
			(SELECT COUNT(*) FROM invoices WHERE status='unpaid') AS invoices_unpaid,
			(SELECT COUNT(*) FROM invoices WHERE status='payment_in_progress') AS invoices_in_progress,
			(SELECT COUNT(*) FROM invoices WHERE status='paid') AS invoices_paid,
			(SELECT COUNT(*) FROM invoices WHERE status='failed') AS invoices_failed,
			(SELECT COUNT(*) FROM payments WHERE status='pending') AS payments_pending,
			(SELECT COUNT(*) FROM payments WHERE status='paid') AS payments_paid,
			(SELECT COUNT(*) FROM payments WHERE status='failed') AS payments_failed,
			(SELECT COUNT(*) FROM payments WHERE status='refunded') AS payments_refunded
	`

	var st AdminDashboardStats
	row := db.Stdlib().QueryRowContext(ctx, query)
	if err := row.Scan(
		&st.UsersTotal,
		&st.UsersVerifiedOrAdmin,
		&st.ProductsTotal,
		&st.PigeonsTotal,
		&st.SuppliesTotal,
		&st.AuctionsLive,
		&st.AuctionsScheduled,
		&st.AuctionsEnded,
		&st.AuctionsCancelled,
		&st.AuctionsWinnerUnpaid,
		&st.OrdersPendingPayment,
		&st.OrdersPaid,
		&st.OrdersAwaitingRefund,
		&st.OrdersRefundRequired,
		&st.OrdersRefunded,
		&st.InvoicesUnpaid,
		&st.InvoicesInProgress,
		&st.InvoicesPaid,
		&st.InvoicesFailed,
		&st.PaymentsPending,
		&st.PaymentsPaid,
		&st.PaymentsFailed,
		&st.PaymentsRefunded,
	); err != nil {
		return nil, errs.New(errs.Internal, "فشل جلب إحصائيات لوحة الإدارة")
	}

	st.GeneratedAt = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	return &st, nil
}

// ====== Admin: Verification Requests ======

type ListVerificationRequestsRequest struct {
	Status string `query:"status"` // pending|approved|rejected (optional)
	Limit  int    `query:"limit"`
	Page   int    `query:"page"`
}

type VerificationRequestItem struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	UserName   string  `json:"user_name"`
	UserEmail  string  `json:"user_email"`
	UserPhone  string  `json:"user_phone"`
	Status     string  `json:"status"`
	Note       *string `json:"note,omitempty"`
	ReviewedBy *int64  `json:"reviewed_by,omitempty"`
	ReviewedAt *string `json:"reviewed_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

type ListVerificationRequestsResponse struct {
	Items []VerificationRequestItem `json:"items"`
	Total int64                     `json:"total"`
	Page  int                       `json:"page"`
	Limit int                       `json:"limit"`
}

//encore:api auth method=GET path=/admin/verification/requests
func (s *Service) ListVerificationRequests(ctx context.Context, req *ListVerificationRequestsRequest) (*ListVerificationRequestsResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	limit := 50
	page := 1
	if req != nil {
		if req.Limit > 0 && req.Limit <= 200 {
			limit = req.Limit
		}
		if req.Page > 0 {
			page = req.Page
		}
	}
	offset := (page - 1) * limit

	base := "SELECT vr.id, vr.user_id, u.name, u.email, u.phone, vr.status::text, vr.note, vr.reviewed_by, to_char(vr.reviewed_at at time zone 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"'), to_char(vr.created_at at time zone 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') FROM verification_requests vr LEFT JOIN users u ON vr.user_id = u.id"
	where := ""
	args := []interface{}{}
	if req != nil && req.Status != "" {
		where = " WHERE vr.status = $1"
		args = append(args, req.Status)
	}
	order := " ORDER BY vr.created_at DESC"
	pag := fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := db.Stdlib().QueryContext(ctx, base+where+order+pag, args...)
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل قراءة طلبات التوثيق")
	}
	defer rows.Close()

	items := make([]VerificationRequestItem, 0, limit)
	for rows.Next() {
		var it VerificationRequestItem
		var status string
		var note sql.NullString
		var reviewedBy sql.NullInt64
		var reviewedAt sql.NullString
		var created string
		var userName, userEmail, userPhone sql.NullString
		if err := rows.Scan(&it.ID, &it.UserID, &userName, &userEmail, &userPhone, &status, &note, &reviewedBy, &reviewedAt, &created); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف")
		}
		it.Status = status
		it.UserName = userName.String
		it.UserEmail = userEmail.String
		it.UserPhone = userPhone.String
		if note.Valid {
			v := note.String
			it.Note = &v
		}
		if reviewedBy.Valid {
			v := reviewedBy.Int64
			it.ReviewedBy = &v
		}
		if reviewedAt.Valid {
			v := reviewedAt.String
			it.ReviewedAt = &v
		}
		it.CreatedAt = created
		items = append(items, it)
	}

	// Count total
	var total int64
	countQuery := "SELECT COUNT(*) FROM verification_requests vr" + strings.Replace(where, " WHERE vr.", " WHERE ", 1)
	if err := db.Stdlib().QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		total = int64(len(items))
	}

	return &ListVerificationRequestsResponse{Items: items, Total: total, Page: page, Limit: limit}, nil
}

// ====== Admin: Approve/Reject Verification via users service ======

//encore:api auth method=POST path=/admin/verification/:id/approve
func (s *Service) AdminApproveVerification(ctx context.Context, id int64, req *users.ReviewVerificationRequest) (*users.ReviewVerificationResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	return users.ApproveVerificationRequest(ctx, id, req)
}

//encore:api auth method=POST path=/admin/verification/:id/reject
func (s *Service) AdminRejectVerification(ctx context.Context, id int64, req *users.ReviewVerificationRequest) (*users.ReviewVerificationResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	return users.RejectVerificationRequest(ctx, id, req)
}

// ====== Admin: Users List ======

type ListUsersRequest struct {
	Q     string `query:"q"`
	Role  string `query:"role"`  // registered|verified|admin
	State string `query:"state"` // active|inactive
	Page  int    `query:"page"`
	Limit int    `query:"limit"`
}

type AdminUserItem struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Email           string  `json:"email"`
	Phone           string  `json:"phone"`
	CityID          *int64  `json:"city_id,omitempty"`
	Role            string  `json:"role"`
	State           string  `json:"state"`
	EmailVerifiedAt *string `json:"email_verified_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

type ListUsersResponse struct {
	Items []AdminUserItem `json:"items"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Limit int             `json:"limit"`
}

//encore:api auth method=GET path=/admin/users
func (s *Service) ListUsers(ctx context.Context, req *ListUsersRequest) (*ListUsersResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	limit := 50
	page := 1
	if req != nil {
		if req.Limit > 0 && req.Limit <= 200 {
			limit = req.Limit
		}
		if req.Page > 0 {
			page = req.Page
		}
	}
	offset := (page - 1) * limit

	base := `SELECT id, COALESCE(name,''), COALESCE(email,''), COALESCE(phone,''), city_id,
                     role::text, state::text,
                     to_char(email_verified_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') AS ev_at,
                     to_char(created_at         at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at
              FROM users`

	var whereParts []string
	var args []interface{}
	addArg := func(v interface{}) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }

	if req != nil {
		if strings.TrimSpace(req.Q) != "" {
			p := "%" + strings.ToLower(strings.TrimSpace(req.Q)) + "%"
			whereParts = append(whereParts, fmt.Sprintf("(LOWER(name) ILIKE %s OR LOWER(email) ILIKE %s)", addArg(p), addArg(p)))
		}
		if strings.TrimSpace(req.Role) != "" {
			whereParts = append(whereParts, fmt.Sprintf("role = %s", addArg(req.Role)))
		}
		if strings.TrimSpace(req.State) != "" {
			whereParts = append(whereParts, fmt.Sprintf("state = %s", addArg(req.State)))
		}
	}

	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	order := " ORDER BY created_at DESC"
	pag := fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := db.Stdlib().QueryContext(ctx, base+where+order+pag, args...)
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل قراءة المستخدمين")
	}
	defer rows.Close()

	items := make([]AdminUserItem, 0, limit)
	for rows.Next() {
		var it AdminUserItem
		var cityID sql.NullInt64
		var evAt sql.NullString
		if err := rows.Scan(&it.ID, &it.Name, &it.Email, &it.Phone, &cityID, &it.Role, &it.State, &evAt, &it.CreatedAt); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف مستخدم")
		}
		if cityID.Valid {
			v := cityID.Int64
			it.CityID = &v
		}
		if evAt.Valid {
			v := evAt.String
			it.EmailVerifiedAt = &v
		}
		items = append(items, it)
	}

	// Total count
	countQuery := "SELECT COUNT(*) FROM users" + where
	var total int64
	if err := db.Stdlib().QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		total = int64(len(items))
	}

	return &ListUsersResponse{Items: items, Total: total, Page: page, Limit: limit}, nil
}

// ====== Admin: Update User Role/State ======

type UpdateUserRoleRequest struct {
	Role   string  `json:"role"` // registered|verified|admin
	Reason *string `json:"reason,omitempty"`
}

type UpdateUserStateRequest struct {
	State  string  `json:"state"` // active|inactive
	Reason *string `json:"reason,omitempty"`
}

//encore:api auth method=POST path=/admin/users/:id/role
func (s *Service) UpdateUserRole(ctx context.Context, id int64, req *UpdateUserRoleRequest) (*AdminUserItem, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	if req == nil || strings.TrimSpace(req.Role) == "" {
		return nil, errs.New(errs.InvalidArgument, "الدور مطلوب")
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "registered" && role != "verified" && role != "admin" {
		return nil, errs.New(errs.ValidationFailed, "دور غير مسموح")
	}

	// Update and return user
	q := `UPDATE users SET role=$1, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$2`
	if _, err := db.Stdlib().ExecContext(ctx, q, role, id); err != nil {
		return nil, errs.New(errs.Internal, "فشل تحديث الدور")
	}

	// Audit
	meta := map[string]interface{}{"user_id": id, "new_role": role}
	if req.Reason != nil {
		meta["reason"] = *req.Reason
	}
	_, _ = audit.Log(ctx, db, audit.Entry{
		Action:     "admin.user.role.update",
		EntityType: "user",
		EntityID:   fmt.Sprintf("%d", id),
		Meta:       meta,
	})

	// Read back
	item, err := fetchAdminUserItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return item, nil
}

//encore:api auth method=POST path=/admin/users/:id/state
func (s *Service) UpdateUserState(ctx context.Context, id int64, req *UpdateUserStateRequest) (*AdminUserItem, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	if req == nil || strings.TrimSpace(req.State) == "" {
		return nil, errs.New(errs.InvalidArgument, "الحالة مطلوبة")
	}
	state := strings.ToLower(strings.TrimSpace(req.State))
	if state != "active" && state != "inactive" {
		return nil, errs.New(errs.ValidationFailed, "حالة غير مسموحة")
	}

	q := `UPDATE users SET state=$1, updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC') WHERE id=$2`
	if _, err := db.Stdlib().ExecContext(ctx, q, state, id); err != nil {
		return nil, errs.New(errs.Internal, "فشل تحديث الحالة")
	}

	// Audit
	meta := map[string]interface{}{"user_id": id, "new_state": state}
	if req.Reason != nil {
		meta["reason"] = *req.Reason
	}
	_, _ = audit.Log(ctx, db, audit.Entry{
		Action:     "admin.user.state.update",
		EntityType: "user",
		EntityID:   fmt.Sprintf("%d", id),
		Meta:       meta,
	})

	item, err := fetchAdminUserItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func fetchAdminUserItem(ctx context.Context, id int64) (*AdminUserItem, error) {
	row := db.Stdlib().QueryRowContext(ctx, `
        SELECT id, COALESCE(name,''), COALESCE(email,''), COALESCE(phone,''), city_id,
               role::text, state::text,
               to_char(email_verified_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') AS ev_at,
               to_char(created_at         at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at
        FROM users WHERE id=$1
    `, id)
	var it AdminUserItem
	var cityID sql.NullInt64
	var evAt sql.NullString
	if err := row.Scan(&it.ID, &it.Name, &it.Email, &it.Phone, &cityID, &it.Role, &it.State, &evAt, &it.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, errs.New(errs.NotFound, "المستخدم غير موجود")
		}
		return nil, errs.New(errs.Internal, "فشل قراءة المستخدم")
	}
	if cityID.Valid {
		v := cityID.Int64
		it.CityID = &v
	}
	if evAt.Valid {
		v := evAt.String
		it.EmailVerifiedAt = &v
	}
	return &it, nil
}

// ====== Admin: Products List ======

type ListProductsRequest struct {
	Q      string `query:"q"`
	Type   string `query:"type"`   // pigeon|supply
	Status string `query:"status"` // per domain
	Page   int    `query:"page"`
	Limit  int    `query:"limit"`
}

type AdminProductItem struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	PriceNet  string `json:"price_net"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type ListProductsResponse struct {
	Items []AdminProductItem `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

//encore:api auth method=GET path=/admin/products
func (s *Service) ListProducts(ctx context.Context, req *ListProductsRequest) (*ListProductsResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	limit := 50
	page := 1
	if req != nil {
		if req.Limit > 0 && req.Limit <= 200 {
			limit = req.Limit
		}
		if req.Page > 0 {
			page = req.Page
		}
	}
	offset := (page - 1) * limit

	base := `SELECT id, type::text, COALESCE(title,''), COALESCE(slug,''), price_net::text, status::text,
                     to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at
              FROM products`

	var whereParts []string
	var args []interface{}
	addArg := func(v interface{}) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }

	if req != nil {
		if strings.TrimSpace(req.Q) != "" {
			p := "%" + strings.ToLower(strings.TrimSpace(req.Q)) + "%"
			whereParts = append(whereParts, fmt.Sprintf("(LOWER(title) ILIKE %s OR LOWER(slug) ILIKE %s)", addArg(p), addArg(p)))
		}
		if strings.TrimSpace(req.Type) != "" {
			whereParts = append(whereParts, fmt.Sprintf("type = %s", addArg(req.Type)))
		}
		if strings.TrimSpace(req.Status) != "" {
			whereParts = append(whereParts, fmt.Sprintf("status = %s", addArg(req.Status)))
		}
	}

	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	order := " ORDER BY created_at DESC"
	pag := fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := db.Stdlib().QueryContext(ctx, base+where+order+pag, args...)
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل قراءة المنتجات")
	}
	defer rows.Close()

	items := make([]AdminProductItem, 0, limit)
	for rows.Next() {
		var it AdminProductItem
		if err := rows.Scan(&it.ID, &it.Type, &it.Title, &it.Slug, &it.PriceNet, &it.Status, &it.CreatedAt); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف منتج")
		}
		items = append(items, it)
	}

	var total int64
	countQuery := "SELECT COUNT(*) FROM products" + where
	if err := db.Stdlib().QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		total = int64(len(items))
	}
	return &ListProductsResponse{Items: items, Total: total, Page: page, Limit: limit}, nil
}

// ====== Admin: Auctions List ======

type ListAuctionsRequest struct {
	Status string `query:"status"` // draft|scheduled|live|ended|cancelled|winner_unpaid
	Q      string `query:"q"`      // by product title
	Page   int    `query:"page"`
	Limit  int    `query:"limit"`
}

type AdminAuctionItem struct {
	ID                    int64   `json:"id"`
	ProductID             int64   `json:"product_id"`
	Title                 string  `json:"title"`
	Status                string  `json:"status"`
	StartPrice            string  `json:"start_price"`
	BidStep               int     `json:"bid_step"`
	ReservePrice          *string `json:"reserve_price,omitempty"`
	CurrentPrice          *string `json:"current_price,omitempty"`
	BidsCount             int     `json:"bids_count"`
	AntiSnipingMinutes    int     `json:"anti_sniping_minutes"`
	ExtensionsCount       int     `json:"extensions_count"`
	MaxExtensionsOverride *int    `json:"max_extensions_override,omitempty"`
	WinnerUserID          *int64  `json:"winner_user_id,omitempty"`
	StartAt               string  `json:"start_at"`
	EndAt                 string  `json:"end_at"`
}

type ListAuctionsResponse struct {
	Items []AdminAuctionItem `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

//encore:api auth method=GET path=/admin/auctions
func (s *Service) ListAuctions(ctx context.Context, req *ListAuctionsRequest) (*ListAuctionsResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	limit := 50
	page := 1
	if req != nil {
		if req.Limit > 0 && req.Limit <= 200 {
			limit = req.Limit
		}
		if req.Page > 0 {
			page = req.Page
		}
	}
	offset := (page - 1) * limit

	base := `SELECT 
		a.id, 
		a.product_id, 
		COALESCE(p.title,'') as title, 
		a.status::text,
		a.start_price::text,
		a.bid_step,
		a.reserve_price::text,
		(SELECT b.amount::text FROM bids b WHERE b.auction_id = a.id ORDER BY b.created_at DESC LIMIT 1) as current_price,
		(SELECT COUNT(*) FROM bids b WHERE b.auction_id = a.id)::int as bids_count,
		a.anti_sniping_minutes,
		a.extensions_count,
		a.max_extensions_override,
		CASE WHEN a.status IN ('ended','winner_unpaid') 
			THEN (SELECT b.user_id FROM bids b WHERE b.auction_id = a.id ORDER BY b.created_at DESC LIMIT 1)
			ELSE NULL 
		END as winner_user_id,
		to_char(a.start_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		to_char(a.end_at   at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
	FROM auctions a JOIN products p ON p.id = a.product_id`

	var whereParts []string
	var args []interface{}
	addArg := func(v interface{}) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	if req != nil {
		if strings.TrimSpace(req.Status) != "" {
			whereParts = append(whereParts, fmt.Sprintf("a.status = %s", addArg(req.Status)))
		}
		if strings.TrimSpace(req.Q) != "" {
			p := "%" + strings.ToLower(strings.TrimSpace(req.Q)) + "%"
			whereParts = append(whereParts, fmt.Sprintf("LOWER(p.title) ILIKE %s", addArg(p)))
		}
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	order := " ORDER BY a.created_at DESC"
	pag := fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := db.Stdlib().QueryContext(ctx, base+where+order+pag, args...)
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل قراءة المزادات")
	}
	defer rows.Close()

	items := make([]AdminAuctionItem, 0, limit)
	for rows.Next() {
		var it AdminAuctionItem
		var reservePrice sql.NullString
		var currentPrice sql.NullString
		var maxExt sql.NullInt64
		var winnerID sql.NullInt64
		
		if err := rows.Scan(
			&it.ID, 
			&it.ProductID, 
			&it.Title, 
			&it.Status,
			&it.StartPrice,
			&it.BidStep,
			&reservePrice,
			&currentPrice,
			&it.BidsCount,
			&it.AntiSnipingMinutes,
			&it.ExtensionsCount,
			&maxExt,
			&winnerID,
			&it.StartAt, 
			&it.EndAt,
		); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف مزاد")
		}
		
		if reservePrice.Valid {
			it.ReservePrice = &reservePrice.String
		}
		if currentPrice.Valid {
			it.CurrentPrice = &currentPrice.String
		}
		if maxExt.Valid {
			v := int(maxExt.Int64)
			it.MaxExtensionsOverride = &v
		}
		if winnerID.Valid {
			it.WinnerUserID = &winnerID.Int64
		}
		
		items = append(items, it)
	}

	var total int64
	countQuery := "SELECT COUNT(*) FROM auctions a JOIN products p ON p.id=a.product_id" + where
	if err := db.Stdlib().QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		total = int64(len(items))
	}
	return &ListAuctionsResponse{Items: items, Total: total, Page: page, Limit: limit}, nil
}

// ====== Admin: Orders List ======

type ListOrdersRequest struct {
	Status string `query:"status"` // pending_payment|paid|cancelled|awaiting_admin_refund|refund_required|refunded
	UserID int64  `query:"user_id"`
	Source string `query:"source"` // auction|direct
	Page   int    `query:"page"`
	Limit  int    `query:"limit"`
}

type AdminOrderItem struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Source     string `json:"source"`
	Status     string `json:"status"`
	GrandTotal string `json:"grand_total"`
	CreatedAt  string `json:"created_at"`
}

type ListOrdersResponse struct {
	Items []AdminOrderItem `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Limit int              `json:"limit"`
}

//encore:api auth method=GET path=/admin/orders
func (s *Service) ListOrders(ctx context.Context, req *ListOrdersRequest) (*ListOrdersResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	limit := 50
	page := 1
	if req != nil {
		if req.Limit > 0 && req.Limit <= 200 {
			limit = req.Limit
		}
		if req.Page > 0 {
			page = req.Page
		}
	}
	offset := (page - 1) * limit

	base := `SELECT id, user_id, source::text, status::text, grand_total::text,
                    to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
             FROM orders`
	var whereParts []string
	var args []interface{}
	addArg := func(v interface{}) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	if req != nil {
		if strings.TrimSpace(req.Status) != "" {
			whereParts = append(whereParts, fmt.Sprintf("status = %s", addArg(req.Status)))
		}
		if req.UserID > 0 {
			whereParts = append(whereParts, fmt.Sprintf("user_id = %s", addArg(req.UserID)))
		}
		if strings.TrimSpace(req.Source) != "" {
			whereParts = append(whereParts, fmt.Sprintf("source = %s", addArg(req.Source)))
		}
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	order := " ORDER BY created_at DESC"
	pag := fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := db.Stdlib().QueryContext(ctx, base+where+order+pag, args...)
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل قراءة الطلبات")
	}
	defer rows.Close()

	items := make([]AdminOrderItem, 0, limit)
	for rows.Next() {
		var it AdminOrderItem
		if err := rows.Scan(&it.ID, &it.UserID, &it.Source, &it.Status, &it.GrandTotal, &it.CreatedAt); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف طلب")
		}
		items = append(items, it)
	}
	var total int64
	countQuery := "SELECT COUNT(*) FROM orders" + where
	if err := db.Stdlib().QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		total = int64(len(items))
	}
	return &ListOrdersResponse{Items: items, Total: total, Page: page, Limit: limit}, nil
}

// ====== Admin: Invoices List ======

type ListInvoicesRequest struct {
	Status  string `query:"status"` // unpaid|payment_in_progress|paid|failed|refund_required|refunded|cancelled|void
	OrderID int64  `query:"order_id"`
	Page    int    `query:"page"`
	Limit   int    `query:"limit"`
}

type AdminInvoiceItem struct {
	ID        int64  `json:"id"`
	OrderID   int64  `json:"order_id"`
	Number    string `json:"number"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type ListInvoicesResponse struct {
	Items []AdminInvoiceItem `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

//encore:api auth method=GET path=/admin/invoices
func (s *Service) ListInvoices(ctx context.Context, req *ListInvoicesRequest) (*ListInvoicesResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	limit := 50
	page := 1
	if req != nil {
		if req.Limit > 0 && req.Limit <= 200 {
			limit = req.Limit
		}
		if req.Page > 0 {
			page = req.Page
		}
	}
	offset := (page - 1) * limit

	base := `SELECT id, order_id, COALESCE(number,''), status::text,
                     to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
              FROM invoices`
	var whereParts []string
	var args []interface{}
	addArg := func(v interface{}) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	if req != nil {
		if strings.TrimSpace(req.Status) != "" {
			whereParts = append(whereParts, fmt.Sprintf("status = %s", addArg(req.Status)))
		}
		if req.OrderID > 0 {
			whereParts = append(whereParts, fmt.Sprintf("order_id = %s", addArg(req.OrderID)))
		}
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	order := " ORDER BY created_at DESC"
	pag := fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := db.Stdlib().QueryContext(ctx, base+where+order+pag, args...)
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل قراءة الفواتير")
	}
	defer rows.Close()

	items := make([]AdminInvoiceItem, 0, limit)
	for rows.Next() {
		var it AdminInvoiceItem
		if err := rows.Scan(&it.ID, &it.OrderID, &it.Number, &it.Status, &it.CreatedAt); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف فاتورة")
		}
		items = append(items, it)
	}
	var total int64
	countQuery := "SELECT COUNT(*) FROM invoices" + where
	if err := db.Stdlib().QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		total = int64(len(items))
	}
	return &ListInvoicesResponse{Items: items, Total: total, Page: page, Limit: limit}, nil
}

// ====== Admin: Shipments & Cities ======

type ListShipmentsRequest struct {
	Status    string `query:"status"`
	OrderID   int64  `query:"order_id"`
	CompanyID int64  `query:"company_id"`
	Page      int    `query:"page"`
	Limit     int    `query:"limit"`
}

type AdminShipmentItem struct {
	ID             int64   `json:"id"`
	OrderID        int64   `json:"order_id"`
	CompanyID      int64   `json:"company_id"`
	DeliveryMethod string  `json:"delivery_method"`
	Status         string  `json:"status"`
	TrackingRef    *string `json:"tracking_ref,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

type ListShipmentsResponse struct {
	Items []AdminShipmentItem `json:"items"`
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
	Limit int                 `json:"limit"`
}

type CreateShipmentRequest struct {
	OrderID        int64   `json:"order_id"`
	DeliveryMethod string  `json:"delivery_method"`           // courier | pickup
	CompanyID      *int64  `json:"company_id,omitempty"`
	TrackingRef    *string `json:"tracking_ref,omitempty"`
}

//encore:api auth method=GET path=/admin/shipments
func (s *Service) ListShipments(ctx context.Context, req *ListShipmentsRequest) (*ListShipmentsResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	limit := 50
	page := 1
	if req != nil {
		if req.Limit > 0 && req.Limit <= 200 {
			limit = req.Limit
		}
		if req.Page > 0 {
			page = req.Page
		}
	}
	offset := (page - 1) * limit

	base := `SELECT id, order_id, company_id, delivery_method::text, status::text, tracking_ref,
                     to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
              FROM shipments`
	var whereParts []string
	var args []interface{}
	addArg := func(v interface{}) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	if req != nil {
		if strings.TrimSpace(req.Status) != "" {
			whereParts = append(whereParts, fmt.Sprintf("status = %s", addArg(req.Status)))
		}
		if req.OrderID > 0 {
			whereParts = append(whereParts, fmt.Sprintf("order_id = %s", addArg(req.OrderID)))
		}
		if req.CompanyID > 0 {
			whereParts = append(whereParts, fmt.Sprintf("company_id = %s", addArg(req.CompanyID)))
		}
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	order := " ORDER BY created_at DESC"
	pag := fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := db.Stdlib().QueryContext(ctx, base+where+order+pag, args...)
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل قراءة الشحنات")
	}
	defer rows.Close()

	items := make([]AdminShipmentItem, 0, limit)
	for rows.Next() {
		var it AdminShipmentItem
		var tr sql.NullString
		if err := rows.Scan(&it.ID, &it.OrderID, &it.CompanyID, &it.DeliveryMethod, &it.Status, &tr, &it.CreatedAt); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف شحنة")
		}
		if tr.Valid {
			v := tr.String
			it.TrackingRef = &v
		}
		items = append(items, it)
	}
	var total int64
	countQuery := "SELECT COUNT(*) FROM shipments" + where
	if err := db.Stdlib().QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		total = int64(len(items))
	}
	return &ListShipmentsResponse{Items: items, Total: total, Page: page, Limit: limit}, nil
}

//encore:api auth method=POST path=/admin/shipments
func (s *Service) CreateShipment(ctx context.Context, req *CreateShipmentRequest) (*AdminShipmentItem, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	if req == nil || req.OrderID <= 0 || strings.TrimSpace(req.DeliveryMethod) == "" {
		return nil, errs.New(errs.InvalidArgument, "order_id و delivery_method مطلوبة")
	}
	dm := strings.ToLower(strings.TrimSpace(req.DeliveryMethod))
	if dm != "courier" && dm != "pickup" {
		return nil, errs.New(errs.ValidationFailed, "delivery_method يجب أن تكون courier أو pickup")
	}

	// Ensure order exists and is paid (friendly validation before DB trigger)
	var ordStatus string
	if err := db.Stdlib().QueryRowContext(ctx, `SELECT status::text FROM orders WHERE id=$1`, req.OrderID).Scan(&ordStatus); err != nil {
		return nil, errs.New(errs.NotFound, "الطلب غير موجود")
	}
	if ordStatus != "paid" {
		return nil, errs.New(errs.ShpOrderNotPaid, "لا يمكن إنشاء شحنة لطلب غير مدفوع")
	}

	// Insert shipment
	var it AdminShipmentItem
	row := db.Stdlib().QueryRowContext(ctx, `
        INSERT INTO shipments (order_id, company_id, delivery_method, tracking_ref)
        VALUES ($1, $2, $3::delivery_method, $4)
        RETURNING id, order_id, COALESCE(company_id, 0), delivery_method::text, status::text, tracking_ref,
                  to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
    `, req.OrderID, req.CompanyID, dm, req.TrackingRef)

	var tr sql.NullString
	if err := row.Scan(&it.ID, &it.OrderID, &it.CompanyID, &it.DeliveryMethod, &it.Status, &tr, &it.CreatedAt); err != nil {
		return nil, errs.New(errs.Internal, "فشل إنشاء الشحنة")
	}
	if tr.Valid {
		v := tr.String
		it.TrackingRef = &v
	}
	return &it, nil
}

type UpdateShipmentRequest struct {
	Status      *string `json:"status,omitempty"`
	TrackingRef *string `json:"tracking_ref,omitempty"`
}

//encore:api auth method=PATCH path=/admin/shipments/:id
func (s *Service) UpdateShipment(ctx context.Context, id int64, req *UpdateShipmentRequest) (*AdminShipmentItem, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	if req == nil || (req.Status == nil && req.TrackingRef == nil) {
		return nil, errs.New(errs.InvalidArgument, "لا يوجد حقول للتحديث")
	}

	// Build dynamic update
	setParts := []string{"updated_at=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')"}
	var args []interface{}
	if req.Status != nil {
		setParts = append(setParts, "status=$1")
		args = append(args, *req.Status)
	}
	if req.TrackingRef != nil {
		setParts = append(setParts, fmt.Sprintf("tracking_ref=$%d", len(args)+1))
		args = append(args, *req.TrackingRef)
	}
	args = append(args, id)
	q := fmt.Sprintf("UPDATE shipments SET %s WHERE id=$%d", strings.Join(setParts, ", "), len(args))
	if _, err := db.Stdlib().ExecContext(ctx, q, args...); err != nil {
		return nil, errs.New(errs.Internal, "فشل تحديث الشحنة")
	}

	// Read back
	row := db.Stdlib().QueryRowContext(ctx, `SELECT id, order_id, company_id, delivery_method::text, status::text, tracking_ref,
        to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM shipments WHERE id=$1`, id)
	var it AdminShipmentItem
	var tr sql.NullString
	if err := row.Scan(&it.ID, &it.OrderID, &it.CompanyID, &it.DeliveryMethod, &it.Status, &tr, &it.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, errs.New(errs.NotFound, "الشحنة غير موجودة")
		}
		return nil, errs.New(errs.Internal, "فشل قراءة الشحنة")
	}
	if tr.Valid {
		v := tr.String
		it.TrackingRef = &v
	}
	return &it, nil
}

// Cities
type AdminCityItem struct {
	ID             int64  `json:"id"`
	NameAr         string `json:"name_ar"`
	NameEn         string `json:"name_en"`
	ShippingFeeNet string `json:"shipping_fee_net"`
	Enabled        bool   `json:"enabled"`
}

type CreateCityRequest struct {
	NameAr         string  `json:"name_ar"`
	NameEn         *string `json:"name_en,omitempty"`
	ShippingFeeNet string  `json:"shipping_fee_net"` // decimal as string
	Enabled        bool    `json:"enabled"`
}

type ListCitiesResponse struct {
	Items []AdminCityItem `json:"items"`
}

// duplicate CreateCity removed; see earlier definition

//encore:api auth method=GET path=/admin/cities
func (s *Service) ListCities(ctx context.Context) (*ListCitiesResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	rows, err := db.Stdlib().QueryContext(ctx, `SELECT id, name_ar, name_en, shipping_fee_net::text, enabled FROM cities ORDER BY id`)
	if err != nil {
		return nil, errs.New(errs.Internal, "فشل قراءة المدن")
	}
	defer rows.Close()
	resp := &ListCitiesResponse{}
	for rows.Next() {
		var it AdminCityItem
		if err := rows.Scan(&it.ID, &it.NameAr, &it.NameEn, &it.ShippingFeeNet, &it.Enabled); err != nil {
			return nil, errs.New(errs.Internal, "فشل قراءة صف مدينة")
		}
		resp.Items = append(resp.Items, it)
	}
	return resp, nil
}

type UpdateCityRequest struct {
	Enabled        *bool   `json:"enabled,omitempty"`
	ShippingFeeNet *string `json:"shipping_fee_net,omitempty"` // decimal as string
}

//encore:api auth method=PATCH path=/admin/cities/:id
func (s *Service) UpdateCity(ctx context.Context, id int64, req *UpdateCityRequest) (*ListCitiesResponse, error) {
	if _, ok := auth.UserID(); !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}
	if req == nil || (req.Enabled == nil && req.ShippingFeeNet == nil) {
		return nil, errs.New(errs.InvalidArgument, "لا يوجد حقول للتحديث")
	}
	// Build update
	var setParts []string
	var args []interface{}
	if req.Enabled != nil {
		setParts = append(setParts, fmt.Sprintf("enabled=$%d", len(args)+1))
		args = append(args, *req.Enabled)
	}
	if req.ShippingFeeNet != nil {
		setParts = append(setParts, fmt.Sprintf("shipping_fee_net=$%d", len(args)+1))
		args = append(args, *req.ShippingFeeNet)
	}
	args = append(args, id)
	q := fmt.Sprintf("UPDATE cities SET %s WHERE id=$%d", strings.Join(setParts, ", "), len(args))
	if _, err := db.Stdlib().ExecContext(ctx, q, args...); err != nil {
		return nil, errs.New(errs.Internal, "فشل تحديث المدينة")
	}
	return s.ListCities(ctx)
}

// ====== Admin: Archive Product ======

type ArchiveProductRequest struct {
	Reason *string `json:"reason,omitempty"`
}

type ArchiveProductResponse struct {
	Message string `json:"message"`
}

//encore:api auth method=PATCH path=/admin/products/:id/archive
func (s *Service) ArchiveProduct(ctx context.Context, id int64, req *ArchiveProductRequest) (*ArchiveProductResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	// Convert user ID for audit
	var actorID *int64
	if id64, err := strconv.ParseInt(string(uidStr), 10, 64); err == nil {
		actorID = &id64
	}

	// Check current product status - prevent archiving products in critical states
	var productType, status string
	err := db.Stdlib().QueryRowContext(ctx,
		`SELECT type::text, status::text FROM products WHERE id = $1`, id,
	).Scan(&productType, &status)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errs.New(errs.NotFound, "المنتج غير موجود")
		}
		return nil, errs.New(errs.Internal, "فشل التحقق من المنتج")
	}

	// Prevent archiving products in critical states
	forbiddenStatuses := []string{"in_auction", "auction_hold", "reserved", "payment_in_progress"}
	for _, s := range forbiddenStatuses {
		if status == s {
			return nil, errs.New(errs.Conflict, fmt.Sprintf("لا يمكن أرشفة منتج في حالة '%s'. يجب إنهاء المزاد أو إلغاء الحجوزات أولاً", status))
		}
	}

	// Update product status to archived
	_, err = db.Stdlib().ExecContext(ctx,
		`UPDATE products SET status = 'archived', updated_at = NOW() WHERE id = $1`,
		id,
	)

	if err != nil {
		return nil, errs.New(errs.Internal, "فشل أرشفة المنتج")
	}

	// Audit log
	meta := map[string]interface{}{
		"product_id":   id,
		"product_type": productType,
		"old_status":   status,
		"new_status":   "archived",
	}
	if req != nil && req.Reason != nil {
		meta["reason"] = *req.Reason
	}

	if _, aerr := audit.Log(ctx, db, audit.Entry{
		ActorUserID: actorID,
		Action:      "product.archive",
		EntityType:  "product",
		EntityID:    strconv.FormatInt(id, 10),
		Meta:        meta,
	}); aerr != nil {
		logger.LogError(ctx, aerr, "failed to write audit log for product archive", logger.Fields{"product_id": id})
	}

	return &ArchiveProductResponse{Message: "تم أرشفة المنتج بنجاح"}, nil
}

// ====== Admin: Unarchive Product ======

//encore:api auth method=PATCH path=/admin/products/:id/unarchive
func (s *Service) UnarchiveProduct(ctx context.Context, id int64) (*ArchiveProductResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, errs.New(errs.Unauthenticated, "مطلوب تسجيل الدخول")
	}
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	// Convert user ID for audit
	var actorID *int64
	if id64, err := strconv.ParseInt(string(uidStr), 10, 64); err == nil {
		actorID = &id64
	}

	// Check if product exists and is archived
	var productType, status string
	err := db.Stdlib().QueryRowContext(ctx,
		`SELECT type::text, status::text FROM products WHERE id = $1`, id,
	).Scan(&productType, &status)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errs.New(errs.NotFound, "المنتج غير موجود")
		}
		return nil, errs.New(errs.Internal, "فشل التحقق من المنتج")
	}

	if status != "archived" {
		return nil, errs.New(errs.Conflict, "المنتج ليس مؤرشفاً")
	}

	// Restore to available status
	_, err = db.Stdlib().ExecContext(ctx,
		`UPDATE products SET status = 'available', updated_at = NOW() WHERE id = $1`,
		id,
	)

	if err != nil {
		return nil, errs.New(errs.Internal, "فشل إلغاء أرشفة المنتج")
	}

	// Audit log
	if _, aerr := audit.Log(ctx, db, audit.Entry{
		ActorUserID: actorID,
		Action:      "product.unarchive",
		EntityType:  "product",
		EntityID:    strconv.FormatInt(id, 10),
		Meta: map[string]interface{}{
			"product_id":   id,
			"product_type": productType,
			"old_status":   "archived",
			"new_status":   "available",
		},
	}); aerr != nil {
		logger.LogError(ctx, aerr, "failed to write audit log for product unarchive", logger.Fields{"product_id": id})
	}

	return &ArchiveProductResponse{Message: "تم إلغاء أرشفة المنتج بنجاح"}, nil
}
