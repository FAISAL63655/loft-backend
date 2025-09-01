# Dynamic Configuration System (pkg/config)

نظام التكوين الديناميكي لمنصة لوفت الدغيري مع دعم **Hot Reload** لجميع إعدادات النظام.

## ✨ المميزات

- 🔄 **Hot Reload**: تحديث الإعدادات دون إعادة تشغيل التطبيق
- 🗄️ **قاعدة البيانات**: تخزين الإعدادات في جدول `system_settings`
- 🚀 **Thread-Safe**: آمن للاستخدام المتزامن
- 💾 **Cache مدمج**: تخزين مؤقت للأداء العالي
- 📡 **Event Listeners**: إشعارات عند تغيير الإعدادات
- 🛡️ **Fallback**: قيم افتراضية عند عدم توفر قاعدة البيانات

## 📁 هيكل النظام

```
pkg/config/
├── config.go          # ConfigManager الرئيسي
├── example_usage.go    # أمثلة للاستخدام
└── README.md          # هذا الملف
```

## 🚀 الاستخدام السريع

### 1. التهيئة الأولية

```go
import "encore.app/pkg/config"

// تهيئة النظام مع Hot Reload كل 5 دقائق
manager := config.Initialize(5 * time.Minute)

// أو استخدام الإعدادات مباشرة
settings := config.GetSettings()
```

### 2. قراءة الإعدادات

```go
settings := config.GetSettings()

// إعدادات التطبيق
fmt.Printf("اسم التطبيق: %s\n", settings.AppName)
fmt.Printf("الإصدار: %s\n", settings.AppVersion)

// إعدادات VAT
if settings.VATEnabled {
    vatRate := settings.VATRate * 100
    fmt.Printf("ضريبة القيمة المضافة: %.0f%%\n", vatRate)
}

// إعدادات CORS
fmt.Printf("النطاقات المسموحة: %v\n", settings.CORSAllowedOrigins)
```

### 3. تحديث الإعدادات

```go
manager := config.GetGlobalManager()

// تحديث إعداد واحد
err := manager.UpdateSetting(ctx, "app.name", "اسم جديد")
if err != nil {
    log.Printf("فشل التحديث: %v", err)
}
```

### 4. الاستماع للتغييرات

```go
manager.AddChangeListener(func(newSettings *config.SystemSettings) {
    log.Printf("تم تحديث الإعدادات!")
    log.Printf("الاسم الجديد: %s", newSettings.AppName)
    
    // إعادة تكوين الخدمات حسب الحاجة
    updateCORSSettings(newSettings.CORSAllowedOrigins)
})
```

## 🌐 CORS الديناميكي

النظام يدعم CORS ديناميكي يقرأ الإعدادات من قاعدة البيانات:

```go
import "encore.app/pkg/middleware"

// استخدام CORS ديناميكي
app.Use(middleware.CORSMiddleware(middleware.DynamicCORSConfig))

// CORS ثابت (للمقارنة)
app.Use(middleware.CORSMiddleware(middleware.DefaultCORSConfig))
```

### إعدادات CORS في قاعدة البيانات:

```sql
-- تحديث النطاقات المسموحة
UPDATE system_settings 
SET value = 'https://loft-dughairi.com,https://admin.loft-dughairi.com' 
WHERE key = 'cors.allowed_origins';

-- تفعيل/تعطيل CORS
UPDATE system_settings 
SET value = 'GET,POST,PUT,DELETE,PATCH,OPTIONS' 
WHERE key = 'cors.allowed_methods';
```

## ⚙️ الإعدادات المتوفرة

### 📱 إعدادات التطبيق
- `app.name`: اسم التطبيق
- `app.version`: الإصدار
- `app.maintenance_mode`: وضع الصيانة
- `app.registration_enabled`: تفعيل التسجيل

### 💳 إعدادات المدفوعات
- `payments.enabled`: تفعيل المدفوعات
- `payments.provider`: مزود الخدمة (moyasar, hyperpay, tabby)
- `payments.test_mode`: وضع الاختبار
- `payments.currency`: العملة (SAR, USD, EUR)

### 🛡️ إعدادات الأمان
- `security.session_timeout`: انتهاء الجلسة (ثانية)
- `security.max_login_attempts`: محاولات تسجيل الدخول
- `security.lockout_duration`: مدة الحظر (ثانية)

### 💰 إعدادات VAT والشحن
- `vat.enabled`: تفعيل ضريبة القيمة المضافة
- `vat.rate`: معدل الضريبة (0.15 = 15%)
- `shipping.free_shipping_threshold`: عتبة الشحن المجاني

### 🎯 إعدادات المزادات
- `auctions.default_duration`: مدة المزاد الافتراضية (أيام)
- `auctions.min_bid_increment`: أقل زيادة مزايدة
- `auctions.auto_extend_enabled`: التمديد التلقائي
- `auctions.max_extensions`: حد التمديدات

### 📎 إعدادات الوسائط
- `media.max_file_size`: أقصى حجم ملف (بايت)
- `media.allowed_types`: أنواع الملفات المسموحة
- `media.watermark_enabled`: تفعيل العلامة المائية

## 🔄 Hot Reload

النظام يدعم Hot Reload التلقائي:

```go
// تهيئة مع فحص كل دقيقة
manager := config.Initialize(1 * time.Minute)

// إيقاف Hot Reload
manager.StopHotReload()
```

### كيف يعمل Hot Reload:
1. **Timer**: فحص دوري لقاعدة البيانات
2. **Change Detection**: مقارنة الإعدادات الجديدة
3. **Atomic Update**: تحديث thread-safe للإعدادات
4. **Notifications**: إشعار المستمعين بالتغييرات
5. **Cache Invalidation**: إزالة Cache عند التحديث

## 💾 نظام Cache

Cache ذكي لتحسين الأداء:

```go
manager := config.GetGlobalManager()

// فحص Cache أولاً
if value, exists := manager.GetCachedValue("expensive_calc"); exists {
    return value
}

// حساب القيمة
result := performExpensiveCalculation()

// حفظ في Cache
manager.SetCachedValue("expensive_calc", result)
```

## 🗄️ إدارة قاعدة البيانات

### إضافة إعداد جديد:

```sql
INSERT INTO system_settings (key, value, description, allowed_values) 
VALUES ('notifications.push_enabled', 'false', 'تفعيل الإشعارات الفورية', ARRAY['true', 'false']);
```

### تحديث إعداد:

```sql
UPDATE system_settings 
SET value = 'true', updated_at = NOW() 
WHERE key = 'notifications.push_enabled';
```

### حذف إعداد:

```sql
DELETE FROM system_settings 
WHERE key = 'old_setting';
```

## 🎯 أفضل الممارسات

### ✅ افعل:
- استخدم `config.GetSettings()` لقراءة الإعدادات
- أضف listeners للتفاعل مع التغييرات
- استخدم DynamicCORSConfig للنطاقات المتغيرة
- اعتمد على fallback values
- استخدم Cache للعمليات المكلفة

### ❌ لا تفعل:
- تعديل `SystemSettings` مباشرة
- استدعاء `LoadSettings()` بشكل متكرر
- تجاهل errors في `UpdateSetting()`
- استخدام Hot Reload بفترات قصيرة جداً (< 1 دقيقة)

## 🧪 أمثلة متقدمة

### مثال: خدمة تتكيف مع الإعدادات

```go
type PaymentService struct {
    config *config.SystemSettings
    manager *config.ConfigManager
}

func NewPaymentService() *PaymentService {
    manager := config.GetGlobalManager()
    service := &PaymentService{
        config: manager.GetSettings(),
        manager: manager,
    }
    
    // الاستماع للتغييرات
    manager.AddChangeListener(service.onConfigChange)
    
    return service
}

func (ps *PaymentService) onConfigChange(newConfig *config.SystemSettings) {
    ps.config = newConfig
    
    // إعادة تكوين مزود الدفع
    if newConfig.PaymentsProvider != ps.config.PaymentsProvider {
        ps.reconfigureProvider(newConfig.PaymentsProvider)
    }
}
```

### مثال: Middleware متكيف

```go
func AdaptiveRateLimitMiddleware() func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            settings := config.GetSettings()
            
            // تكييف معدل الطلبات حسب الإعدادات
            if settings.AppMaintenanceMode {
                http.Error(w, "نظام تحت الصيانة", http.StatusServiceUnavailable)
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

## 📊 مراقبة النظام

```go
// إحصائيات النظام
settings := config.GetSettings()
fmt.Printf("آخر تحديث: %v\n", settings.LastUpdated)

// Cache statistics
manager := config.GetGlobalManager()
cacheStats := map[string]interface{}{
    "cache_size": len(manager.cache),
    "last_reload": manager.lastReload,
    "listeners_count": len(manager.listeners),
}
```

## 🔧 Troubleshooting

### مشكلة: Hot Reload لا يعمل
```go
// تأكد من أن Timer يعمل
manager := config.GetGlobalManager()
if manager.reloadTicker == nil {
    log.Printf("Hot Reload غير مفعل")
}
```

### مشكلة: إعدادات لا تتحديث
```sql
-- فحص آخر تحديث
SELECT key, value, updated_at 
FROM system_settings 
ORDER BY updated_at DESC 
LIMIT 10;
```

---

## 📝 ملاحظات

- النظام thread-safe ويمكن استخدامه في تطبيقات متعددة الخيوط
- Hot Reload يعمل في background دون تأثير على الأداء
- Fallback values تضمن استمرارية العمل حتى عند فشل قاعدة البيانات
- Cache يتم تنظيفه تلقائياً عند تحديث الإعدادات

تم التطوير مع ❤️ لمنصة لوفت الدغيري
