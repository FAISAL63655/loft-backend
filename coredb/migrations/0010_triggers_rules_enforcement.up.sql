-- 0010_triggers_rules_enforcement.up.sql
-- القوانين والمحفزات لفرض قواعد العمل
-- منصة لوفت الدغيري للحمام الزاجل

-- دالة لفرض قواعد حالات المنتجات عند تغيير الحالة
CREATE OR REPLACE FUNCTION enforce_product_state_transitions()
RETURNS TRIGGER AS $$
DECLARE
    active_auction_count INTEGER;
    pending_order_count INTEGER;
BEGIN
    -- التحقق من الانتقالات المسموحة للحمام
    IF NEW.type = 'pigeon' THEN
        -- منع الانتقال إلى in_auction إذا كان هناك مزاد نشط
        IF NEW.status = 'in_auction' AND OLD.status != 'in_auction' THEN
            SELECT COUNT(*) INTO active_auction_count
            FROM auctions 
            WHERE product_id = NEW.id 
            AND status IN ('scheduled', 'live');
            
            IF active_auction_count = 0 THEN
                RAISE EXCEPTION 'Cannot set product to in_auction without active auction';
            END IF;
        END IF;
        
        -- منع الانتقال إلى reserved إذا كان هناك طلب معلق
        IF NEW.status = 'reserved' AND OLD.status = 'available' THEN
            SELECT COUNT(*) INTO pending_order_count
            FROM orders o
            JOIN order_items oi ON o.id = oi.order_id
            WHERE oi.product_id = NEW.id 
            AND o.status = 'pending_payment';
            
            IF pending_order_count > 0 THEN
                RAISE EXCEPTION 'Product already has pending order';
            END IF;
        END IF;
        
        -- منع الانتقال المباشر من available إلى sold
        IF OLD.status = 'available' AND NEW.status = 'sold' THEN
            RAISE EXCEPTION 'Cannot transition directly from available to sold';
        END IF;
    END IF;
    
    -- قواعد خاصة بالمستلزمات
    IF NEW.type = 'supply' THEN
        -- منع استخدام حالات الحمام للمستلزمات
        IF NEW.status IN ('reserved', 'payment_in_progress', 'auction_hold', 'in_auction') THEN
            RAISE EXCEPTION 'Status % not valid for supply products', NEW.status;
        END IF;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger لفرض قواعد الحالات
CREATE TRIGGER enforce_product_state_transitions_trigger
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION enforce_product_state_transitions();

-- دالة لتحديث حالة المنتج عند تغيير حالة المزاد
CREATE OR REPLACE FUNCTION sync_product_auction_status()
RETURNS TRIGGER AS $$
BEGIN
    -- عند بدء المزاد
    IF OLD.status != 'live' AND NEW.status = 'live' THEN
        UPDATE products 
        SET status = 'in_auction' 
        WHERE id = NEW.product_id;
    END IF;
    
    -- عند انتهاء المزاد
    IF OLD.status = 'live' AND NEW.status = 'ended' THEN
        -- التحقق من وجود فائز
        IF EXISTS (
            SELECT 1 FROM bids 
            WHERE auction_id = NEW.id 
            AND amount >= COALESCE(NEW.reserve_price, 0)
        ) THEN
            UPDATE products 
            SET status = 'auction_hold' 
            WHERE id = NEW.product_id;
        ELSE
            UPDATE products 
            SET status = 'available' 
            WHERE id = NEW.product_id;
        END IF;
    END IF;
    
    -- عند إلغاء المزاد
    IF NEW.status = 'cancelled' THEN
        UPDATE products 
        SET status = 'available' 
        WHERE id = NEW.product_id;
    END IF;
    
    -- عند وسم الفائز كغير مسدد
    IF NEW.status = 'winner_unpaid' THEN
        UPDATE products 
        SET status = 'available' 
        WHERE id = NEW.product_id;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger لمزامنة حالة المنتج مع المزاد
CREATE TRIGGER sync_product_auction_status_trigger
    AFTER UPDATE ON auctions
    FOR EACH ROW EXECUTE FUNCTION sync_product_auction_status();

-- دالة لتحديث حالة الطلب عند تغيير حالة الفاتورة
CREATE OR REPLACE FUNCTION sync_order_invoice_status()
RETURNS TRIGGER AS $$
BEGIN
    -- عند دفع الفاتورة
    IF OLD.status != 'paid' AND NEW.status = 'paid' THEN
        UPDATE orders 
        SET status = 'paid' 
        WHERE id = NEW.order_id;
        
        -- تحديث حالة المنتجات في الطلب إلى sold
        UPDATE products 
        SET status = 'sold' 
        WHERE id IN (
            SELECT oi.product_id 
            FROM order_items oi 
            WHERE oi.order_id = NEW.order_id
            AND (SELECT type FROM products WHERE id = oi.product_id) = 'pigeon'
        );
        
        -- تقليل مخزون المستلزمات
        UPDATE supplies 
        SET stock_qty = stock_qty - oi.qty
        FROM order_items oi
        WHERE supplies.product_id = oi.product_id
        AND oi.order_id = NEW.order_id
        AND EXISTS (
            SELECT 1 FROM products p 
            WHERE p.id = oi.product_id AND p.type = 'supply'
        );
    END IF;
    
    -- عند فشل الفاتورة
    IF NEW.status = 'failed' THEN
        UPDATE orders 
        SET status = 'cancelled' 
        WHERE id = NEW.order_id;
        
        -- تحرير المنتجات المحجوزة
        UPDATE products 
        SET status = 'available' 
        WHERE id IN (
            SELECT oi.product_id 
            FROM order_items oi 
            WHERE oi.order_id = NEW.order_id
            AND (SELECT type FROM products WHERE id = oi.product_id) = 'pigeon'
            AND (SELECT status FROM products WHERE id = oi.product_id) IN ('reserved', 'payment_in_progress')
        );
    END IF;
    
    -- عند طلب الاسترداد
    IF NEW.status = 'refund_required' THEN
        UPDATE orders 
        SET status = 'refund_required' 
        WHERE id = NEW.order_id;
    END IF;
    
    -- عند اكتمال الاسترداد
    IF NEW.status = 'refunded' THEN
        UPDATE orders 
        SET status = 'refunded' 
        WHERE id = NEW.order_id;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger لمزامنة حالة الطلب مع الفاتورة
CREATE TRIGGER sync_order_invoice_status_trigger
    AFTER UPDATE ON invoices
    FOR EACH ROW EXECUTE FUNCTION sync_order_invoice_status();

-- دالة لمنع حذف المستخدمين الذين لديهم بيانات نشطة مرتبطة بهم
CREATE OR REPLACE FUNCTION prevent_user_deletion_with_active_data()
RETURNS TRIGGER AS $$
DECLARE
    active_orders INTEGER;
    live_bids INTEGER;
BEGIN
    -- التحقق من الطلبات النشطة (غير منتهية بالكامل)
    SELECT COUNT(*) INTO active_orders
    FROM orders
    WHERE user_id = OLD.id
    AND status NOT IN ('paid', 'cancelled', 'refunded');

    -- التحقق من وجود مزايدات للمستخدم على مزادات حية فقط
    SELECT COUNT(*) INTO live_bids
    FROM bids b
    JOIN auctions a ON b.auction_id = a.id
    WHERE b.user_id = OLD.id
    AND a.status = 'live';

    IF active_orders > 0 OR live_bids > 0 THEN
        RAISE EXCEPTION 'Cannot delete user with active orders or live bids';
    END IF;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger لمنع حذف المستخدمين النشطين
CREATE TRIGGER prevent_user_deletion_with_active_data_trigger
    BEFORE DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION prevent_user_deletion_with_active_data();

-- دالة لتسجيل العمليات الحساسة تلقائياً
CREATE OR REPLACE FUNCTION auto_audit_sensitive_operations()
RETURNS TRIGGER AS $$
DECLARE
    action_name TEXT;
    entity_type_name TEXT;
    entity_id_val TEXT;
    actor_id BIGINT;
BEGIN
    -- تحديد نوع العملية والكيان
    CASE TG_TABLE_NAME
        WHEN 'bids' THEN
            entity_type_name := 'bid';
            entity_id_val := NEW.id::TEXT;
            actor_id := NEW.user_id;
            IF TG_OP = 'INSERT' THEN
                action_name := 'bid_placed';
            ELSIF TG_OP = 'DELETE' THEN
                action_name := 'bid_removed';
                entity_id_val := OLD.id::TEXT;
                actor_id := NULL; -- الحذف إداري
            END IF;
        WHEN 'auctions' THEN
            entity_type_name := 'auction';
            entity_id_val := NEW.id::TEXT;
            IF TG_OP = 'UPDATE' AND OLD.status != NEW.status THEN
                action_name := 'auction_status_changed';
            END IF;
        WHEN 'payments' THEN
            entity_type_name := 'payment';
            entity_id_val := NEW.id::TEXT;
            IF TG_OP = 'UPDATE' AND OLD.status != NEW.status THEN
                action_name := 'payment_status_changed';
            END IF;
        ELSE
            RETURN COALESCE(NEW, OLD);
    END CASE;
    
    -- إدراج سجل التدقيق
    IF action_name IS NOT NULL THEN
        INSERT INTO audit_logs (
            actor_user_id, action, entity_type, entity_id, 
            meta, created_at
        ) VALUES (
            actor_id, action_name, entity_type_name, entity_id_val,
            jsonb_build_object(
                'old_status', CASE WHEN TG_OP = 'UPDATE' THEN OLD.status ELSE NULL END,
                'new_status', CASE WHEN TG_OP != 'DELETE' THEN NEW.status ELSE NULL END,
                'table', TG_TABLE_NAME,
                'operation', TG_OP
            ),
            NOW()
        );
    END IF;
    
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- إضافة triggers للتدقيق التلقائي
CREATE TRIGGER auto_audit_bids_trigger
    AFTER INSERT OR DELETE ON bids
    FOR EACH ROW EXECUTE FUNCTION auto_audit_sensitive_operations();

CREATE TRIGGER auto_audit_auctions_trigger
    AFTER UPDATE ON auctions
    FOR EACH ROW EXECUTE FUNCTION auto_audit_sensitive_operations();

CREATE TRIGGER auto_audit_payments_trigger
    AFTER UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION auto_audit_sensitive_operations();

-- إنشاء views مفيدة للاستعلامات المتكررة

-- view للمزادات مع السعر الحالي
CREATE VIEW auction_current_prices AS
SELECT 
    a.id,
    a.product_id,
    a.start_price,
    a.status,
    a.end_at,
    COALESCE(MAX(b.amount), a.start_price) as current_price,
    COUNT(b.id) as bid_count,
    CASE 
        WHEN a.reserve_price IS NOT NULL AND COALESCE(MAX(b.amount), a.start_price) < a.reserve_price 
        THEN false 
        ELSE true 
    END as reserve_met
FROM auctions a
LEFT JOIN bids b ON a.id = b.auction_id
GROUP BY a.id, a.product_id, a.start_price, a.status, a.end_at, a.reserve_price;

-- view للطلبات مع تفاصيل المستخدم
CREATE VIEW orders_with_user_details AS
SELECT 
    o.*,
    u.name as user_name,
    u.email as user_email,
    c.name_ar as city_name_ar,
    c.name_en as city_name_en
FROM orders o
JOIN users u ON o.user_id = u.id
LEFT JOIN cities c ON u.city_id = c.id;

-- view للمنتجات مع معلومات إضافية
CREATE VIEW products_with_details AS
SELECT 
    p.*,
    CASE 
        WHEN p.type = 'pigeon' THEN 
            jsonb_build_object(
                'ring_number', pg.ring_number,
                'sex', pg.sex,
                'birth_date', pg.birth_date,
                'lineage', pg.lineage
            )
        WHEN p.type = 'supply' THEN 
            jsonb_build_object(
                'sku', s.sku,
                'stock_qty', s.stock_qty,
                'low_stock_threshold', s.low_stock_threshold
            )
        ELSE NULL
    END as details,
    (
        SELECT COUNT(*) 
        FROM media m 
        WHERE m.product_id = p.id AND m.archived_at IS NULL
    ) as media_count
FROM products p
LEFT JOIN pigeons pg ON p.id = pg.product_id AND p.type = 'pigeon'
LEFT JOIN supplies s ON p.id = s.product_id AND p.type = 'supply';

-- view للإحصائيات اليومية
CREATE VIEW daily_stats AS
SELECT 
    DATE(created_at) as date,
    COUNT(*) FILTER (WHERE status = 'paid') as paid_orders,
    COUNT(*) FILTER (WHERE status = 'pending_payment') as pending_orders,
    SUM(grand_total) FILTER (WHERE status = 'paid') as daily_revenue,
    COUNT(DISTINCT user_id) as unique_customers
FROM orders 
WHERE created_at >= CURRENT_DATE - INTERVAL '30 days'
GROUP BY DATE(created_at)
ORDER BY date DESC;

-- ملاحظة: لا يمكن إنشاء فهارس على views غير مادية؛ تزال هذه الفهارس
