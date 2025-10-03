-- 0010_indexes_views_grants.up.sql
-- Baseline (10/10): additional indexes, views, and optional grants (no reservation artifacts)

-- Product indexes
CREATE INDEX idx_products_search ON products(type, status, created_at DESC) WHERE status NOT IN ('archived','sold');
CREATE INDEX idx_products_title_search ON products USING gin(to_tsvector('simple', title));
CREATE INDEX idx_products_price_range ON products(price_net) WHERE status IN ('available','in_auction');

-- Auction/bids indexes
CREATE INDEX idx_auctions_ending_soon ON auctions(status, end_at);
CREATE INDEX idx_auctions_with_current_price ON auctions(id, status, end_at) WHERE status IN ('live','scheduled');
CREATE INDEX idx_bids_user_latest ON bids(user_id, created_at DESC);
CREATE INDEX idx_bids_auction_amount ON bids(auction_id, amount DESC, created_at DESC);

-- Orders/invoices indexes
CREATE INDEX idx_orders_pending_admin ON orders(status, created_at) WHERE status IN ('pending_payment','awaiting_admin_refund');
CREATE INDEX idx_orders_source_status ON orders(source, status, created_at DESC);
CREATE INDEX idx_invoices_pending ON invoices(status, created_at DESC) WHERE status IN ('unpaid','payment_in_progress');
CREATE INDEX idx_invoices_number_year ON invoices(SUBSTRING(number FROM 5 FOR 4), number);

-- Payments indexes
CREATE INDEX idx_payments_active ON payments(status, created_at DESC) WHERE status IN ('initiated','pending');
CREATE INDEX idx_payments_gateway_ref_invoice ON payments(gateway_ref, invoice_id);

-- Shipments indexes
CREATE INDEX idx_shipments_active ON shipments(status, created_at DESC) WHERE status IN ('pending','processing','shipped');
CREATE INDEX idx_shipments_by_city ON shipments(id) INCLUDE (order_id, status);

-- Notifications/media indexes
CREATE INDEX idx_notifications_unread ON notifications(user_id, created_at DESC) WHERE channel='internal' AND status='sent';
CREATE INDEX idx_media_product_kind ON media(product_id, kind, created_at DESC) WHERE archived_at IS NULL;

-- Immutable product type
CREATE OR REPLACE FUNCTION prevent_product_type_change() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP='UPDATE' AND NEW.type <> OLD.type THEN RAISE EXCEPTION 'Product type is immutable after creation'; END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER prevent_product_type_change_trigger BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION prevent_product_type_change();

-- Views
CREATE VIEW auction_current_prices AS
SELECT a.id, a.product_id, a.start_price, a.status, a.end_at,
       COALESCE(MAX(b.amount), a.start_price) AS current_price,
       COUNT(b.id) AS bid_count,
       CASE WHEN a.reserve_price IS NOT NULL AND COALESCE(MAX(b.amount), a.start_price) < a.reserve_price THEN false ELSE true END AS reserve_met
FROM auctions a
LEFT JOIN bids b ON a.id = b.auction_id
GROUP BY a.id, a.product_id, a.start_price, a.status, a.end_at, a.reserve_price;

CREATE VIEW orders_with_user_details AS
SELECT o.*, u.name AS user_name, u.email AS user_email, c.name_ar AS city_name_ar, c.name_en AS city_name_en
FROM orders o
JOIN users u ON o.user_id = u.id
LEFT JOIN cities c ON u.city_id = c.id;

CREATE VIEW products_with_details AS
SELECT p.*, CASE WHEN p.type='pigeon' THEN jsonb_build_object('ring_number', pg.ring_number, 'sex', pg.sex, 'birth_date', pg.birth_date, 'lineage', pg.lineage)
                 WHEN p.type='supply' THEN jsonb_build_object('sku', s.sku, 'stock_qty', s.stock_qty, 'low_stock_threshold', s.low_stock_threshold)
                 ELSE NULL END AS details,
       (SELECT COUNT(*) FROM media m WHERE m.product_id = p.id AND m.archived_at IS NULL) AS media_count
FROM products p
LEFT JOIN pigeons pg ON p.id=pg.product_id AND p.type='pigeon'
LEFT JOIN supplies s ON p.id=s.product_id AND p.type='supply';

CREATE VIEW daily_stats AS
SELECT DATE(created_at) AS date,
       COUNT(*) FILTER (WHERE status='paid') AS paid_orders,
       COUNT(*) FILTER (WHERE status='pending_payment') AS pending_orders,
       SUM(grand_total) FILTER (WHERE status='paid') AS daily_revenue,
       COUNT(DISTINCT user_id) AS unique_customers
FROM orders
WHERE created_at >= CURRENT_DATE - INTERVAL '30 days'
GROUP BY DATE(created_at)
ORDER BY date DESC;

-- Optional grants for encore-read role
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='encore-read') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.notifications TO "encore-read";
        GRANT USAGE, SELECT ON SEQUENCE public.notifications_id_seq TO "encore-read";
        GRANT SELECT, INSERT ON TABLE public.audit_logs TO "encore-read";
        GRANT USAGE, SELECT ON SEQUENCE public.audit_logs_id_seq TO "encore-read";
        ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "encore-read";
        ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO "encore-read";
    END IF;
END $$;
