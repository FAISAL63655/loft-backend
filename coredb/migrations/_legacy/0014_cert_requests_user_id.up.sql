-- 0014_cert_requests_user_id.up.sql
-- Add user_id to certificate_requests to link requests to authenticated users

ALTER TABLE certificate_requests
ADD COLUMN IF NOT EXISTS user_id BIGINT;

-- Optional FK constraint (kept nullable for existing rows)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_certificate_requests_user'
          AND table_name = 'certificate_requests'
    ) THEN
        ALTER TABLE certificate_requests
        ADD CONSTRAINT fk_certificate_requests_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
    END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_certificate_requests_user_id ON certificate_requests(user_id);
