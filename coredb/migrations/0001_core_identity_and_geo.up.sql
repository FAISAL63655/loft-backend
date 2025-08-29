-- 0001_core_identity_and_geo.up.sql
-- الهجرة الأولى: الهوية والجغرافيا
-- منصة لوفت الدغيري للحمام الزاجل

-- إنشاء أنواع البيانات المخصصة
CREATE TYPE user_role AS ENUM ('registered', 'verified', 'admin');
CREATE TYPE user_state AS ENUM ('active', 'inactive');
CREATE TYPE verification_status AS ENUM ('pending', 'approved', 'rejected');

-- جدول المدن
CREATE TABLE cities (
    id BIGSERIAL PRIMARY KEY,
    name_ar TEXT NOT NULL,
    name_en TEXT NOT NULL,
    shipping_fee_net NUMERIC(12,2) NOT NULL DEFAULT 0.00,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهرس للبحث في المدن المفعّلة
CREATE INDEX idx_cities_enabled ON cities(enabled) WHERE enabled = true;

-- جدول المستخدمين
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهارس للمستخدمين
CREATE UNIQUE INDEX idx_users_email_lower ON users(LOWER(email));
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_state ON users(state);
CREATE INDEX idx_users_city_id ON users(city_id);
CREATE INDEX idx_users_email_verified ON users(email_verified_at) WHERE email_verified_at IS NOT NULL;

-- جدول طلبات التوثيق
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

-- فهارس لطلبات التوثيق
CREATE INDEX idx_verification_requests_user_id ON verification_requests(user_id);
CREATE INDEX idx_verification_requests_status ON verification_requests(status);
CREATE INDEX idx_verification_requests_reviewed_by ON verification_requests(reviewed_by);

-- جدول العناوين
CREATE TABLE addresses (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    city_id BIGINT NOT NULL REFERENCES cities(id),
    label TEXT NOT NULL,
    line1 TEXT NOT NULL,
    line2 TEXT,
    is_default BOOLEAN NOT NULL DEFAULT false,
    archived_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهارس للعناوين
CREATE INDEX idx_addresses_user_id ON addresses(user_id);
CREATE INDEX idx_addresses_city_id ON addresses(city_id);
CREATE INDEX idx_addresses_user_active ON addresses(user_id) WHERE archived_at IS NULL;

-- قيد فريد جزئي: عنوان افتراضي واحد فعّال لكل مستخدم
CREATE UNIQUE INDEX uq_default_address_per_user 
ON addresses(user_id) 
WHERE is_default = true AND archived_at IS NULL;

-- دالة تحديث التوقيت التلقائي
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- دالة تطبيع البريد الإلكتروني إلى حروف صغيرة
CREATE OR REPLACE FUNCTION normalize_email_lower()
RETURNS TRIGGER AS $$
BEGIN
    NEW.email := lower(NEW.email);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة triggers لتحديث updated_at تلقائياً
CREATE TRIGGER update_cities_updated_at 
    BEFORE UPDATE ON cities 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- إضافة trigger لتطبيع البريد الإلكتروني
CREATE TRIGGER normalize_email_lower_trg
    BEFORE INSERT OR UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION normalize_email_lower();

-- إضافة قيد التحقق للبريد الإلكتروني (يجب أن يكون بحروف صغيرة)
ALTER TABLE users
    ADD CONSTRAINT chk_email_lower CHECK (email = lower(email));

-- إزالة القيد المكرر على البريد الإلكتروني الخام (نبقي فقط فهرس LOWER(email))
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;

CREATE TRIGGER update_verification_requests_updated_at 
    BEFORE UPDATE ON verification_requests 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_addresses_updated_at 
    BEFORE UPDATE ON addresses 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- دالة لضبط العنوان الافتراضي (trigger)
CREATE OR REPLACE FUNCTION manage_default_address()
RETURNS TRIGGER AS $$
BEGIN
    -- إذا تم أرشفة العنوان وكان افتراضياً، أطفئ الافتراضية
    IF NEW.archived_at IS NOT NULL AND NEW.is_default = true THEN
        NEW.is_default := false;
    END IF;

    -- إذا تم تعيين عنوان كافتراضي
    IF NEW.is_default = true AND NEW.archived_at IS NULL THEN
        -- إلغاء الافتراضية من جميع العناوين الأخرى للمستخدم
        UPDATE addresses
        SET is_default = false
        WHERE user_id = NEW.user_id
        AND id <> NEW.id
        AND archived_at IS NULL;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger لإدارة العنوان الافتراضي
CREATE TRIGGER manage_default_address_trigger
    BEFORE INSERT OR UPDATE ON addresses
    FOR EACH ROW EXECUTE FUNCTION manage_default_address();

-- إدراج بيانات المدن الأولية (المدن الرئيسية في السعودية)
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

-- إنشاء مستخدم إداري أولي (كلمة المرور: admin123 - يجب تغييرها)
-- hash للكلمة admin123 باستخدام Argon2id
INSERT INTO users (name, email, password_hash, role, state, email_verified_at, city_id) VALUES
('مدير النظام', 'admin@loft-dughairi.com', '$argon2id$v=19$m=65536,t=3,p=2$placeholder_salt$placeholder_hash', 'admin', 'active', NOW(), 1);

-- تعليق: كلمة المرور المؤقتة هي admin123 ويجب تغييرها فور أول تسجيل دخول