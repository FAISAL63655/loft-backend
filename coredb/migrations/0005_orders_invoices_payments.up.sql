-- 0005_orders_invoices_payments.up.sql
-- Baseline (5/10): orders, order_items, invoice_counters, invoices, payments, cart_items

CREATE TYPE order_source AS ENUM ('auction','direct');
CREATE TYPE order_status AS ENUM ('pending_payment','awaiting_admin_refund','paid','cancelled','refund_required','refunded');
CREATE TYPE invoice_status AS ENUM ('unpaid','payment_in_progress','paid','failed','refund_required','refunded','cancelled','void');
CREATE TYPE payment_status AS ENUM ('initiated','pending','paid','failed','refunded','cancelled');

CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    source order_source NOT NULL,
    status order_status NOT NULL DEFAULT 'pending_payment',
    subtotal_gross NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    vat_amount NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    shipping_fee_gross NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    grand_total NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_source ON orders(source);
CREATE INDEX idx_orders_created_at ON orders(created_at DESC);
CREATE INDEX idx_orders_user_status_created ON orders(user_id, status, created_at DESC);

CREATE TABLE order_items (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id),
    qty INTEGER NOT NULL CHECK (qty>0),
    unit_price_gross NUMERIC(12,2) NOT NULL CHECK (unit_price_gross>=0),
    line_total_gross NUMERIC(12,2) NOT NULL CHECK (line_total_gross>=0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_order_items_order_id ON order_items(order_id);
CREATE INDEX idx_order_items_product_id ON order_items(product_id);

CREATE TABLE invoice_counters (
    year INTEGER PRIMARY KEY,
    last_seq INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE invoices (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    number TEXT NOT NULL UNIQUE,
    status invoice_status NOT NULL DEFAULT 'unpaid',
    vat_rate_snapshot DECIMAL(4,3) NOT NULL,
    totals JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_invoices_order_id ON invoices(order_id);
CREATE INDEX idx_invoices_number ON invoices(number);
CREATE INDEX idx_invoices_status ON invoices(status);
CREATE INDEX idx_invoices_created_at ON invoices(created_at DESC);

CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    invoice_id BIGINT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    gateway TEXT NOT NULL,
    gateway_ref TEXT NOT NULL UNIQUE,
    status payment_status NOT NULL DEFAULT 'initiated',
    amount_authorized NUMERIC(12,2) DEFAULT 0.00,
    amount_captured NUMERIC(12,2) DEFAULT 0.00,
    amount_refunded NUMERIC(12,2) DEFAULT 0.00,
    refund_partial BOOLEAN NOT NULL DEFAULT false,
    currency CHAR(3) NOT NULL DEFAULT 'SAR',
    raw_response JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_payments_invoice_id ON payments(invoice_id);
CREATE INDEX idx_payments_gateway_ref ON payments(gateway_ref);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_created_at ON payments(created_at DESC);
CREATE UNIQUE INDEX uq_payments_invoice_live ON payments(invoice_id) WHERE status IN ('initiated','pending');

CREATE TRIGGER update_orders_updated_at BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_invoice_counters_updated_at BEFORE UPDATE ON invoice_counters FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_invoices_updated_at BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_payments_updated_at BEFORE UPDATE ON payments FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION next_invoice_number(target_year INTEGER) RETURNS TEXT AS $$
DECLARE next_seq INTEGER; invoice_number TEXT; BEGIN
    INSERT INTO invoice_counters (year, last_seq) VALUES (target_year, 1)
    ON CONFLICT (year) DO UPDATE SET last_seq = invoice_counters.last_seq + 1, updated_at = NOW()
    RETURNING last_seq INTO next_seq;
    invoice_number := 'INV-' || target_year || '-' || LPAD(next_seq::TEXT, 6, '0');
    RETURN invoice_number;
END; $$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION calculate_order_totals() RETURNS TRIGGER AS $$
DECLARE subtotal NUMERIC(12,2); vat_rate DECIMAL(4,3); vat_enabled BOOLEAN; shipping_fee_net NUMERIC(12,2); shipping_fee_gross NUMERIC(12,2); shipping_vat NUMERIC(12,2); user_city_id BIGINT; free_threshold NUMERIC(12,2);
BEGIN
    SELECT COALESCE(SUM(line_total_gross),0) INTO subtotal FROM order_items WHERE order_id = NEW.id;
    SELECT CAST(value AS BOOLEAN) INTO vat_enabled FROM system_settings WHERE key='vat.enabled';
    SELECT CAST(value AS DECIMAL) INTO vat_rate FROM system_settings WHERE key='vat.rate';
    IF NOT vat_enabled THEN vat_rate := 0; END IF;
    SELECT city_id INTO user_city_id FROM users WHERE id=NEW.user_id;
    SELECT c.shipping_fee_net INTO shipping_fee_net FROM cities c WHERE c.id=user_city_id;
    shipping_fee_gross := ROUND(shipping_fee_net * (1 + vat_rate), 2);
    shipping_vat := shipping_fee_gross - shipping_fee_net;
    SELECT CAST(value AS NUMERIC) INTO free_threshold FROM system_settings WHERE key='shipping.free_shipping_threshold';
    IF subtotal >= free_threshold THEN shipping_fee_gross := 0; shipping_vat := 0; END IF;
    NEW.subtotal_gross := subtotal;
    NEW.vat_amount := (SELECT COALESCE(SUM(ROUND(oi.line_total_gross * vat_rate / (1 + vat_rate), 2)), 0) FROM order_items oi WHERE oi.order_id = NEW.id) + shipping_vat;
    NEW.shipping_fee_gross := shipping_fee_gross;
    NEW.grand_total := subtotal + shipping_fee_gross;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER calculate_order_totals_trigger BEFORE INSERT OR UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION calculate_order_totals();

CREATE OR REPLACE FUNCTION enforce_pigeon_qty() RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM products p WHERE p.id = NEW.product_id AND p.type='pigeon') AND NEW.qty != 1 THEN
        RAISE EXCEPTION 'Pigeon items must have qty=1';
    END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER enforce_pigeon_qty_trigger BEFORE INSERT OR UPDATE ON order_items FOR EACH ROW EXECUTE FUNCTION enforce_pigeon_qty();

-- Cart items (no-reservation model)
CREATE TABLE cart_items (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    qty INTEGER NOT NULL DEFAULT 1 CHECK (qty > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, product_id)
);
CREATE INDEX idx_cart_items_user_id ON cart_items(user_id);
CREATE INDEX idx_cart_items_created_at ON cart_items(created_at DESC);
