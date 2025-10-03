-- 0015_site_pages.up.sql
-- Create site_pages table to store static page contents (terms, privacy, cookies, refund)

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'page_content_format') THEN
        CREATE TYPE page_content_format AS ENUM ('html', 'markdown');
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS site_pages (
    slug TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    format page_content_format NOT NULL DEFAULT 'html',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed default rows if missing
INSERT INTO site_pages (slug, title, content, format)
VALUES
    ('terms',   'الشروط والأحكام',  '', 'html'),
    ('privacy', 'سياسة الخصوصية',   '', 'html'),
    ('cookies', 'سياسة الكوكيز',    '', 'html'),
    ('refund',  'سياسة الاسترجاع',  '', 'html')
ON CONFLICT (slug) DO NOTHING;

-- Trigger to keep updated_at fresh
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_site_pages_updated_at'
    ) THEN
        CREATE TRIGGER update_site_pages_updated_at
            BEFORE UPDATE ON site_pages
            FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END$$;
