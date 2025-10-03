package notifications

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	"encore.app/pkg/templates"
	"encore.dev/beta/auth"
	"encore.app/pkg/errs"
	"encore.dev/storage/sqldb"
)

var db = sqldb.Named("coredb")

//encore:service
type Service struct{}

func initService() (*Service, error) { return &Service{}, nil }

type Notification struct {
	ID        int64           `json:"id"`
	UserID    int64           `json:"user_id"`
	Channel   string          `json:"channel"`
	Template  string          `json:"template_id"`
	Payload   json.RawMessage `json:"payload"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"created_at"`
}

type ListResponse struct {
	Items []Notification `json:"items"`
}

// ListQuery معلمات التصفح
type ListQuery struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

//encore:api auth method=GET path=/notifications
func (s *Service) List(ctx context.Context, req *ListQuery) (*ListResponse, error) {
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, errs.New(errs.NotifUnauthenticated, "مطلوب تسجيل الدخول")
	}
	uid, err := strconv.ParseInt(string(uidStr), 10, 64)
	if err != nil {
		return nil, errs.New(errs.InvalidArgument, "معرّف مستخدم غير صالح")
	}
	// ضبط الحدود الافتراضية
	limit := 20
	offset := 0
	if req != nil {
		if req.Limit > 0 {
			if req.Limit > 100 {
				limit = 100
			} else {
				limit = req.Limit
			}
		}
		if req.Offset > 0 {
			offset = req.Offset
		}
	}
	rows, err := db.Stdlib().QueryContext(ctx, `
        SELECT id, user_id, channel::text, template_id, payload, status::text,
               to_char(created_at at time zone 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
        FROM notifications 
        WHERE user_id=$1 AND channel='internal' 
        ORDER BY created_at DESC
        LIMIT $2 OFFSET $3
    `, uid, limit, offset)
	if err != nil {
		return nil, errs.New(errs.NotifListQueryFailed, "فشل الاستعلام")
	}
	defer rows.Close()
	var items []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Channel, &n.Template, &n.Payload, &n.Status, &n.CreatedAt); err != nil {
			return nil, errs.New(errs.NotifListQueryFailed, "فشل القراءة")
		}
		items = append(items, n)
	}
	return &ListResponse{Items: items}, nil
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

// TestEmailRequest طلب اختبار البريد الإلكتروني
type TestEmailRequest struct {
	Email      string          `json:"email"`
	TemplateID string          `json:"template_id"`
	Language   string          `json:"language"` // ar or en
	Data       json.RawMessage `json:"data,omitempty"`
}

// TestEmailResponse استجابة اختبار البريد
type TestEmailResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	EmailID int64  `json:"email_id,omitempty"`
}

//encore:api auth method=POST path=/notifications/email/test
func (s *Service) TestEmail(ctx context.Context, req *TestEmailRequest) (*TestEmailResponse, error) {
	// Check if user is admin (simplified - in production check roles)
	uidStr, ok := auth.UserID()
	if !ok {
		return nil, errs.New(errs.NotifUnauthenticated, "مطلوب تسجيل الدخول")
	}
	// Enforce admin-only access using role from auth.Data()
	if !isAdmin() {
		return nil, errs.New(errs.Forbidden, "يتطلب صلاحيات مدير")
	}

	uid, err := strconv.ParseInt(string(uidStr), 10, 64)
	if err != nil {
		return nil, errs.New(errs.InvalidArgument, "معرّف مستخدم غير صالح")
	}
	
	// Validate template exists
	_, err = templates.GetTemplateInfo(req.TemplateID)
	if err != nil {
		return nil, errs.New(errs.NotifInvalidTemplate, "القالب غير موجود")
	}
	
	// Prepare test data
	dataMap := make(map[string]interface{})
	if len(req.Data) > 0 {
		_ = json.Unmarshal(req.Data, &dataMap)
	}
	dataMap["email"] = req.Email
	dataMap["name"] = "Test User"
	dataMap["language"] = req.Language
	
	// Add sample data based on template
	switch req.TemplateID {
	case "welcome":
		dataMap["ActivationURL"] = "https://loft-dughairi.com/activate/test-token"
	case "bid_placed":
		dataMap["ItemName"] = "ساعة رولكس تجريبية"
		dataMap["BidAmount"] = "5000"
		dataMap["BidTime"] = time.Now().Format("2006-01-02 15:04:05")
		dataMap["AuctionID"] = "TEST123"
		dataMap["AuctionURL"] = "https://loft-dughairi.com/auction/TEST123"
	case "auction_won":
		dataMap["ItemName"] = "ساعة رولكس تجريبية"
		dataMap["FinalPrice"] = "10000"
		dataMap["AuctionID"] = "TEST123"
		dataMap["EndDate"] = time.Now().Format("2006-01-02 15:04:05")
		dataMap["PaymentURL"] = "https://loft-dughairi.com/payment/TEST123"
	case "password_reset":
		dataMap["ResetURL"] = "https://loft-dughairi.com/reset-password/test-token"
	}
	
	// Enqueue test email
	emailID, err := EnqueueEmail(ctx, uid, req.TemplateID, dataMap)
	if err != nil {
		return nil, errs.New(errs.NotifQueueInsertFailed, "فشل إرسال البريد التجريبي")
	}
	
	return &TestEmailResponse{
		Success: true,
		Message: "تم إضافة البريد إلى الطابور بنجاح. سيتم إرساله خلال دقيقتين.",
		EmailID: emailID,
	}, nil
}

// TemplateInfo تمثيل معلومات القالب
type TemplateInfo struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Languages   []string `json:"languages"`
}

// GetTemplatesRequest طلب الحصول على القوالب
type GetTemplatesRequest struct{}

// GetTemplatesResponse استجابة القوالب المتاحة
type GetTemplatesResponse struct {
	Templates []TemplateInfo `json:"templates"`
}

//encore:api auth method=GET path=/notifications/templates
func (s *Service) GetTemplates(ctx context.Context) (*GetTemplatesResponse, error) {
	_, ok := auth.UserID()
	if !ok {
		return nil, errs.New(errs.NotifUnauthenticated, "مطلوب تسجيل الدخول")
	}
	
	templateIDs := templates.GetAvailableTemplates()
	var templatesInfo []TemplateInfo
    
	for _, id := range templateIDs {
		info, _ := templates.GetTemplateInfo(id)
		ti := TemplateInfo{}
		if v, ok := info["id"].(string); ok { ti.ID = v }
		if v, ok := info["description"].(string); ok { ti.Description = v }
		if langs, ok := info["languages"].([]string); ok { ti.Languages = langs }
		templatesInfo = append(templatesInfo, ti)
	}
    
	return &GetTemplatesResponse{
		Templates: templatesInfo,
	}, nil
}

