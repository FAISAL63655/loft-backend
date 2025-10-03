-- 0009_questions_pages_certificates.up.sql
-- Baseline (9/10): Q&A, site pages, certificate requests (+user_id)

CREATE TYPE question_status AS ENUM ('pending','approved','rejected');
CREATE TABLE product_questions (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    question TEXT NOT NULL CHECK (char_length(question) BETWEEN 3 AND 2000),
    answer TEXT NULL,
    answered_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    status question_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    answered_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_product_questions_product_id ON product_questions(product_id);
CREATE INDEX idx_product_questions_status ON product_questions(status);

CREATE TABLE auction_questions (
    id BIGSERIAL PRIMARY KEY,
    auction_id BIGINT NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    question TEXT NOT NULL CHECK (char_length(question) BETWEEN 3 AND 2000),
    answer TEXT NULL,
    answered_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    status question_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    answered_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_auction_questions_auction_id ON auction_questions(auction_id);
CREATE INDEX idx_auction_questions_status ON auction_questions(status);

CREATE TRIGGER update_product_questions_updated_at BEFORE UPDATE ON product_questions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_auction_questions_updated_at BEFORE UPDATE ON auction_questions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TYPE page_content_format AS ENUM ('html','markdown');
CREATE TABLE site_pages (
    slug TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    format page_content_format NOT NULL DEFAULT 'html',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO site_pages (slug,title,content,format) VALUES
('terms','الشروط والأحكام','', 'html'),
('privacy','سياسة الخصوصية','', 'html'),
('cookies','سياسة الكوكيز','', 'html'),
('refund','سياسة الاسترجاع','', 'html')
ON CONFLICT (slug) DO NOTHING;
CREATE TRIGGER update_site_pages_updated_at BEFORE UPDATE ON site_pages FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TYPE certificate_request_status AS ENUM ('pending','approved','rejected');
CREATE TABLE certificate_requests (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    club_name TEXT NOT NULL,
    excel_gcs_path TEXT,
    status certificate_request_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE certificate_request_races (
    id BIGSERIAL PRIMARY KEY,
    request_id BIGINT NOT NULL REFERENCES certificate_requests(id) ON DELETE CASCADE,
    race_name TEXT NOT NULL,
    race_date DATE NOT NULL,
    quantity INT NOT NULL CHECK (quantity > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_certificate_requests_created_at ON certificate_requests (created_at DESC);
CREATE INDEX idx_certificate_requests_status ON certificate_requests (status);
CREATE INDEX idx_certificate_requests_club_name ON certificate_requests (club_name);
CREATE INDEX idx_certificate_request_races_req ON certificate_request_races (request_id);
CREATE INDEX idx_certificate_requests_user_id ON certificate_requests(user_id);

INSERT INTO system_settings (key, value, description) VALUES ('cert.print_access_pin_hash','', 'Argon2id hash for certificate print requests access PIN') ON CONFLICT (key) DO NOTHING;
