-- 0007_stock_reservations.up.sql
-- حجوزات المخزون للمستلزمات
-- منصة لوفت الدغيري للحمام الزاجل

-- جدول حجوزات المخزون (للمستلزمات فقط)
CREATE TABLE stock_reservations (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id BIGINT NULL REFERENCES orders(id) ON DELETE SET NULL,
    invoice_id BIGINT NULL REFERENCES invoices(id) ON DELETE SET NULL,
    qty INTEGER NOT NULL CHECK (qty > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    
    -- ملاحظة: سيتم فرض قيد نوع المنتج عبر trigger بدلاً من CHECK constraint
);

-- فهارس لحجوزات المخزون
CREATE INDEX idx_stock_reservations_product_id ON stock_reservations(product_id);
CREATE INDEX idx_stock_reservations_user_id ON stock_reservations(user_id);
CREATE INDEX idx_stock_reservations_order_id ON stock_reservations(order_id) WHERE order_id IS NOT NULL;
CREATE INDEX idx_stock_reservations_invoice_id ON stock_reservations(invoice_id) WHERE invoice_id IS NOT NULL;
CREATE INDEX idx_stock_reservations_expires_at ON stock_reservations(expires_at);
CREATE INDEX idx_stock_reservations_created_at ON stock_reservations(created_at DESC);

-- دالة للتحقق من توفر المخزون قبل الحجز
CREATE OR REPLACE FUNCTION validate_stock_availability()
RETURNS TRIGGER AS $$
DECLARE
    available_stock INTEGER;
    reserved_stock INTEGER;
    pending_orders INTEGER;
    total_reserved INTEGER;
BEGIN
    -- الحصول على المخزون المتاح
    SELECT stock_qty INTO available_stock
    FROM supplies 
    WHERE product_id = NEW.product_id;
    
    IF available_stock IS NULL THEN
        RAISE EXCEPTION 'Product % is not a supply item', NEW.product_id;
    END IF;
    
    -- حساب المخزون المحجوز حالياً
    SELECT COALESCE(SUM(qty), 0) INTO reserved_stock
    FROM stock_reservations
    WHERE product_id = NEW.product_id 
    AND (expires_at > NOW() OR invoice_id IS NOT NULL);
    
    -- حساب الكميات في الطلبات المعلقة
    SELECT COALESCE(SUM(oi.qty), 0) INTO pending_orders
    FROM order_items oi
    JOIN orders o ON oi.order_id = o.id
    WHERE oi.product_id = NEW.product_id 
    AND o.status = 'pending_payment';
    
    -- حساب إجمالي المحجوز
    total_reserved := reserved_stock + pending_orders;
    
    -- التحقق من التوفر (استثناء التحديث للحجز نفسه)
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND OLD.qty != NEW.qty) THEN
        IF TG_OP = 'UPDATE' THEN
            total_reserved := total_reserved - OLD.qty;
        END IF;
        
        IF total_reserved + NEW.qty > available_stock THEN
            RAISE EXCEPTION 'Insufficient stock: available=%, requested=%, already_reserved=%', 
                available_stock, NEW.qty, total_reserved;
        END IF;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger للتحقق من المخزون
CREATE TRIGGER validate_stock_availability_trigger
    BEFORE INSERT OR UPDATE ON stock_reservations
    FOR EACH ROW EXECUTE FUNCTION validate_stock_availability();

-- دالة لتنظيف الحجوزات المنتهية
CREATE OR REPLACE FUNCTION cleanup_expired_reservations()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    -- حذف الحجوزات المنتهية غير المربوطة بفاتورة
    DELETE FROM stock_reservations 
    WHERE expires_at <= NOW() 
    AND invoice_id IS NULL;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- دالة لربط الحجوزات بالفاتورة عند بدء الدفع
CREATE OR REPLACE FUNCTION link_reservations_to_invoice(
    target_user_id BIGINT,
    target_invoice_id BIGINT
)
RETURNS INTEGER AS $$
DECLARE
    linked_count INTEGER;
    session_ttl_minutes INTEGER;
    new_expiry TIMESTAMPTZ;
BEGIN
    -- الحصول على مدة جلسة الدفع
    SELECT CAST(value AS INTEGER) INTO session_ttl_minutes
    FROM system_settings WHERE key = 'payments.session_ttl_minutes';
    
    -- حساب وقت الانتهاء الجديد
    new_expiry := NOW() + (session_ttl_minutes || ' minutes')::INTERVAL;
    
    -- ربط الحجوزات الفعّالة بالفاتورة وتمديد صلاحيتها
    UPDATE stock_reservations 
    SET invoice_id = target_invoice_id,
        expires_at = new_expiry
    WHERE user_id = target_user_id 
    AND expires_at > NOW() 
    AND invoice_id IS NULL;
    
    GET DIAGNOSTICS linked_count = ROW_COUNT;
    
    RETURN linked_count;
END;
$$ LANGUAGE plpgsql;

-- دالة لتحرير الحجوزات عند فشل الدفع
CREATE OR REPLACE FUNCTION release_invoice_reservations(target_invoice_id BIGINT)
RETURNS INTEGER AS $$
DECLARE
    released_count INTEGER;
BEGIN
    -- حذف الحجوزات المربوطة بالفاتورة الفاشلة
    DELETE FROM stock_reservations 
    WHERE invoice_id = target_invoice_id;
    
    GET DIAGNOSTICS released_count = ROW_COUNT;
    
    RETURN released_count;
END;
$$ LANGUAGE plpgsql;
