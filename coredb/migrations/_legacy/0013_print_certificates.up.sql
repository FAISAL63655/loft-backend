-- 0013_print_certificates.up.sql
-- إضافة دعم طلبات طباعة الشهادات مع نموذج محمي برمز وصول

-- نوع حالة الطلب
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'certificate_request_status') THEN
        CREATE TYPE certificate_request_status AS ENUM ('pending', 'approved', 'rejected');
    END IF;
END$$;

-- جدول الطلبات
CREATE TABLE IF NOT EXISTS certificate_requests (
    id BIGSERIAL PRIMARY KEY,
    club_name TEXT NOT NULL,
    excel_gcs_path TEXT,
    status certificate_request_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- جدول تفاصيل السباقات للطلب
CREATE TABLE IF NOT EXISTS certificate_request_races (
    id BIGSERIAL PRIMARY KEY,
    request_id BIGINT NOT NULL REFERENCES certificate_requests(id) ON DELETE CASCADE,
    race_name TEXT NOT NULL,
    race_date DATE NOT NULL,
    quantity INT NOT NULL CHECK (quantity > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهارس مساعدة
CREATE INDEX IF NOT EXISTS idx_certificate_requests_created_at ON certificate_requests (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_certificate_requests_status ON certificate_requests (status);
CREATE INDEX IF NOT EXISTS idx_certificate_requests_club_name ON certificate_requests (club_name);
CREATE INDEX IF NOT EXISTS idx_certificate_request_races_req ON certificate_request_races (request_id);

-- Trigger لتحديث updated_at
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_certificate_requests_updated_at'
    ) THEN
        CREATE TRIGGER update_certificate_requests_updated_at
            BEFORE UPDATE ON certificate_requests
            FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END$$;

-- إعداد الإعداد الخاص بالرمز السري للوصول للنموذج (hash بقيمة Argon2id)
INSERT INTO system_settings (key, value, description)
VALUES ('cert.print_access_pin_hash', '', 'Argon2id hash for certificate print requests access PIN')
ON CONFLICT (key) DO NOTHING;
