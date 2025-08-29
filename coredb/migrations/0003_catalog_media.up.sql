-- 0003_catalog_media.up.sql
-- الكتالوج والوسائط والمنتجات
-- منصة لوفت الدغيري للحمام الزاجل

-- إنشاء أنواع البيانات للمنتجات
CREATE TYPE product_type AS ENUM ('pigeon', 'supply');
CREATE TYPE product_status AS ENUM (
    'available', 'reserved', 'payment_in_progress', 
    'in_auction', 'auction_hold', 'sold', 
    'out_of_stock', 'archived'
);
CREATE TYPE pigeon_sex AS ENUM ('male', 'female', 'unknown');
CREATE TYPE media_kind AS ENUM ('image', 'video', 'file');

-- جدول المنتجات الرئيسي
CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    type product_type NOT NULL,
    title TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    description TEXT,
    price_net NUMERIC(12,2) NOT NULL DEFAULT 0.00 CHECK (price_net >= 0),
    status product_status NOT NULL DEFAULT 'available',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهارس للمنتجات (slug لديه UNIQUE constraint فلا نحتاج فهرس منفصل)
CREATE INDEX idx_products_type ON products(type);
CREATE INDEX idx_products_status ON products(status);
CREATE INDEX idx_products_created_at ON products(created_at DESC);
CREATE INDEX idx_products_type_status ON products(type, status);

-- فهرس للمنتجات المتاحة والنشطة
CREATE INDEX idx_products_available ON products(status, created_at DESC) 
WHERE status IN ('available', 'in_auction');

-- جدول تفاصيل الحمام
CREATE TABLE pigeons (
    product_id BIGINT PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
    ring_number TEXT UNIQUE NOT NULL,
    sex pigeon_sex NOT NULL DEFAULT 'unknown',
    birth_date DATE,
    lineage TEXT,
    origin_proof_url TEXT,
    origin_proof_file_ref TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهارس للحمام (ring_number لديه UNIQUE constraint فلا نحتاج فهرس منفصل)
CREATE INDEX idx_pigeons_sex ON pigeons(sex);
CREATE INDEX idx_pigeons_birth_date ON pigeons(birth_date);

-- جدول تفاصيل المستلزمات
CREATE TABLE supplies (
    product_id BIGINT PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
    sku TEXT NULL,
    stock_qty INTEGER NOT NULL DEFAULT 0,
    low_stock_threshold INTEGER NOT NULL DEFAULT 5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- قيد للتأكد من أن الكمية لا تقل عن الصفر
    CONSTRAINT chk_supplies_stock_qty_non_negative CHECK (stock_qty >= 0),
    CONSTRAINT chk_supplies_threshold_positive CHECK (low_stock_threshold > 0)
);

-- فهارس للمستلزمات
CREATE INDEX idx_supplies_sku ON supplies(sku) WHERE sku IS NOT NULL;
CREATE INDEX idx_supplies_stock_qty ON supplies(stock_qty);
CREATE INDEX idx_supplies_low_stock ON supplies(stock_qty, low_stock_threshold) 
WHERE stock_qty <= low_stock_threshold;

-- جدول الوسائط
CREATE TABLE media (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    kind media_kind NOT NULL,
    gcs_path TEXT NOT NULL,
    thumb_path TEXT NULL,
    watermark_applied BOOLEAN NOT NULL DEFAULT false,
    file_size BIGINT,
    mime_type TEXT,
    original_filename TEXT,
    archived_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهارس للوسائط
CREATE INDEX idx_media_product_id ON media(product_id);
CREATE INDEX idx_media_kind ON media(kind);

-- فهرس جزئي للوسائط غير المؤرشفة (كما هو مطلوب في PRD)
CREATE INDEX idx_media_product_unarchived ON media(product_id, created_at DESC) 
WHERE archived_at IS NULL;

-- إضافة triggers لتحديث updated_at
CREATE TRIGGER update_products_updated_at 
    BEFORE UPDATE ON products 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_pigeons_updated_at 
    BEFORE UPDATE ON pigeons 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_supplies_updated_at 
    BEFORE UPDATE ON supplies 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_media_updated_at 
    BEFORE UPDATE ON media 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- دالة لتحديث حالة المستلزمات عند نفاد المخزون
CREATE OR REPLACE FUNCTION update_supply_status()
RETURNS TRIGGER AS $$
BEGIN
    -- إذا وصلت الكمية إلى صفر، غيّر حالة المنتج إلى out_of_stock
    IF NEW.stock_qty = 0 AND OLD.stock_qty > 0 THEN
        UPDATE products 
        SET status = 'out_of_stock' 
        WHERE id = NEW.product_id AND type = 'supply';
    -- إذا أصبحت الكمية أكبر من صفر، غيّر الحالة إلى available
    ELSIF NEW.stock_qty > 0 AND OLD.stock_qty = 0 THEN
        UPDATE products 
        SET status = 'available' 
        WHERE id = NEW.product_id AND type = 'supply';
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger لتحديث حالة المستلزمات
CREATE TRIGGER update_supply_status_trigger
    AFTER UPDATE ON supplies
    FOR EACH ROW EXECUTE FUNCTION update_supply_status();

-- دالة للتحقق من صحة حالات المنتجات
CREATE OR REPLACE FUNCTION validate_product_status()
RETURNS TRIGGER AS $$
BEGIN
    -- التحقق من أن حالات معينة مخصصة للحمام فقط
    IF NEW.type = 'supply' AND NEW.status IN ('reserved', 'payment_in_progress', 'sold', 'auction_hold', 'in_auction') THEN
        RAISE EXCEPTION 'Status % is not valid for supply products', NEW.status;
    END IF;
    
    -- التحقق من أن out_of_stock مخصص للمستلزمات فقط
    IF NEW.type = 'pigeon' AND NEW.status = 'out_of_stock' THEN
        RAISE EXCEPTION 'Status out_of_stock is not valid for pigeon products';
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger للتحقق من حالات المنتجات
CREATE TRIGGER validate_product_status_trigger
    BEFORE INSERT OR UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION validate_product_status();

-- إدراج بيانات تجريبية (اختياري للتطوير)
-- يمكن حذف هذا القسم في الإنتاج

-- منتج حمام تجريبي
INSERT INTO products (type, title, slug, description, price_net, status) VALUES
('pigeon', 'حمام زاجل أصيل - ذكر', 'pigeon-male-001', 'حمام زاجل أصيل من سلالة ممتازة، مدرب على المسافات الطويلة', 500.00, 'available');

-- تفاصيل الحمام
INSERT INTO pigeons (product_id, ring_number, sex, birth_date, lineage) VALUES
(1, 'SA-2024-001', 'male', '2024-01-15', 'سلالة الدغيري الأصيلة');

-- منتج مستلزمات تجريبي
INSERT INTO products (type, title, slug, description, price_net, status) VALUES
('supply', 'علف حمام فاخر - 25 كيلو', 'pigeon-feed-premium-25kg', 'علف حمام عالي الجودة مخصص للحمام الزاجل', 75.00, 'available');

-- تفاصيل المستلزمات
INSERT INTO supplies (product_id, sku, stock_qty, low_stock_threshold) VALUES
(2, 'FEED-PREM-25', 50, 10);