# إصلاح مشكلة حفظ حالة طلب التوثيق

## المشكلة
عند إرسال طلب توثيق من الفرونت إند، كان الطلب يُرسل بنجاح لكن عند تحديث الصفحة يختفي. السبب: **لا يوجد endpoint في الباك إند لجلب طلب التوثيق الحالي للمستخدم**.

## الحل المطبق

### 1. إضافة Endpoint جديد في الباك إند

#### الملف: `svc/users/api.go`
```go
// GetMyVerificationRequest returns the current user's verification request if it exists
//
//encore:api auth method=GET path=/verify/my-request
func (s *Service) GetMyVerificationRequest(ctx context.Context) (*VerificationRequestDetail, error) {
	userID, ok := auth.UserID()
	if !ok {
		return nil, &errs.Error{Code: errs.Unauthenticated, Message: "المستخدم غير مصادق."}
	}

	userIDInt64, err := strconv.ParseInt(string(userID), 10, 64)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "معرّف المستخدم غير صالح."}
	}

	return s.GetVerificationRequestByUserID(ctx, userIDInt64)
}
```

### 2. إضافة DTO جديد

#### الملف: `svc/users/dto.go`
```go
// VerificationRequestDetail represents a detailed verification request with admin_reason
type VerificationRequestDetail struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Note        string     `json:"note"`
	Status      string     `json:"status"` // pending, approved, rejected
	AdminReason *string    `json:"admin_reason"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
```

### 3. تحديث Database Model

#### الملف: `svc/users/repo.go`
```go
// VerificationRequestDB - تم تحديثه ليشمل admin_reason و updated_at
type VerificationRequestDB struct {
    ID          int64
    UserID      int64
    Note        string
    Status      string
    AdminReason *string      // ← جديد
    ReviewedBy  *int64
    ReviewedAt  *time.Time
    CreatedAt   time.Time
    UpdatedAt   time.Time    // ← جديد
}
```

### 4. إضافة دالة Repository

#### الملف: `svc/users/repo.go`
```go
// GetLatestVerificationRequestByUserID retrieves the latest verification request for a user
func (r *Repository) GetLatestVerificationRequestByUserID(ctx context.Context, userID int64) (*VerificationRequestDB, error) {
    var req VerificationRequestDB
    err := r.db.QueryRow(ctx, `
        SELECT id, user_id, note, status, admin_reason, reviewed_by, reviewed_at, created_at, updated_at
        FROM verification_requests 
        WHERE user_id = $1 
        ORDER BY created_at DESC 
        LIMIT 1`, userID,
    ).Scan(&req.ID, &req.UserID, &req.Note, &req.Status, &req.AdminReason, &req.ReviewedBy, &req.ReviewedAt, &req.CreatedAt, &req.UpdatedAt)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, &errs.Error{Code: errs.NotFound, Message: "لا يوجد طلب توثيق."}
        }
        return nil, &errs.Error{Code: errs.Internal, Message: "خطأ في قراءة طلب التوثيق."}
    }
    return &req, nil
}
```

### 5. إضافة دالة Service

#### الملف: `svc/users/service.go`
```go
// GetVerificationRequestByUserID retrieves the current verification request for a user
func (s *Service) GetVerificationRequestByUserID(ctx context.Context, userID int64) (*VerificationRequestDetail, error) {
	verificationReq, err := s.repo.GetLatestVerificationRequestByUserID(ctx, userID)
	if err != nil {
		if err.Error() == "sql: no rows in result set" || err.Error() == "no verification request found" {
			return nil, &errs.Error{
				Code:    errs.NotFound,
				Message: "لا يوجد طلب توثيق.",
			}
		}
		return nil, err
	}

	return &VerificationRequestDetail{
		ID:          verificationReq.ID,
		UserID:      verificationReq.UserID,
		Note:        verificationReq.Note,
		Status:      verificationReq.Status,
		AdminReason: verificationReq.AdminReason,
		CreatedAt:   verificationReq.CreatedAt,
		UpdatedAt:   verificationReq.UpdatedAt,
	}, nil
}
```

### 6. تحديث Frontend API

#### الملف: `src/lib/api/users-api.ts`
```typescript
// Backend returns the request directly, not wrapped
const data = await response.json();
return data; // بدلاً من data.request
```

## الـ API الجديد

### Endpoint
```
GET /verify/my-request
```

### Headers
```
Authorization: Bearer <access_token>
```

### Response (200 OK)
```json
{
  "id": 123,
  "user_id": 456,
  "note": "مربي حمام زاجل منذ 5 سنوات",
  "status": "pending",
  "admin_reason": null,
  "created_at": "2024-01-01T10:00:00Z",
  "updated_at": "2024-01-01T10:00:00Z"
}
```

### Response (404 Not Found)
```json
{
  "code": "not_found",
  "message": "لا يوجد طلب توثيق."
}
```

## الحالات المدعومة

1. **لا يوجد طلب**: يرجع 404
2. **طلب معلق (pending)**: يعرض حالة "قيد المراجعة"
3. **طلب مقبول (approved)**: يعرض حالة "تم القبول"
4. **طلب مرفوض (rejected)**: يعرض حالة "تم الرفض" مع سبب الرفض

## الاختبار

```bash
# تشغيل الباك إند
cd loft-backend
encore run

# اختبار الـ endpoint
curl -X GET http://localhost:4000/verify/my-request \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

## النتيجة النهائية

✅ عند إرسال طلب توثيق، يتم حفظه في قاعدة البيانات
✅ عند تحديث الصفحة، يتم جلب الطلب من الباك إند
✅ الحالة تظهر بشكل صحيح (معلق/مقبول/مرفوض)
✅ سبب الرفض يظهر إذا كان موجوداً
