-- seed_products_auctions.sql
-- ملف Seed شامل للمنتجات والمزادات
-- منصة لوفت الدغيري للحمام الزاجل
-- تاريخ الإنشاء: 2024

-- ========================================
-- إدراج المدن (مطلوبة للمزادات)
-- ========================================
INSERT INTO cities (name_ar, name_en, shipping_fee_net, enabled) VALUES
('الرياض', 'Riyadh', 25.00, true),
('جدة', 'Jeddah', 30.00, true),
('الدمام', 'Dammam', 35.00, true),
('مكة المكرمة', 'Makkah', 28.00, true),
('المدينة المنورة', 'Madinah', 32.00, true),
('تبوك', 'Tabuk', 40.00, true),
('أبها', 'Abha', 38.00, true),
('حائل', 'Hail', 42.00, true),
('القصيم', 'Qassim', 30.00, true),
('الجوف', 'Al-Jouf', 45.00, true)
ON CONFLICT DO NOTHING;

-- ========================================
-- إدراج المستخدمين (مطلوبين للمزادات)
-- ========================================
INSERT INTO users (name, email, phone, password_hash, city_id, role, state, email_verified_at) VALUES
('أحمد الدغيري', 'ahmed@loft-dughairi.com', '+966501234567', '$2a$10$hashed_password_1', 1, 'admin', 'active', NOW()),
('محمد العتيبي', 'mohammed@example.com', '+966502345678', '$2a$10$hashed_password_2', 2, 'verified', 'active', NOW()),
('علي الشمري', 'ali@example.com', '+966503456789', '$2a$10$hashed_password_3', 3, 'verified', 'active', NOW()),
('فهد القحطاني', 'fahad@example.com', '+966504567890', '$2a$10$hashed_password_4', 4, 'verified', 'active', NOW()),
('خالد الحربي', 'khalid@example.com', '+966505678901', '$2a$10$hashed_password_5', 5, 'verified', 'active', NOW()),
('عبدالله المطيري', 'abdullah@example.com', '+966506789012', '$2a$10$hashed_password_6', 6, 'verified', 'active', NOW()),
('سعد الغامدي', 'saad@example.com', '+966507890123', '$2a$10$hashed_password_7', 7, 'verified', 'active', NOW()),
('يوسف الزهراني', 'yousef@example.com', '+966508901234', '$2a$10$hashed_password_8', 8, 'verified', 'active', NOW()),
('عبدالرحمن السبيعي', 'abdulrahman@example.com', '+966509012345', '$2a$10$hashed_password_9', 9, 'verified', 'active', NOW()),
('سلطان العنزي', 'sultan@example.com', '+966500123456', '$2a$10$hashed_password_10', 10, 'verified', 'active', NOW())
ON CONFLICT DO NOTHING;

-- ========================================
-- إدراج المنتجات - الحمام الزاجل
-- ========================================
INSERT INTO products (type, title, slug, description, price_net, status) VALUES
-- حمام زاجل ذكور
('pigeon', 'حمام زاجل أصيل - ذكر سلالة الدغيري', 'pigeon-male-dughairi-001', 'حمام زاجل أصيل من سلالة الدغيري المميزة، مدرب على المسافات الطويلة، عمر 2 سنة، لون أبيض مع بقع سوداء', 800.00, 'available'),
('pigeon', 'حمام زاجل ذكر - سلالة العتيبي', 'pigeon-male-utaybi-001', 'حمام زاجل من سلالة العتيبي الأصيلة، مدرب على المسافات المتوسطة، عمر 18 شهر، لون رمادي فاتح', 650.00, 'available'),
('pigeon', 'حمام زاجل ذكر - سلالة الشمري', 'pigeon-male-shamri-001', 'حمام زاجل من سلالة الشمري المميزة، مدرب على المسافات القصيرة والمتوسطة، عمر 3 سنوات، لون بني محمر', 900.00, 'available'),
('pigeon', 'حمام زاجل ذكر - سلالة القحطاني', 'pigeon-male-qahthani-001', 'حمام زاجل من سلالة القحطاني الأصيلة، مدرب على المسافات الطويلة، عمر 2.5 سنة، لون أبيض نقي', 750.00, 'available'),
('pigeon', 'حمام زاجل ذكر - سلالة الحربي', 'pigeon-male-harbi-001', 'حمام زاجل من سلالة الحربي المميزة، مدرب على المسافات المتوسطة، عمر 20 شهر، لون أسود مع بقع بيضاء', 700.00, 'available'),

-- حمام زاجل إناث
('pigeon', 'حمام زاجل أنثى - سلالة الدغيري', 'pigeon-female-dughairi-001', 'حمام زاجل أنثى من سلالة الدغيري الأصيلة، مناسبة للتربية والتكاثر، عمر 2.5 سنة، لون أبيض مع بقع رمادية', 850.00, 'available'),
('pigeon', 'حمام زاجل أنثى - سلالة العتيبي', 'pigeon-female-utaybi-001', 'حمام زاجل أنثى من سلالة العتيبي المميزة، مناسبة للتربية، عمر 22 شهر، لون رمادي غامق', 700.00, 'available'),
('pigeon', 'حمام زاجل أنثى - سلالة الشمري', 'pigeon-female-shamri-001', 'حمام زاجل أنثى من سلالة الشمري الأصيلة، مناسبة للتربية والتكاثر، عمر 2.8 سنة، لون بني فاتح', 800.00, 'available'),
('pigeon', 'حمام زاجل أنثى - سلالة القحطاني', 'pigeon-female-qahthani-001', 'حمام زاجل أنثى من سلالة القحطاني المميزة، مناسبة للتربية، عمر 2.2 سنة، لون أبيض مع بقع سوداء', 750.00, 'available'),
('pigeon', 'حمام زاجل أنثى - سلالة الحربي', 'pigeon-female-harbi-001', 'حمام زاجل أنثى من سلالة الحربي الأصيلة، مناسبة للتربية والتكاثر، عمر 2.6 سنة، لون أسود مع بقع بيضاء', 720.00, 'available'),

-- حمام زاجل صغار
('pigeon', 'حمام زاجل صغير - ذكر', 'pigeon-young-male-001', 'حمام زاجل صغير ذكر، عمر 6 أشهر، مدرب على المسافات القصيرة، لون أبيض نقي، مناسب للمبتدئين', 400.00, 'available'),
('pigeon', 'حمام زاجل صغير - أنثى', 'pigeon-young-female-001', 'حمام زاجل صغير أنثى، عمر 7 أشهر، مدرب على المسافات القصيرة، لون رمادي فاتح، مناسب للمبتدئين', 420.00, 'available'),
('pigeon', 'حمام زاجل صغير - ذكر', 'pigeon-young-male-002', 'حمام زاجل صغير ذكر، عمر 8 أشهر، مدرب على المسافات القصيرة، لون بني محمر، مناسب للمبتدئين', 380.00, 'available'),
('pigeon', 'حمام زاجل صغير - أنثى', 'pigeon-young-female-002', 'حمام زاجل صغير أنثى، عمر 6.5 أشهر، مدرب على المسافات القصيرة، لون أبيض مع بقع سوداء، مناسب للمبتدئين', 400.00, 'available'),
('pigeon', 'حمام زاجل صغير - ذكر', 'pigeon-young-male-003', 'حمام زاجل صغير ذكر، عمر 7.5 أشهر، مدرب على المسافات القصيرة، لون أسود مع بقع بيضاء، مناسب للمبتدئين', 390.00, 'available')
ON CONFLICT DO NOTHING;

-- ========================================
-- إدراج تفاصيل الحمام
-- ========================================
INSERT INTO pigeons (product_id, ring_number, sex, birth_date, lineage) VALUES
-- حمام ذكور
(1, 'SA-2022-001', 'male', '2022-03-15', 'سلالة الدغيري الأصيلة - الجيل الثالث'),
(2, 'SA-2022-002', 'male', '2022-06-20', 'سلالة العتيبي الأصيلة - الجيل الثاني'),
(3, 'SA-2021-003', 'male', '2021-09-10', 'سلالة الشمري المميزة - الجيل الرابع'),
(4, 'SA-2021-004', 'male', '2021-12-05', 'سلالة القحطاني الأصيلة - الجيل الثالث'),
(5, 'SA-2022-005', 'male', '2022-04-18', 'سلالة الحربي المميزة - الجيل الثاني'),

-- حمام إناث
(6, 'SA-2021-006', 'female', '2021-10-12', 'سلالة الدغيري الأصيلة - الجيل الثالث'),
(7, 'SA-2022-007', 'female', '2022-05-25', 'سلالة العتيبي المميزة - الجيل الثاني'),
(8, 'SA-2021-008', 'female', '2021-07-30', 'سلالة الشمري الأصيلة - الجيل الرابع'),
(9, 'SA-2022-009', 'female', '2022-01-15', 'سلالة القحطاني المميزة - الجيل الثالث'),
(10, 'SA-2021-010', 'female', '2021-11-08', 'سلالة الحربي الأصيلة - الجيل الثاني'),

-- حمام صغار
(11, 'SA-2023-011', 'male', '2023-09-15', 'سلالة مختلطة - جيل أول'),
(12, 'SA-2023-012', 'female', '2023-08-20', 'سلالة مختلطة - جيل أول'),
(13, 'SA-2023-013', 'male', '2023-07-10', 'سلالة مختلطة - جيل أول'),
(14, 'SA-2023-014', 'female', '2023-09-25', 'سلالة مختلطة - جيل أول'),
(15, 'SA-2023-015', 'male', '2023-08-05', 'سلالة مختلطة - جيل أول')
ON CONFLICT DO NOTHING;

-- ========================================
-- إدراج المنتجات - المستلزمات
-- ========================================
INSERT INTO products (type, title, slug, description, price_net, status) VALUES
-- علف الحمام
('supply', 'علف حمام فاخر - 25 كيلو', 'pigeon-feed-premium-25kg', 'علف حمام عالي الجودة مخصص للحمام الزاجل، يحتوي على جميع العناصر الغذائية المطلوبة', 75.00, 'available'),
('supply', 'علف حمام عادي - 25 كيلو', 'pigeon-feed-standard-25kg', 'علف حمام عادي مناسب للاستخدام اليومي، جودة متوسطة', 55.00, 'available'),
('supply', 'علف حمام صغار - 15 كيلو', 'pigeon-feed-young-15kg', 'علف حمام مخصص للصغار، يحتوي على بروتين عالي', 65.00, 'available'),
('supply', 'علف حمام تربية - 20 كيلو', 'pigeon-feed-breeding-20kg', 'علف حمام مخصص لفترة التربية والتكاثر', 80.00, 'available'),
('supply', 'علف حمام مسابقات - 30 كيلو', 'pigeon-feed-racing-30kg', 'علف حمام مخصص للمسابقات والتدريب، طاقة عالية', 95.00, 'available'),

-- فيتامينات ومكملات
('supply', 'فيتامينات حمام شاملة - 500 مل', 'pigeon-vitamins-complete-500ml', 'فيتامينات شاملة للحمام، تحتوي على جميع الفيتامينات الأساسية', 45.00, 'available'),
('supply', 'مكمل بروتين حمام - 250 جرام', 'pigeon-protein-supplement-250g', 'مكمل بروتين عالي الجودة للحمام النشط', 35.00, 'available'),
('supply', 'مكمل طاقة حمام - 300 جرام', 'pigeon-energy-supplement-300g', 'مكمل طاقة للحمام المشارك في المسابقات', 40.00, 'available'),
('supply', 'مكمل مناعة حمام - 400 مل', 'pigeon-immunity-supplement-400ml', 'مكمل لتعزيز مناعة الحمام', 50.00, 'available'),
('supply', 'مكمل هضم حمام - 200 جرام', 'pigeon-digestive-supplement-200g', 'مكمل لتحسين عملية الهضم', 30.00, 'available'),

-- مستلزمات الأقفاص
('supply', 'قفص حمام كبير - 60×40×40 سم', 'pigeon-cage-large-60x40x40', 'قفص حمام كبير مصنوع من الفولاذ المقاوم للصدأ', 120.00, 'available'),
('supply', 'قفص حمام متوسط - 50×35×35 سم', 'pigeon-cage-medium-50x35x35', 'قفص حمام متوسط مصنوع من الفولاذ المقاوم للصدأ', 95.00, 'available'),
('supply', 'قفص حمام صغير - 40×30×30 سم', 'pigeon-cage-small-40x30x30', 'قفص حمام صغير مصنوع من الفولاذ المقاوم للصدأ', 75.00, 'available'),
('supply', 'قفص حمام تربية - 80×50×50 سم', 'pigeon-cage-breeding-80x50x50', 'قفص حمام كبير مخصص للتربية والتكاثر', 180.00, 'available'),
('supply', 'قفص حمام مسابقات - 100×60×60 سم', 'pigeon-cage-racing-100x60x60', 'قفص حمام كبير مخصص للمسابقات', 220.00, 'available'),

-- مستلزمات النظافة
('supply', 'مطهر أقفاص حمام - 1 لتر', 'pigeon-cage-disinfectant-1l', 'مطهر فعال لتنظيف وتعقيم أقفاص الحمام', 25.00, 'available'),
('supply', 'منظف حمام - 500 مل', 'pigeon-cleaner-500ml', 'منظف لطيف على الحمام، آمن للاستخدام', 20.00, 'available'),
('supply', 'مطهر مياه حمام - 250 مل', 'pigeon-water-disinfectant-250ml', 'مطهر لمياه الشرب، يقتل البكتيريا الضارة', 15.00, 'available'),
('supply', 'معطر أقفاص حمام - 300 مل', 'pigeon-cage-freshener-300ml', 'معطر طبيعي لأقفاص الحمام', 18.00, 'available'),
('supply', 'منظف حمام طبيعي - 400 مل', 'pigeon-natural-cleaner-400ml', 'منظف طبيعي 100%، آمن تماماً', 22.00, 'available'),

-- مستلزمات التدريب
('supply', 'سوار هوية حمام - 10 قطع', 'pigeon-id-band-10pcs', 'أساور هوية بلاستيكية للحمام، 10 قطع', 12.00, 'available'),
('supply', 'سوار هوية حمام معدني - 5 قطع', 'pigeon-metal-id-band-5pcs', 'أساور هوية معدنية دائمة للحمام، 5 قطع', 25.00, 'available'),
('supply', 'مقياس وزن حمام - 5 كيلو', 'pigeon-weight-scale-5kg', 'مقياس وزن دقيق للحمام، سعة 5 كيلو', 85.00, 'available'),
('supply', 'مقياس وزن حمام - 10 كيلو', 'pigeon-weight-scale-10kg', 'مقياس وزن دقيق للحمام، سعة 10 كيلو', 120.00, 'available'),
('supply', 'ساعة توقيت حمام - رقمية', 'pigeon-digital-timer', 'ساعة توقيت رقمية لتدريب الحمام', 45.00, 'available')
ON CONFLICT DO NOTHING;

-- ========================================
-- إدراج تفاصيل المستلزمات
-- ========================================
INSERT INTO supplies (product_id, sku, stock_qty, low_stock_threshold) VALUES
-- علف الحمام
(16, 'FEED-PREM-25', 50, 10),
(17, 'FEED-STD-25', 75, 15),
(18, 'FEED-YOUNG-15', 40, 8),
(19, 'FEED-BREED-20', 35, 7),
(20, 'FEED-RACE-30', 30, 6),

-- فيتامينات ومكملات
(21, 'VIT-COMP-500', 25, 5),
(22, 'PROT-SUP-250', 30, 6),
(23, 'ENERGY-SUP-300', 28, 6),
(24, 'IMM-SUP-400', 22, 5),
(25, 'DIG-SUP-200', 35, 7),

-- مستلزمات الأقفاص
(26, 'CAGE-LG-60x40', 15, 3),
(27, 'CAGE-MD-50x35', 20, 4),
(28, 'CAGE-SM-40x30', 25, 5),
(29, 'CAGE-BREED-80x50', 10, 2),
(30, 'CAGE-RACE-100x60', 8, 2),

-- مستلزمات النظافة
(31, 'DIS-CAGE-1L', 40, 8),
(32, 'CLEAN-500', 45, 9),
(33, 'DIS-WATER-250', 50, 10),
(34, 'FRESH-300', 38, 8),
(35, 'NAT-CLEAN-400', 42, 9),

-- مستلزمات التدريب
(36, 'ID-BAND-10', 100, 20),
(37, 'ID-METAL-5', 50, 10),
(38, 'SCALE-5KG', 12, 3),
(39, 'SCALE-10KG', 8, 2),
(40, 'TIMER-DIG', 20, 4)
ON CONFLICT DO NOTHING;

-- ========================================
-- إدراج المزادات
-- ========================================
INSERT INTO auctions (product_id, start_price, bid_step, reserve_price, start_at, end_at, anti_sniping_minutes, status, extensions_count, max_extensions_override) VALUES
-- مزاد حمام ذكر سلالة الدغيري
(1, 800.00, 50, 1000.00, NOW() + INTERVAL '1 day', NOW() + INTERVAL '7 days', 15, 'scheduled', 0, 3),
-- مزاد حمام ذكر سلالة العتيبي
(2, 650.00, 25, 800.00, NOW() + INTERVAL '2 days', NOW() + INTERVAL '5 days', 10, 'scheduled', 0, 2),
-- مزاد حمام ذكر سلالة الشمري
(3, 900.00, 75, 1200.00, NOW() + INTERVAL '3 days', NOW() + INTERVAL '10 days', 20, 'scheduled', 0, 4),
-- مزاد حمام أنثى سلالة الدغيري
(6, 850.00, 50, 1100.00, NOW() + INTERVAL '4 days', NOW() + INTERVAL '8 days', 15, 'scheduled', 0, 3),
-- مزاد حمام أنثى سلالة العتيبي
(7, 700.00, 30, 900.00, NOW() + INTERVAL '5 days', NOW() + INTERVAL '6 days', 12, 'scheduled', 0, 2)
ON CONFLICT DO NOTHING;

-- ========================================
-- إدراج المزايدات (للمزادات النشطة)
-- ========================================
-- إنشاء مزاد نشط للمزاد الأول
UPDATE auctions SET status = 'live', start_at = NOW() - INTERVAL '1 hour' WHERE id = 1;

-- إدراج مزايدات للمزاد النشط
INSERT INTO bids (auction_id, user_id, amount, bidder_name_snapshot, bidder_city_id_snapshot) VALUES
(1, 2, 850.00, 'محمد العتيبي', 2),
(1, 3, 900.00, 'علي الشمري', 3),
(1, 4, 950.00, 'فهد القحطاني', 4),
(1, 5, 1000.00, 'خالد الحربي', 5),
(1, 6, 1050.00, 'عبدالله المطيري', 6),
(1, 7, 1100.00, 'سعد الغامدي', 7),
(1, 8, 1150.00, 'يوسف الزهراني', 8),
(1, 9, 1200.00, 'عبدالرحمن السبيعي', 9),
(1, 10, 1250.00, 'سلطان العنزي', 10)
ON CONFLICT DO NOTHING;

-- ========================================
-- إدراج الوسائط (صور تجريبية)
-- ========================================
INSERT INTO media (product_id, kind, gcs_path, thumb_path, watermark_applied, file_size, mime_type, original_filename) VALUES
-- صور الحمام
(1, 'image', 'products/pigeons/pigeon-001-main.jpg', 'products/pigeons/pigeon-001-thumb.jpg', true, 2048576, 'image/jpeg', 'pigeon-001-main.jpg'),
(1, 'image', 'products/pigeons/pigeon-001-side.jpg', 'products/pigeons/pigeon-001-side-thumb.jpg', true, 1856320, 'image/jpeg', 'pigeon-001-side.jpg'),
(1, 'image', 'products/pigeons/pigeon-001-front.jpg', 'products/pigeons/pigeon-001-front-thumb.jpg', true, 1987456, 'image/jpeg', 'pigeon-001-front.jpg'),
(2, 'image', 'products/pigeons/pigeon-002-main.jpg', 'products/pigeons/pigeon-002-thumb.jpg', true, 2150400, 'image/jpeg', 'pigeon-002-main.jpg'),
(3, 'image', 'products/pigeons/pigeon-003-main.jpg', 'products/pigeons/pigeon-003-thumb.jpg', true, 1925120, 'image/jpeg', 'pigeon-003-main.jpg'),

-- صور المستلزمات
(16, 'image', 'products/supplies/feed-premium-main.jpg', 'products/supplies/feed-premium-thumb.jpg', false, 1048576, 'image/jpeg', 'feed-premium-main.jpg'),
(16, 'image', 'products/supplies/feed-premium-package.jpg', 'products/supplies/feed-premium-package-thumb.jpg', false, 983040, 'image/jpeg', 'feed-premium-package.jpg'),
(26, 'image', 'products/supplies/cage-large-main.jpg', 'products/supplies/cage-large-thumb.jpg', false, 1572864, 'image/jpeg', 'cage-large-main.jpg'),
(26, 'image', 'products/supplies/cage-large-detail.jpg', 'products/supplies/cage-large-detail-thumb.jpg', false, 1310720, 'image/jpeg', 'cage-large-detail.jpg'),
(31, 'image', 'products/supplies/disinfectant-main.jpg', 'products/supplies/disinfectant-thumb.jpg', false, 786432, 'image/jpeg', 'disinfectant-main.jpg')
ON CONFLICT DO NOTHING;

-- ========================================
-- تحديث حالة المنتجات في المزادات
-- ========================================
UPDATE products SET status = 'in_auction' WHERE id IN (1, 2, 3, 6, 7);

-- ========================================
-- رسالة تأكيد
-- ========================================
SELECT 'تم إدراج البيانات بنجاح!' as message,
       COUNT(*) FILTER (WHERE type = 'pigeon') as total_pigeons,
       COUNT(*) FILTER (WHERE type = 'supply') as total_supplies,
       COUNT(*) FILTER (WHERE status = 'in_auction') as total_auctions
FROM products;
