-- 0001_core_and_users.up.sql
-- Baseline (1/10): core identity, geo, users, addresses with full city seeds

-- Enums
CREATE TYPE user_role AS ENUM ('registered', 'verified', 'admin');
CREATE TYPE user_state AS ENUM ('active', 'inactive');
CREATE TYPE verification_status AS ENUM ('pending', 'approved', 'rejected');

-- Cities
CREATE TABLE cities (
    id BIGSERIAL PRIMARY KEY,
    name_ar TEXT NOT NULL,
    name_en TEXT NOT NULL,
    shipping_fee_net NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_cities_enabled ON cities(enabled) WHERE enabled = true;

-- Users
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    phone TEXT,
    password_hash TEXT NOT NULL,
    city_id BIGINT REFERENCES cities(id),
    role user_role NOT NULL DEFAULT 'registered',
    state user_state NOT NULL DEFAULT 'active',
    email_verified_at TIMESTAMPTZ NULL,
    last_login_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_users_email_lower ON users(LOWER(email));
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_state ON users(state);
CREATE INDEX idx_users_city_id ON users(city_id);
CREATE INDEX idx_users_email_verified ON users(email_verified_at) WHERE email_verified_at IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_phone_unique ON users(phone) WHERE phone IS NOT NULL;

-- Verification requests
CREATE TABLE verification_requests (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note TEXT,
    status verification_status NOT NULL DEFAULT 'pending',
    reviewed_by BIGINT REFERENCES users(id),
    reviewed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_verification_requests_user_id ON verification_requests(user_id);
CREATE INDEX idx_verification_requests_status ON verification_requests(status);
CREATE INDEX idx_verification_requests_reviewed_by ON verification_requests(reviewed_by);

-- Email verification codes
CREATE TABLE email_verification_codes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    code TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_email_verification_codes_user_id ON email_verification_codes(user_id);
CREATE INDEX idx_email_verification_codes_email ON email_verification_codes(email);
CREATE INDEX idx_email_verification_codes_code ON email_verification_codes(code);
CREATE INDEX idx_email_verification_codes_expires_at ON email_verification_codes(expires_at);

-- Phone verification sessions
CREATE TABLE phone_verification_sessions (
    id BIGSERIAL PRIMARY KEY,
    phone TEXT NOT NULL,
    code TEXT NOT NULL, 
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ NULL,
    verification_token TEXT NULL, 
    token_expires_at TIMESTAMPTZ NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_phone_verif_sessions_phone ON phone_verification_sessions(phone);
CREATE INDEX idx_phone_verif_sessions_code ON phone_verification_sessions(code);
CREATE INDEX idx_phone_verif_sessions_token ON phone_verification_sessions(verification_token);
CREATE INDEX idx_phone_verif_sessions_expires ON phone_verification_sessions(expires_at);


-- Addresses
CREATE TABLE addresses (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    city_id BIGINT NOT NULL REFERENCES cities(id),
    label TEXT NOT NULL,
    line1 TEXT NOT NULL,
    line2 TEXT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    archived_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_addresses_user_id ON addresses(user_id);
CREATE INDEX idx_addresses_city_id ON addresses(city_id);
CREATE INDEX idx_addresses_user_active ON addresses(user_id) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX uq_default_address_per_user ON addresses(user_id) WHERE is_default = true AND archived_at IS NULL;

-- Utility functions & triggers
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION normalize_email_lower()
RETURNS TRIGGER AS $$
BEGIN
    NEW.email := lower(NEW.email);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION manage_default_address()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.archived_at IS NOT NULL AND NEW.is_default = true THEN
        NEW.is_default := false;
    END IF;
    IF NEW.is_default = true AND NEW.archived_at IS NULL THEN
        UPDATE addresses
        SET is_default = false
        WHERE user_id = NEW.user_id
          AND id <> NEW.id
          AND archived_at IS NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_cities_updated_at 
    BEFORE UPDATE ON cities 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER normalize_email_lower_trg
    BEFORE INSERT OR UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION normalize_email_lower();

ALTER TABLE users ADD CONSTRAINT chk_email_lower CHECK (email = lower(email));
ALTER TABLE users ADD CONSTRAINT users_email_unique UNIQUE (email);

CREATE TRIGGER update_verification_requests_updated_at 
    BEFORE UPDATE ON verification_requests 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_phone_verif_sessions_updated_at 
    BEFORE UPDATE ON phone_verification_sessions 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_addresses_updated_at 
    BEFORE UPDATE ON addresses 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER manage_default_address_trigger
    BEFORE INSERT OR UPDATE ON addresses
    FOR EACH ROW EXECUTE FUNCTION manage_default_address();

-- Seed: full cities list
INSERT INTO cities (name_ar, name_en, shipping_fee_net, enabled) VALUES
('الرياض', 'Riyadh', 25.00, true),
('جدة', 'Jeddah', 30.00, true),
('مكة المكرمة', 'Makkah', 35.00, true),
('المدينة المنورة', 'Madinah', 35.00, true),
('الدمام', 'Dammam', 30.00, true),
('الخبر', 'Khobar', 30.00, true),
('الظهران', 'Dhahran', 30.00, true),
('الطائف', 'Taif', 35.00, true),
('بريدة', 'Buraydah', 40.00, true),
('تبوك', 'Tabuk', 45.00, true),
('خميس مشيط', 'Khamis Mushait', 40.00, true),
('الهفوف', 'Hofuf', 35.00, true),
('المبرز', 'Mubarraz', 35.00, true),
('حائل', 'Hail', 45.00, true),
('نجران', 'Najran', 50.00, true),
('الجبيل', 'Jubail', 35.00, true),
('ينبع', 'Yanbu', 40.00, true),
('الخرج', 'Kharj', 25.00, true),
('أبها', 'Abha', 45.00, true),
('عرعر', 'Arar', 50.00, true);

-- Seed: initial admin (password: "password")
INSERT INTO users (name, email, password_hash, role, state, email_verified_at, city_id)
VALUES ('مدير النظام', 'admin@loft-dughairi.com', '$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi', 'admin', 'active', NOW(), 1);
