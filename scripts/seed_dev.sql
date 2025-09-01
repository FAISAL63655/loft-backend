-- seed_dev.sql
-- بيانات التطوير الشاملة
-- منصة لوفت الدغيري للحمام الزاجل
-- تاريخ التحديث: 2024

-- ========================================
-- 1. المدن الأساسية
-- ========================================
INSERT INTO cities (name_ar, name_en, shipping_fee_net, enabled) VALUES
('الرياض', 'Riyadh', 25.00, true),
('جدة', 'Jeddah', 30.00, true),
('الدمام', 'Dammam', 35.00, true),
('مكة المكرمة', 'Makkah', 28.00, true),
('المدينة المنورة', 'Madinah', 32.00, true)
ON CONFLICT DO NOTHING;

-- ========================================
-- 2. المستخدمون (كلمات المرور موحدة للتطوير)
-- ========================================
-- مستخدم إداري (كلمة المرور: admin123)
INSERT INTO users (name, email, password_hash, phone, role, state, email_verified_at, city_id) VALUES
('مدير النظام', 'admin@loft-dughairi.com', '$argon2id$v=19$m=65536,t=3,p=2$aGVsbG93b3JsZA$5Hp1/PgKHdMPgANL1q9eSQ2ZTMB9GF2dC5eqBiKsNBE', '+966501234567', 'admin', 'active', NOW(), 1)
ON CONFLICT (email) DO NOTHING;

-- مستخدمون مؤكدون للاختبار (كلمة المرور: user123)
INSERT INTO users (name, email, password_hash, phone, role, state, email_verified_at, city_id) VALUES
('أحمد الدغيري', 'ahmad@test.com', '$argon2id$v=19$m=65536,t=3,p=2$dGVzdHNhbHQ$9Kj8fGqWLCrP4YhMzBx9mQ3NQHA8RF7bN4tpKjLxODE', '+966502345678', 'verified', 'active', NOW(), 1),
('محمد العتيبي', 'mohammed@test.com', '$argon2id$v=19$m=65536,t=3,p=2$dGVzdHNhbHQ$9Kj8fGqWLCrP4YhMzBx9mQ3NQHA8RF7bN4tpKjLxODE', '+966503456789', 'verified', 'active', NOW(), 2),
('علي الشمري', 'ali@test.com', '$argon2id$v=19$m=65536,t=3,p=2$dGVzdHNhbHQ$9Kj8fGqWLCrP4YhMzBx9mQ3NQHA8RF7bN4tpKjLxODE', '+966504567890', 'verified', 'active', NOW(), 3),
('خالد الحربي', 'khalid@test.com', '$argon2id$v=19$m=65536,t=3,p=2$dGVzdHNhbHQ$9Kj8fGqWLCrP4YhMzBx9mQ3NQHA8RF7bN4tpKjLxODE', '+966505678901', 'verified', 'active', NOW(), 4)
ON CONFLICT (email) DO NOTHING;

-- مستخدم غير مؤكد للاختبار (كلمة المرور: user123)
INSERT INTO users (name, email, password_hash, phone, role, state, email_verified_at, city_id) VALUES
('سعد الجديد', 'saad@test.com', '$argon2id$v=19$m=65536,t=3,p=2$dGVzdHNhbHQ$9Kj8fGqWLCrP4YhMzBx9mQ3NQHA8RF7bN4tpKjLxODE', '+966506789012', 'registered', 'active', NULL, 5)
ON CONFLICT (email) DO NOTHING;

-- ========================================
-- 3. إعدادات النظام
-- ========================================
-- إعدادات المخزون
INSERT INTO system_settings (key, value, description) VALUES
('stock.checkout_hold_minutes', '10', 'مدة حجز الحمام بالدقائق'),
('stock.supplies_hold_minutes', '15', 'مدة حجز المستلزمات بالدقائق'),
('stock.max_active_holds_per_user', '5', 'الحد الأقصى للحجوزات النشطة لكل مستخدم'),
-- إعدادات الضريبة
('vat.enabled', 'true', 'تفعيل ضريبة القيمة المضافة'),
('vat.rate', '0.15', 'معدل ضريبة القيمة المضافة'),
-- إعدادات الشحن
('shipping.free_shipping_threshold', '500', 'الحد الأدنى للشحن المجاني'),
-- إعدادات المزادات
('auction.auto_extend_enabled', 'true', 'تفعيل التمديد التلقائي للمزادات'),
('auction.default_anti_sniping_minutes', '10', 'مدة مكافحة القنص بالدقائق'),
('auction.max_extensions', '3', 'الحد الأقصى للتمديدات'),
-- إعدادات الدفع
('payment.test_mode', 'true', 'وضع الاختبار للمدفوعات'),
('payment.timeout_minutes', '30', 'مهلة انتهاء جلسة الدفع'),
-- إعدادات الإشعارات
('notifications.retention_days', '30', 'مدة الاحتفاظ بالإشعارات بالأيام'),
-- إعدادات المعدل
('rate_limit.enabled', 'true', 'تفعيل تحديد المعدل'),
('rate_limit.requests_per_minute', '60', 'عدد الطلبات في الدقيقة')
ON CONFLICT (key) DO NOTHING;

-- ========================================
-- 4. المنتجات الأساسية للتطوير
-- ========================================
-- حمام زاجل
INSERT INTO products (type, title, slug, description, price_net, status) VALUES
('pigeon', 'حمام زاجل أصيل - ذكر', 'pigeon-male-001', 'حمام زاجل أصيل من سلالة الدغيري، مدرب على المسافات الطويلة', 800.00, 'available'),
('pigeon', 'حمام زاجل أصيل - أنثى', 'pigeon-female-001', 'حمام زاجل أنثى للتربية من سلالة العتيبي', 750.00, 'available'),
('pigeon', 'حمام زاجل صغير', 'pigeon-young-001', 'حمام زاجل صغير عمر 6 أشهر', 400.00, 'available'),
-- مستلزمات
('supply', 'علف حمام فاخر - 25كغ', 'feed-premium-25kg', 'علف عالي الجودة مخصص للحمام الزاجل', 75.00, 'available'),
('supply', 'فيتامينات شاملة - 500مل', 'vitamins-500ml', 'فيتامينات ومكملات غذائية للحمام', 45.00, 'available'),
('supply', 'قفص حمام كبير', 'cage-large', 'قفص حمام 60×40×40 سم', 120.00, 'available')
ON CONFLICT DO NOTHING;

-- تفاصيل الحمام
INSERT INTO pigeons (product_id, ring_number, sex, birth_date, lineage) 
SELECT id, 'DEV-' || LPAD(id::text, 4, '0'), 
       CASE WHEN title LIKE '%ذكر%' THEN 'male' 
            WHEN title LIKE '%أنثى%' THEN 'female' 
            ELSE 'unknown' END,
       CURRENT_DATE - INTERVAL '2 years',
       'سلالة تجريبية للتطوير'
FROM products WHERE type = 'pigeon'
ON CONFLICT DO NOTHING;

-- تفاصيل المستلزمات
INSERT INTO supplies (product_id, sku, stock_qty, low_stock_threshold)
SELECT id, 'SKU-' || LPAD(id::text, 4, '0'), 50, 10
FROM products WHERE type = 'supply'
ON CONFLICT DO NOTHING;

-- ========================================
-- 5. مزادات تجريبية
-- ========================================
-- مزاد نشط
INSERT INTO auctions (product_id, start_price, bid_step, reserve_price, start_at, end_at, status)
SELECT id, price_net, 50, price_net * 1.5, 
       NOW() - INTERVAL '1 hour', 
       NOW() + INTERVAL '2 days',
       'live'
FROM products WHERE type = 'pigeon' LIMIT 1
ON CONFLICT DO NOTHING;

-- مزاد مجدول
INSERT INTO auctions (product_id, start_price, bid_step, reserve_price, start_at, end_at, status)
SELECT id, price_net, 25, price_net * 1.3,
       NOW() + INTERVAL '1 day',
       NOW() + INTERVAL '5 days',
       'scheduled'
FROM products WHERE type = 'pigeon' LIMIT 1 OFFSET 1
ON CONFLICT DO NOTHING;

-- ========================================
-- 6. بيانات تجريبية للمزايدات
-- ========================================
INSERT INTO bids (auction_id, user_id, amount, bidder_name_snapshot, bidder_city_id_snapshot)
SELECT a.id, u.id, a.start_price + (50 * ROW_NUMBER() OVER (PARTITION BY a.id ORDER BY u.id)),
       u.name, u.city_id
FROM auctions a
CROSS JOIN users u
WHERE a.status = 'live' AND u.role = 'verified'
LIMIT 3
ON CONFLICT DO NOTHING;

-- ========================================
-- 7. طلبات تجريبية
-- ========================================
-- طلب مكتمل
INSERT INTO orders (user_id, status, subtotal_gross, vat_amount, shipping_fee_gross, grand_total)
SELECT u.id, 'paid', 850.00, 127.50, 28.75, 1006.25
FROM users u WHERE u.email = 'ahmad@test.com'
ON CONFLICT DO NOTHING;

-- فاتورة للطلب
INSERT INTO invoices (order_id, number, status, total_amount, paid_amount)
SELECT o.id, 'INV-2024-0001', 'paid', 1006.25, 1006.25
FROM orders o WHERE o.status = 'paid' LIMIT 1
ON CONFLICT DO NOTHING;

-- ========================================
-- 8. قوالب الإشعارات
-- ========================================
INSERT INTO notification_templates (name, subject, body, language, template_type) VALUES
('welcome_email', 'مرحباً بك في لوفت الدغيري', 'أهلاً {{.Name}}، نرحب بك في منصتنا', 'ar', 'email'),
('bid_outbid', 'تم تجاوز مزايدتك', 'تم تجاوز مزايدتك على {{.ProductTitle}}', 'ar', 'internal'),
('auction_won', 'مبروك! فزت بالمزاد', 'تهانينا {{.Name}}، لقد فزت بالمزاد على {{.ProductTitle}}', 'ar', 'email')
ON CONFLICT DO NOTHING;

-- ========================================
-- ملاحظات مهمة
-- ========================================
-- 1. هذا الملف للتطوير فقط - لا تستخدمه في الإنتاج
-- 2. كلمات المرور الافتراضية:
--    - admin@loft-dughairi.com: admin123
--    - جميع المستخدمين الآخرين: user123
-- 3. يجب تغيير كلمات المرور عند الاستخدام الفعلي
-- 4. البيانات تشمل:
--    - 5 مدن، 6 مستخدمين، 6 منتجات
--    - 2 مزاد (نشط ومجدول)
--    - طلب واحد مع فاتورة مدفوعة
--    - 3 قوالب إشعارات
-- 5. لتشغيل الملف:
--    psql -U postgres -d loft_dughairi_dev -f seed_dev.sql

-- رسالة التأكيد
SELECT 
    'تم تحميل بيانات التطوير بنجاح!' as message,
    (SELECT COUNT(*) FROM users) as total_users,
    (SELECT COUNT(*) FROM products) as total_products,
    (SELECT COUNT(*) FROM auctions) as total_auctions,
    (SELECT COUNT(*) FROM system_settings) as total_settings;
