-- 0002_system_settings.up.sql
-- الهجرة الثانية: إعدادات النظام
-- منصة لوفت الدغيري للحمام الزاجل

-- جدول إعدادات النظام
CREATE TABLE system_settings (
    id BIGSERIAL PRIMARY KEY,
    key TEXT NOT NULL,
    value TEXT,
    description TEXT,
    allowed_values TEXT[], -- قائمة القيم المسموحة (اختياري)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- قيد فريد على المفتاح
ALTER TABLE system_settings ADD CONSTRAINT uq_system_settings_key UNIQUE (key);

-- قيد التحقق: المفتاح غير فارغ
ALTER TABLE system_settings
    ADD CONSTRAINT chk_setting_key_not_empty
    CHECK (length(trim(key)) > 0);

-- إضافة trigger لتحديث updated_at تلقائياً
CREATE TRIGGER update_system_settings_updated_at
    BEFORE UPDATE ON system_settings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- إدراج الإعدادات الأولية
INSERT INTO system_settings (key, value, description, allowed_values) VALUES
-- إعدادات WebSocket
('ws.enabled', 'true', 'تفعيل WebSocket للإشعارات المباشرة', ARRAY['true', 'false']),
('ws.max_connections', '1000', 'الحد الأقصى لاتصالات WebSocket المتزامنة', NULL),
('ws.heartbeat_interval', '30', 'فترة نبضة القلب بالثواني', NULL),
('ws.max_connections_per_host', '120', 'الحد الأقصى لاتصالات WebSocket لكل مضيف', NULL),
('ws.msgs_per_minute', '30', 'الحد الأقصى لرسائل WS في الدقيقة لكل مضيف', NULL),

-- إعدادات المدفوعات
('payments.enabled', 'true', 'تفعيل نظام المدفوعات', ARRAY['true', 'false']),
('payments.provider', 'moyasar', 'مزود خدمة الدفع', ARRAY['moyasar', 'hyperpay', 'tabby']),
('payments.test_mode', 'true', 'وضع الاختبار للمدفوعات', ARRAY['true', 'false']),
('payments.currency', 'SAR', 'العملة الافتراضية', ARRAY['SAR', 'USD', 'EUR']),

-- إعدادات CORS
('cors.allowed_origins', '*', 'النطاقات المسموحة لـ CORS', NULL),
('cors.allowed_methods', 'GET,POST,PUT,DELETE,OPTIONS', 'الطرق المسموحة لـ CORS', NULL),
('cors.allowed_headers', 'Content-Type,Authorization,X-Requested-With', 'الرؤوس المسموحة لـ CORS', NULL),
('cors.max_age', '86400', 'مدة تخزين إعدادات CORS بالثواني', NULL),

-- إعدادات الوسائط
('media.max_file_size', '10485760', 'الحد الأقصى لحجم الملف بالبايت (10MB)', NULL),
('media.allowed_types', 'image/jpeg,image/png,image/webp,video/mp4', 'أنواع الملفات المسموحة', NULL),
('media.storage_provider', 'local', 'مزود تخزين الوسائط', ARRAY['local', 's3', 'cloudinary']),
('media.watermark.enabled', 'true', 'تفعيل العلامة المائية', ARRAY['true', 'false']),
('media.watermark.position', 'bottom-right', 'موضع العلامة المائية', ARRAY['top-left', 'top-right', 'bottom-left', 'bottom-right', 'center']),
('media.watermark.opacity', '30', 'شفافية العلامة المائية (0..100 كنسبة مئوية)', NULL),

-- إعدادات عامة
('app.name', 'لوفت الدغيري', 'اسم التطبيق', NULL),
('app.version', '1.0.0', 'إصدار التطبيق', NULL),
('app.maintenance_mode', 'false', 'وضع الصيانة', ARRAY['true', 'false']),
('app.registration_enabled', 'true', 'تفعيل التسجيل الجديد', ARRAY['true', 'false']),

-- إعدادات الإشعارات
('notifications.email_enabled', 'true', 'تفعيل إشعارات البريد الإلكتروني', ARRAY['true', 'false']),
('notifications.sms_enabled', 'false', 'تفعيل إشعارات الرسائل النصية', ARRAY['true', 'false']),
('notifications.push_enabled', 'false', 'تفعيل الإشعارات الفورية', ARRAY['true', 'false']),

-- إعدادات الأمان
('security.session_timeout', '3600', 'انتهاء صلاحية الجلسة بالثواني', NULL),
('security.max_login_attempts', '5', 'الحد الأقصى لمحاولات تسجيل الدخول', NULL),
('security.lockout_duration', '900', 'مدة الحظر بعد تجاوز المحاولات بالثواني', NULL),

-- إعدادات المزادات
('auctions.default_duration', '7', 'المدة الافتراضية للمزاد بالأيام', NULL),
('auctions.min_bid_increment', '10.00', 'الحد الأدنى لزيادة المزايدة', NULL),
('auctions.auto_extend_enabled', 'true', 'تفعيل التمديد التلقائي للمزاد', ARRAY['true', 'false']),
('auctions.auto_extend_duration', '300', 'مدة التمديد التلقائي بالثواني', NULL),
('auctions.anti_sniping_minutes', '10', 'مدة تمديد المزاد بالدقائق عند المزايدة في الدقائق الأخيرة', NULL),

-- إعدادات VAT والشحن وجلسة الدفع ومحفوظات الإشعارات
('vat.enabled', 'true', 'تفعيل ضريبة القيمة المضافة', ARRAY['true', 'false']),
('vat.rate', '0.15', 'معدل ضريبة القيمة المضافة (0..0.25)', NULL),
('shipping.free_shipping_threshold', '300.00', 'عتبة الشحن المجاني (المجموع الشامل قبل الشحن)', NULL),
('auctions.max_extensions', '0', 'الحد الأقصى لتمديدات Anti-Sniping (0=غير محدود)', NULL),
('payments.session_ttl_minutes', '30', 'صلاحية جلسة الدفع بالدقائق', NULL),
('payments.idempotency_ttl_hours', '24', 'مدة الاحتفاظ بمفاتيح Idempotency بالساعات', NULL),
('notifications.email.retention_days', '7', 'عدد أيام الاحتفاظ بإشعارات البريد', NULL),
('bids.rate_limit_per_minute', '60', 'حد المزايدات لكل مستخدم في الدقيقة', NULL),
('payments.rate_limit_per_5min', '5', 'حد تهيئة الدفع لكل مستخدم خلال 5 دقائق', NULL),

-- إعدادات المخزون والسلة
('stock.checkout_hold_minutes', '10', 'مدة حجز الحمام بالدقائق', NULL),
('stock.supplies_hold_minutes', '15', 'مدة حجز المستلزمات بالدقائق', NULL),
('stock.max_active_holds_per_user', '5', 'الحد الأقصى للحجوزات النشطة لكل مستخدم', NULL),

-- إعدادات إضافية
('media.thumbnails.enabled', 'true', 'تفعيل إنشاء المصغّرات عند الرفع', ARRAY['true','false']),
('pay.methods', '["mada","credit_card","applepay"]', 'طرق الدفع المفعّلة', NULL);