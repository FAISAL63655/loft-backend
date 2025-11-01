# خطوات تطبيق Migration لإصلاح طلبات التوثيق

## المشكلة
الجدول `verification_requests` لا يحتوي على عمود `admin_reason`، مما يسبب خطأ عند محاولة جلب طلب التوثيق.

## الحل

### 1. تطبيق Migration
قم بتشغيل الأمر التالي لتطبيق migration الجديد:

```bash
cd loft-backend
encore db migrate
```

أو يدوياً:

```sql
-- Add admin_reason column to verification_requests table
ALTER TABLE verification_requests 
ADD COLUMN IF NOT EXISTS admin_reason TEXT;

COMMENT ON COLUMN verification_requests.admin_reason IS 'سبب الموافقة أو الرفض من قبل المدير';
```

### 2. التحقق من التطبيق
تحقق من أن العمود تم إضافته بنجاح:

```sql
\d verification_requests
```

يجب أن ترى:
```
Column       | Type         | Nullable
-------------+--------------+----------
id           | bigint       | not null
user_id      | bigint       | not null
note         | text         |
status       | verification_status | not null
admin_reason | text         |          ← الجديد
reviewed_by  | bigint       |
reviewed_at  | timestamptz  |
created_at   | timestamptz  | not null
updated_at   | timestamptz  | not null
```

### 3. إعادة تشغيل الباك إند

```bash
encore run
```

### 4. اختبار الـ API

```bash
# اختبار جلب طلب التوثيق
curl -X GET http://localhost:4000/verify/my-request \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

## الملفات المعدلة

### Backend:
- ✅ `coredb/migrations/0017_add_admin_reason_to_verification_requests.up.sql` - Migration جديد
- ✅ `coredb/migrations/0017_add_admin_reason_to_verification_requests.down.sql` - Rollback
- ✅ `svc/users/dto.go` - إضافة `VerificationRequestDetail`
- ✅ `svc/users/repo.go` - تحديث queries لتشمل `admin_reason`
- ✅ `svc/users/service.go` - تمرير `admin_reason` عند الموافقة/الرفض
- ✅ `svc/users/api.go` - إضافة `GetMyVerificationRequest` endpoint

### Frontend:
- ✅ `src/lib/api/users-api.ts` - تحديث لاستقبال الرد المباشر

## النتيجة المتوقعة

بعد تطبيق Migration:
- ✅ يمكن جلب طلب التوثيق الحالي بنجاح
- ✅ يتم حفظ سبب الموافقة/الرفض من الأدمن
- ✅ عند تحديث الصفحة، يبقى الطلب محفوظاً
- ✅ تظهر رسالة الأدمن (إن وجدت) في واجهة المستخدم

## Troubleshooting

### إذا فشل Migration:
```bash
# تحقق من حالة migrations
encore db migrate status

# إذا كان هناك خطأ، يمكنك التراجع
encore db migrate down
```

### إذا استمر الخطأ:
1. تحقق من أن قاعدة البيانات تعمل
2. تحقق من أن المستخدم لديه صلاحيات ALTER TABLE
3. جرب تطبيق SQL يدوياً في psql

```bash
psql -U postgres -d loft_dev
```

ثم:
```sql
ALTER TABLE verification_requests ADD COLUMN IF NOT EXISTS admin_reason TEXT;
```
