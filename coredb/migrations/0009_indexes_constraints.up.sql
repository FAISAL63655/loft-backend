-- 0009_indexes_constraints.up.sql
-- الفهارس والقيود الإضافية للأداء والأمان
-- منصة لوفت الدغيري للحمام الزاجل

-- فهارس إضافية للأداء المحسّن

-- فهرس مركب للبحث في المنتجات المتاحة
CREATE INDEX idx_products_search ON products(type, status, created_at DESC) 
WHERE status NOT IN ('archived', 'sold');

-- فهرس للبحث النصي في المنتجات (اختياري)
-- ملاحظة: استخدام 'simple' لتجنّب فشل الإنشاء إذا لم يتوفر config 'arabic'
CREATE INDEX idx_products_title_search ON products USING gin(to_tsvector('simple', title));

-- فهرس لأسعار المنتجات للفلترة
CREATE INDEX idx_products_price_range ON products(price_net) 
WHERE status IN ('available', 'in_auction');

-- فهارس إضافية للمزادات
-- فهرس للمزادات التي تنتهي قريباً
-- ملاحظة: لا يجوز استخدام NOW() في predicate للفهرس الجزئي
-- سنستخدم فهرس مركب على (status, end_at) بدون شرط، والاستعلام يضيف شرط الوقت
CREATE INDEX idx_auctions_ending_soon ON auctions(status, end_at);

-- فهرس للمزادات النشطة مع السعر الحالي
CREATE INDEX idx_auctions_with_current_price ON auctions(id, status, end_at) 
WHERE status IN ('live', 'scheduled');

-- فهارس إضافية للمزايدات
-- فهرس لآخر مزايدة لكل مستخدم
CREATE INDEX idx_bids_user_latest ON bids(user_id, created_at DESC);

-- فهرس للمزايدات على مزاد معين مرتبة بالمبلغ
CREATE INDEX idx_bids_auction_amount ON bids(auction_id, amount DESC, created_at DESC);

-- فهارس إضافية للطلبات
-- فهرس للطلبات المعلقة للمتابعة الإدارية
CREATE INDEX idx_orders_pending_admin ON orders(status, created_at) 
WHERE status IN ('pending_payment', 'awaiting_admin_refund');

-- فهرس للطلبات حسب المصدر والحالة
CREATE INDEX idx_orders_source_status ON orders(source, status, created_at DESC);

-- فهارس إضافية للفواتير
-- فهرس للفواتير المعلقة
CREATE INDEX idx_invoices_pending ON invoices(status, created_at DESC) 
WHERE status IN ('unpaid', 'payment_in_progress');

-- فهرس للبحث برقم الفاتورة
CREATE INDEX idx_invoices_number_year ON invoices(SUBSTRING(number FROM 5 FOR 4), number);

-- فهارس إضافية للمدفوعات
-- فهرس للمدفوعات النشطة
CREATE INDEX idx_payments_active ON payments(status, created_at DESC) 
WHERE status IN ('initiated', 'pending');

-- فهرس لمراجع البوابة للتحقق من التكرار
CREATE INDEX idx_payments_gateway_ref_invoice ON payments(gateway_ref, invoice_id);

-- فهارس إضافية للشحنات
-- فهرس للشحنات النشطة
CREATE INDEX idx_shipments_active ON shipments(status, created_at DESC) 
WHERE status IN ('pending', 'processing', 'shipped');

-- فهرس للشحنات حسب المدينة (عبر الطلب والمستخدم)
CREATE INDEX idx_shipments_by_city ON shipments(id) 
INCLUDE (order_id, status);

-- قيود إضافية للأمان وسلامة البيانات

-- منع تغيير نوع المنتج بعد الإنشاء (CHECK لا يكفي لأنه لا يرى OLD)
CREATE OR REPLACE FUNCTION prevent_product_type_change()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.type <> OLD.type THEN
        RAISE EXCEPTION 'Product type is immutable after creation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER prevent_product_type_change_trigger
BEFORE UPDATE ON products
FOR EACH ROW EXECUTE FUNCTION prevent_product_type_change();

-- قيد لمنع المزايدة على مزاد المستخدم نفسه (اختياري)
-- ALTER TABLE bids ADD CONSTRAINT chk_no_self_bidding 
-- CHECK (user_id != (SELECT user_id FROM products p JOIN auctions a ON p.id = a.product_id WHERE a.id = auction_id));

-- قيد للتأكد من أن تاريخ انتهاء المزاد في المستقبل
ALTER TABLE auctions ADD CONSTRAINT chk_auction_end_future 
CHECK (end_at > start_at);

-- قيد للتأكد من أن مبلغ المزايدة موجب
ALTER TABLE bids ADD CONSTRAINT chk_bid_amount_positive 
CHECK (amount > 0);

-- قيد للتأكد من أن إجماليات الطلب صحيحة
ALTER TABLE orders ADD CONSTRAINT chk_order_totals_non_negative 
CHECK (subtotal_gross >= 0 AND vat_amount >= 0 AND shipping_fee_gross >= 0 AND grand_total >= 0);

-- قيد للتأكد من أن إجمالي الطلب = المجموع الفرعي + الشحن
ALTER TABLE orders ADD CONSTRAINT chk_order_total_calculation 
CHECK (grand_total = subtotal_gross + shipping_fee_gross);

-- قيد للتأكد من صحة مبالغ المدفوعات
ALTER TABLE payments ADD CONSTRAINT chk_payment_amounts_non_negative 
CHECK (amount_authorized >= 0 AND amount_captured >= 0 AND amount_refunded >= 0);

-- قيد للتأكد من أن المبلغ المسترد لا يتجاوز المقبوض
ALTER TABLE payments ADD CONSTRAINT chk_refund_not_exceed_captured 
CHECK (amount_refunded <= amount_captured);

-- قيد للتأكد من صحة علامة الاسترداد الجزئي
ALTER TABLE payments ADD CONSTRAINT chk_partial_refund_flag 
CHECK (
    (refund_partial = true AND amount_refunded > 0 AND amount_refunded < amount_captured) OR
    (refund_partial = false AND (amount_refunded = 0 OR amount_refunded = amount_captured))
);

-- فهارس للأداء في العمليات الحرجة
-- فهرس للبحث السريع في سجلات التدقيق للعمليات الحساسة
CREATE INDEX idx_audit_logs_sensitive_actions ON audit_logs(action, created_at DESC) 
WHERE action IN ('bid_placed', 'payment_initiated', 'payment_completed', 'auction_cancelled');

-- فهرس للمزايدات الأخيرة لكل مستخدم (لمنع الإسراف)
-- إزالة استخدام NOW() من predicate للفهرس
CREATE INDEX idx_bids_user_recent ON bids(user_id, created_at DESC);

-- فهرس للحجوزات النشطة للمستخدم
CREATE INDEX idx_stock_reservations_user_active ON stock_reservations(user_id, expires_at);

-- فهرس للإشعارات غير المقروءة
CREATE INDEX idx_notifications_unread ON notifications(user_id, created_at DESC) 
WHERE channel = 'internal' AND status = 'sent';

-- تحسين أداء البحث في الوسائط
CREATE INDEX idx_media_product_kind ON media(product_id, kind, created_at DESC) 
WHERE archived_at IS NULL;
