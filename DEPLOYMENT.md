# دليل نشر منصة لوفت الدغيري على Encore

## 📋 المتطلبات الأساسية

### 1. تثبيت Encore CLI
```bash
# Windows (PowerShell as Admin)
iwr https://encore.dev/install.ps1 | iex

# macOS/Linux
curl -L https://encore.dev/install.sh | bash
```

### 2. إنشاء حساب Encore
```bash
encore auth signup
# أو
encore auth login
```

## 🚀 خطوات النشر

### 1. التحقق من البيئة المحلية
```bash
cd loft-backend

# التأكد من صحة التطبيق
encore check

# تشغيل التطبيق محلياً
encore run
```

### 2. إعداد قاعدة البيانات
```bash
# تطبيق migrations
encore db migrate

# تحميل بيانات التطوير (اختياري)
encore db exec coredb < scripts/seed_dev.sql
```

### 3. تكوين المتغيرات البيئية
```bash
# إنشاء ملف .env.production
cp .env.example .env.production

# تحديث القيم الإنتاجية:
# - JWT_SECRET_KEY
# - MOYASAR_API_KEY  
# - SENDGRID_API_KEY
# - S3_BUCKET_NAME
# - وغيرها...
```

### 4. النشر على Encore Cloud

#### أ. إنشاء التطبيق (أول مرة فقط)
```bash
encore app create loft-dughairi
```

#### ب. ربط المشروع المحلي
```bash
encore app link loft-dughairi
```

#### ج. النشر إلى بيئة التطوير
```bash
encore deploy --env=development
```

#### د. النشر إلى الإنتاج
```bash
encore deploy --env=production
```

## 🔧 إدارة البيئات

### عرض البيئات المتاحة
```bash
encore env list
```

### إنشاء بيئة جديدة
```bash
encore env create staging
```

### تكوين الأسرار
```bash
# إضافة سر جديد
encore secret set --env=production JWT_SECRET_KEY

# عرض الأسرار المكونة
encore secret list --env=production
```

## 📊 المراقبة والسجلات

### عرض السجلات المباشرة
```bash
encore logs --env=production --follow
```

### عرض المقاييس
```bash
encore metrics --env=production
```

### الوصول إلى لوحة التحكم
```bash
encore dashboard
# أو زيارة: https://app.encore.dev
```

## 🔐 الأمان

### 1. فحص الأمان
```bash
encore test --security
```

### 2. تحديث الاعتماديات
```bash
go get -u ./...
go mod tidy
```

### 3. مراجعة CORS
تأكد من تكوين CORS في `encore.app`:
```yaml
global_cors:
  allowed_origins:
    - "https://loft-dughairi.com"
    - "https://www.loft-dughairi.com"
```

## 🌍 الربط بالدومين

### 1. في لوحة تحكم Encore
1. اذهب إلى Settings > Domains
2. أضف دومين مخصص: `api.loft-dughairi.com`
3. احصل على سجلات DNS المطلوبة

### 2. تكوين DNS (في مزود الدومين)
```
Type: CNAME
Name: api
Value: <your-app>.encr.app
TTL: 3600
```

### 3. تفعيل SSL
يتم تلقائياً عبر Let's Encrypt

## 📱 الربط مع Frontend

### رابط API الإنتاجي
```javascript
const API_BASE_URL = 'https://api.loft-dughairi.com'
// أو
const API_BASE_URL = 'https://loft-dughairi.encr.app'
```

### مثال الاستخدام
```javascript
const response = await fetch(`${API_BASE_URL}/auth/login`, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    email: 'user@example.com',
    password: 'password'
  })
});
```

## 🔄 التحديثات والصيانة

### نشر تحديث
```bash
# تأكد من commit جميع التغييرات
git add .
git commit -m "Update: وصف التحديث"

# النشر
encore deploy --env=production
```

### الرجوع لإصدار سابق
```bash
encore deploy --env=production --version=<version-id>
```

## 📞 الدعم

- [Encore Documentation](https://encore.dev/docs)
- [Discord Community](https://encore.dev/discord)
- [GitHub Issues](https://github.com/encoredev/encore)

## ✅ قائمة التحقق قبل النشر

- [ ] جميع الاختبارات تمر بنجاح
- [ ] تم تكوين جميع المتغيرات البيئية
- [ ] تم مراجعة إعدادات الأمان
- [ ] تم إعداد النسخ الاحتياطي لقاعدة البيانات
- [ ] تم تكوين المراقبة والتنبيهات
- [ ] تم توثيق API بالكامل
- [ ] تم اختبار التكامل مع بوابات الدفع
- [ ] تم التحقق من إعدادات CORS
- [ ] تم تجهيز خطة الطوارئ

---

**ملاحظة**: هذا التطبيق يستخدم Encore.app كمنصة سحابية متخصصة في تطبيقات Go، وليس Netlify أو Vercel.
