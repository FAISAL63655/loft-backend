-- 0006_shipping.up.sql
-- شركات الشحن والشحنات
-- منصة لوفت الدغيري للحمام الزاجل

-- إنشاء أنواع البيانات للشحن
CREATE TYPE delivery_method AS ENUM ('courier', 'pickup');
CREATE TYPE shipment_status AS ENUM (
    'pending', 'processing', 'shipped', 
    'delivered', 'failed', 'returned'
);

-- جدول شركات الشحن
CREATE TABLE shipping_companies (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهارس لشركات الشحن
CREATE INDEX idx_shipping_companies_enabled ON shipping_companies(enabled) WHERE enabled = true;

-- جدول الشحنات
CREATE TABLE shipments (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    company_id BIGINT REFERENCES shipping_companies(id),
    delivery_method delivery_method NOT NULL DEFAULT 'courier',
    status shipment_status NOT NULL DEFAULT 'pending',
    tracking_ref TEXT,
    events JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(events) = 'array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    
    -- ملاحظة: سيتم فرض قيد الطلب المدفوع عبر trigger بدلاً من CHECK constraint
);

-- فهارس للشحنات
CREATE INDEX idx_shipments_order_id ON shipments(order_id);
CREATE INDEX idx_shipments_company_id ON shipments(company_id);
CREATE INDEX idx_shipments_status ON shipments(status);
CREATE INDEX idx_shipments_tracking_ref ON shipments(tracking_ref) WHERE tracking_ref IS NOT NULL;
CREATE INDEX idx_shipments_created_at ON shipments(created_at DESC);

-- إضافة triggers لتحديث updated_at
CREATE TRIGGER update_shipping_companies_updated_at 
    BEFORE UPDATE ON shipping_companies 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_shipments_updated_at 
    BEFORE UPDATE ON shipments 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- دالة لإضافة حدث شحن جديد
CREATE OR REPLACE FUNCTION add_shipment_event(
    shipment_id BIGINT,
    new_status shipment_status,
    note TEXT DEFAULT NULL
)
RETURNS VOID AS $$
DECLARE
    event_data JSONB;
BEGIN
    -- إنشاء بيانات الحدث
    event_data := jsonb_build_object(
        'at', NOW()::TEXT,
        'status', new_status::TEXT,
        'note', note
    );
    
    -- إضافة الحدث إلى قائمة الأحداث
    UPDATE shipments
    SET events = events || jsonb_build_array(event_data), -- ضمان إضافة كعنصر ضمن مصفوفة
        status = new_status,
        updated_at = NOW()
    WHERE id = shipment_id;
END;
$$ LANGUAGE plpgsql;

-- دالة لتحديث حالة الشحنة مع إضافة حدث تلقائي
CREATE OR REPLACE FUNCTION update_shipment_status()
RETURNS TRIGGER AS $$
BEGIN
    -- إذا تغيرت الحالة، أضف حدث تلقائي
    IF OLD.status != NEW.status THEN
        NEW.events := OLD.events || jsonb_build_array(jsonb_build_object(
            'at', NOW()::TEXT,
            'status', NEW.status::TEXT,
            'note', 'Status updated automatically'
        ));
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger لتحديث حالة الشحنة
CREATE TRIGGER update_shipment_status_trigger
    BEFORE UPDATE ON shipments
    FOR EACH ROW EXECUTE FUNCTION update_shipment_status();

-- إدراج شركات الشحن الأولية
INSERT INTO shipping_companies (name, enabled) VALUES
('شركة الشحن السريع', true),
('البريد السعودي', true),
('شركة النقل المتطور', true),
('خدمات التوصيل المميز', false);  -- معطلة كمثال
