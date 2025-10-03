-- 0006_shipping.up.sql
-- Baseline (6/10): shipping companies and shipments

CREATE TYPE delivery_method AS ENUM ('courier','pickup');
CREATE TYPE shipment_status AS ENUM ('pending','processing','shipped','delivered','failed','returned');

CREATE TABLE shipping_companies (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_shipping_companies_enabled ON shipping_companies(enabled) WHERE enabled = true;

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
);
CREATE INDEX idx_shipments_order_id ON shipments(order_id);
CREATE INDEX idx_shipments_company_id ON shipments(company_id);
CREATE INDEX idx_shipments_status ON shipments(status);
CREATE INDEX idx_shipments_tracking_ref ON shipments(tracking_ref) WHERE tracking_ref IS NOT NULL;
CREATE INDEX idx_shipments_created_at ON shipments(created_at DESC);

CREATE TRIGGER update_shipping_companies_updated_at BEFORE UPDATE ON shipping_companies FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_shipments_updated_at BEFORE UPDATE ON shipments FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION add_shipment_event(shipment_id BIGINT, new_status shipment_status, note TEXT DEFAULT NULL) RETURNS VOID AS $$
DECLARE event_data JSONB; BEGIN
    event_data := jsonb_build_object('at', to_char(NOW() at time zone 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),'status', new_status::TEXT,'note', note);
    UPDATE shipments SET events = events || jsonb_build_array(event_data), status = new_status WHERE id = shipment_id;
END; $$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enforce_shipment_paid() RETURNS TRIGGER AS $$
DECLARE ord_status TEXT; BEGIN
    SELECT status::text INTO ord_status FROM orders WHERE id = NEW.order_id;
    IF ord_status IS NULL THEN RAISE EXCEPTION 'Order not found for shipment'; END IF;
    IF ord_status <> 'paid' THEN RAISE EXCEPTION 'SHP_ORDER_NOT_PAID'; END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER enforce_shipment_paid_ins BEFORE INSERT ON shipments FOR EACH ROW EXECUTE FUNCTION enforce_shipment_paid();
CREATE TRIGGER enforce_shipment_paid_upd BEFORE UPDATE ON shipments FOR EACH ROW EXECUTE FUNCTION enforce_shipment_paid();

INSERT INTO shipping_companies (name, enabled) VALUES
('شركة الشحن السريع', true),('البريد السعودي', true),('شركة النقل المتطور', true),('خدمات التوصيل المميز', false)
ON CONFLICT DO NOTHING;
