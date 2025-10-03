-- 0004_auctions_bids.up.sql
-- Baseline (4/10): auctions, bids, extensions, validation & anti-sniping

CREATE TYPE auction_status AS ENUM ('draft','scheduled','live','ended','cancelled','winner_unpaid');

CREATE TABLE auctions (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES pigeons(product_id) ON DELETE CASCADE,
    start_price NUMERIC(12,2) NOT NULL CHECK (start_price >= 0),
    bid_step INTEGER NOT NULL CHECK (bid_step >= 1),
    reserve_price NUMERIC(12,2) NULL CHECK (reserve_price IS NULL OR reserve_price >= start_price),
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL CHECK (end_at > start_at),
    anti_sniping_minutes INTEGER NOT NULL DEFAULT 10 CHECK (anti_sniping_minutes >= 0 AND anti_sniping_minutes <= 60),
    status auction_status NOT NULL DEFAULT 'draft',
    extensions_count INTEGER NOT NULL DEFAULT 0,
    max_extensions_override INTEGER NULL CHECK (max_extensions_override IS NULL OR max_extensions_override >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_auctions_product_id ON auctions(product_id);
CREATE INDEX idx_auctions_status ON auctions(status);
CREATE INDEX idx_auctions_end_at ON auctions(end_at);
CREATE INDEX idx_auctions_status_end_at ON auctions(status, end_at);
CREATE UNIQUE INDEX uq_auction_active_product ON auctions(product_id) WHERE status IN ('scheduled','live');

CREATE TABLE bids (
    id BIGSERIAL PRIMARY KEY,
    auction_id BIGINT NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    bidder_name_snapshot TEXT NOT NULL,
    bidder_city_id_snapshot BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_bids_auction_created ON bids(auction_id, created_at DESC);
CREATE INDEX idx_bids_user_id ON bids(user_id);

CREATE TABLE auction_extensions (
    id BIGSERIAL PRIMARY KEY,
    auction_id BIGINT NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
    extended_by_bid_id BIGINT NOT NULL REFERENCES bids(id) ON DELETE CASCADE,
    old_end_at TIMESTAMPTZ NOT NULL,
    new_end_at TIMESTAMPTZ NOT NULL CHECK (new_end_at > old_end_at),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_auction_extensions_auction_id ON auction_extensions(auction_id);

CREATE TRIGGER update_auctions_updated_at BEFORE UPDATE ON auctions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION validate_bid_step() RETURNS TRIGGER AS $$
DECLARE
    auction_record RECORD;
    current_highest NUMERIC(12,2);
    expected_min NUMERIC(12,2);
    bid_diff NUMERIC(12,2);
BEGIN
    SELECT * INTO auction_record FROM auctions WHERE id = NEW.auction_id FOR UPDATE;
    IF auction_record.status != 'live' THEN RAISE EXCEPTION 'Cannot bid on auction with status: %', auction_record.status; END IF;
    IF auction_record.end_at <= NOW() THEN RAISE EXCEPTION 'Auction has ended'; END IF;
    SELECT COALESCE(MAX(amount), auction_record.start_price) INTO current_highest FROM bids WHERE auction_id = NEW.auction_id;
    expected_min := current_highest + auction_record.bid_step;
    IF NEW.amount < expected_min THEN RAISE EXCEPTION 'Bid amount % is less than required minimum %', NEW.amount, expected_min; END IF;
    bid_diff := NEW.amount - current_highest;
    IF mod(bid_diff, auction_record.bid_step) != 0 THEN RAISE EXCEPTION 'Bid amount must be in multiples of bid step %', auction_record.bid_step; END IF;
    SELECT name, city_id INTO NEW.bidder_name_snapshot, NEW.bidder_city_id_snapshot FROM users WHERE id = NEW.user_id;
    PERFORM 1 FROM users WHERE id = NEW.user_id AND role IN ('verified','admin') AND email_verified_at IS NOT NULL;
    IF NOT FOUND THEN RAISE EXCEPTION 'Bidding requires a verified account'; END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER validate_bid_step_trigger BEFORE INSERT ON bids FOR EACH ROW EXECUTE FUNCTION validate_bid_step();

CREATE OR REPLACE FUNCTION handle_anti_sniping() RETURNS TRIGGER AS $$
DECLARE
    auction_record RECORD;
    max_extensions INTEGER;
    time_remaining INTERVAL;
    new_end_time TIMESTAMPTZ;
    update_count INTEGER;
BEGIN
    SELECT * INTO auction_record FROM auctions WHERE id = NEW.auction_id;
    time_remaining := auction_record.end_at - NOW();
    IF time_remaining <= (auction_record.anti_sniping_minutes || ' minutes')::INTERVAL THEN
        SELECT COALESCE(auction_record.max_extensions_override, CAST(value AS INTEGER)) INTO max_extensions
        FROM system_settings WHERE key = 'auctions.max_extensions';
        IF max_extensions = 0 OR auction_record.extensions_count < max_extensions THEN
            new_end_time := auction_record.end_at + (auction_record.anti_sniping_minutes || ' minutes')::INTERVAL;
            UPDATE auctions SET end_at = new_end_time, extensions_count = extensions_count + 1 WHERE id = NEW.auction_id AND end_at = auction_record.end_at;
            GET DIAGNOSTICS update_count = ROW_COUNT;
            IF update_count > 0 THEN
                INSERT INTO auction_extensions (auction_id, extended_by_bid_id, old_end_at, new_end_at) VALUES (NEW.auction_id, NEW.id, auction_record.end_at, new_end_time);
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER handle_anti_sniping_trigger AFTER INSERT ON bids FOR EACH ROW EXECUTE FUNCTION handle_anti_sniping();
