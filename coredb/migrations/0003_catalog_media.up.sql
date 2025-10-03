-- 0003_catalog_media.up.sql
-- Baseline (3/10): catalog & media (no reservation model)

CREATE TYPE product_type AS ENUM ('pigeon', 'supply');
CREATE TYPE product_status AS ENUM ('available','in_auction','auction_hold','sold','out_of_stock','archived');
CREATE TYPE pigeon_sex AS ENUM ('male','female','unknown');
CREATE TYPE media_kind AS ENUM ('image','video','file');

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
CREATE INDEX idx_products_type ON products(type);
CREATE INDEX idx_products_status ON products(status);
CREATE INDEX idx_products_created_at ON products(created_at DESC);
CREATE INDEX idx_products_type_status ON products(type,status);
CREATE INDEX idx_products_available ON products(status, created_at DESC)
WHERE status IN ('available','in_auction');

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
CREATE INDEX idx_pigeons_sex ON pigeons(sex);
CREATE INDEX idx_pigeons_birth_date ON pigeons(birth_date);

CREATE TABLE supplies (
    product_id BIGINT PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
    sku TEXT NULL,
    stock_qty INTEGER NOT NULL DEFAULT 0,
    low_stock_threshold INTEGER NOT NULL DEFAULT 5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_supplies_stock_qty_non_negative CHECK (stock_qty >= 0),
    CONSTRAINT chk_supplies_threshold_positive CHECK (low_stock_threshold > 0)
);
CREATE INDEX idx_supplies_sku ON supplies(sku) WHERE sku IS NOT NULL;
CREATE INDEX idx_supplies_stock_qty ON supplies(stock_qty);
CREATE INDEX idx_supplies_low_stock ON supplies(stock_qty, low_stock_threshold)
WHERE stock_qty <= low_stock_threshold;

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
CREATE INDEX idx_media_product_id ON media(product_id);
CREATE INDEX idx_media_kind ON media(kind);
CREATE INDEX idx_media_product_unarchived ON media(product_id, created_at DESC)
WHERE archived_at IS NULL;

-- update timestamps triggers
CREATE TRIGGER update_products_updated_at BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_pigeons_updated_at BEFORE UPDATE ON pigeons FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_supplies_updated_at BEFORE UPDATE ON supplies FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_media_updated_at BEFORE UPDATE ON media FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- supply auto status
CREATE OR REPLACE FUNCTION update_supply_status() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.stock_qty = 0 AND OLD.stock_qty > 0 THEN
        UPDATE products SET status='out_of_stock' WHERE id=NEW.product_id AND type='supply';
    ELSIF NEW.stock_qty > 0 AND OLD.stock_qty = 0 THEN
        UPDATE products SET status='available' WHERE id=NEW.product_id AND type='supply';
    END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER update_supply_status_trigger AFTER UPDATE ON supplies FOR EACH ROW EXECUTE FUNCTION update_supply_status();

-- validate product status semantics
CREATE OR REPLACE FUNCTION validate_product_status() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.type='supply' AND NEW.status IN ('auction_hold','in_auction') THEN
        RAISE EXCEPTION 'Auction status % is not valid for supply products', NEW.status;
    END IF;
    IF NEW.type='pigeon' AND NEW.status='out_of_stock' THEN
        RAISE EXCEPTION 'Status out_of_stock is not valid for pigeon products';
    END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER validate_product_status_trigger BEFORE INSERT OR UPDATE ON products FOR EACH ROW EXECUTE FUNCTION validate_product_status();
