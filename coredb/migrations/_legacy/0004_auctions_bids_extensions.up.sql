-- 0004_auctions_bids_extensions.up.sql
-- المزادات والمزايدات والتمديدات
-- منصة لوفت الدغيري للحمام الزاجل

-- إنشاء أنواع البيانات للمزادات
CREATE TYPE auction_status AS ENUM (
    'draft', 'scheduled', 'live', 'ended', 
    'cancelled', 'winner_unpaid'
);

-- جدول المزادات
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

-- فهارس للمزادات (مبسطة)
CREATE INDEX idx_auctions_product_id ON auctions(product_id);
CREATE INDEX idx_auctions_status ON auctions(status);
CREATE INDEX idx_auctions_end_at ON auctions(end_at);
CREATE INDEX idx_auctions_status_end_at ON auctions(status, end_at);

-- قيد فريد جزئي: منع مزادين نشطين لنفس المنتج
CREATE UNIQUE INDEX uq_auction_active_product 
ON auctions(product_id) 
WHERE status IN ('scheduled', 'live');

-- جدول المزايدات
CREATE TABLE bids (
    id BIGSERIAL PRIMARY KEY,
    auction_id BIGINT NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    bidder_name_snapshot TEXT NOT NULL,
    bidder_city_id_snapshot BIGINT NULL, -- يسمح NULL لأن city_id قد يكون NULL
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهارس للمزايدات (مبسطة حسب PRD)  
CREATE INDEX idx_bids_auction_created ON bids(auction_id, created_at DESC);
CREATE INDEX idx_bids_user_id ON bids(user_id);

-- جدول تمديدات المزادات (Anti-Sniping)
CREATE TABLE auction_extensions (
    id BIGSERIAL PRIMARY KEY,
    auction_id BIGINT NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
    extended_by_bid_id BIGINT NOT NULL REFERENCES bids(id) ON DELETE CASCADE,
    old_end_at TIMESTAMPTZ NOT NULL,
    new_end_at TIMESTAMPTZ NOT NULL CHECK (new_end_at > old_end_at),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- فهارس لتمديدات المزادات
CREATE INDEX idx_auction_extensions_auction_id ON auction_extensions(auction_id);

-- إضافة trigger لتحديث updated_at
CREATE TRIGGER update_auctions_updated_at 
    BEFORE UPDATE ON auctions 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- دالة للتحقق من صحة خطوة المزايدة (مع حماية race conditions)
CREATE OR REPLACE FUNCTION validate_bid_step()
RETURNS TRIGGER AS $$
DECLARE
    auction_record RECORD;
    current_highest NUMERIC(12,2);
    expected_min NUMERIC(12,2);
    bid_diff NUMERIC(12,2);
BEGIN
    -- قفل صف المزاد لمنع race conditions
    SELECT * INTO auction_record 
    FROM auctions 
    WHERE id = NEW.auction_id
    FOR UPDATE;
    
    -- التحقق من أن المزاد في حالة live
    IF auction_record.status != 'live' THEN
        RAISE EXCEPTION 'Cannot bid on auction with status: %', auction_record.status;
    END IF;
    
    -- التحقق من أن المزاد لم ينته
    IF auction_record.end_at <= NOW() THEN
        RAISE EXCEPTION 'Auction has ended';
    END IF;
    
    -- الحصول على أعلى مزايدة حالية
    SELECT COALESCE(MAX(amount), auction_record.start_price) INTO current_highest
    FROM bids 
    WHERE auction_id = NEW.auction_id;
    
    -- حساب الحد الأدنى المطلوب
    expected_min := current_highest + auction_record.bid_step;
    
    -- التحقق من أن المبلغ يتبع خطوة الزيادة
    IF NEW.amount < expected_min THEN
        RAISE EXCEPTION 'Bid amount % is less than required minimum %', NEW.amount, expected_min;
    END IF;
    
    -- التحقق من أن المبلغ مضاعف صحيح للخطوة (إصلاح نوع البيانات)
    bid_diff := NEW.amount - current_highest;
    IF mod(bid_diff, auction_record.bid_step) != 0 THEN
        RAISE EXCEPTION 'Bid amount must be in multiples of bid step %', auction_record.bid_step;
    END IF;
    
    -- نسخ بيانات المزايد وقت المزايدة (مع السماح بـ NULL للمدينة)
    SELECT name, city_id INTO NEW.bidder_name_snapshot, NEW.bidder_city_id_snapshot
    FROM users
    WHERE id = NEW.user_id;

    -- تحقق: المزايدة للمستخدمين الموثقين فقط (Verified/Admin) مع تفعيل البريد
    PERFORM 1 FROM users
    WHERE id = NEW.user_id
      AND role IN ('verified', 'admin')
      AND email_verified_at IS NOT NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'Bidding requires a verified account';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger للتحقق من المزايدات
CREATE TRIGGER validate_bid_step_trigger
    BEFORE INSERT ON bids
    FOR EACH ROW EXECUTE FUNCTION validate_bid_step();

-- دالة لإدارة Anti-Sniping (مع شرط تفاؤلي)
CREATE OR REPLACE FUNCTION handle_anti_sniping()
RETURNS TRIGGER AS $$
DECLARE
    auction_record RECORD;
    max_extensions INTEGER;
    time_remaining INTERVAL;
    new_end_time TIMESTAMPTZ;
    update_count INTEGER;
BEGIN
    -- الحصول على تفاصيل المزاد
    SELECT * INTO auction_record 
    FROM auctions 
    WHERE id = NEW.auction_id;
    
    -- حساب الوقت المتبقي
    time_remaining := auction_record.end_at - NOW();
    
    -- التحقق من أن المزايدة ضمن فترة Anti-Sniping
    IF time_remaining <= (auction_record.anti_sniping_minutes || ' minutes')::INTERVAL THEN
        -- حساب الحد الأقصى للتمديدات
        SELECT COALESCE(
            auction_record.max_extensions_override,
            CAST(value AS INTEGER)
        ) INTO max_extensions
        FROM system_settings 
        WHERE key = 'auctions.max_extensions';
        
        -- التحقق من عدم تجاوز الحد الأقصى للتمديدات (0 = غير محدود)
        IF max_extensions = 0 OR auction_record.extensions_count < max_extensions THEN
            -- حساب الوقت الجديد للانتهاء
            new_end_time := auction_record.end_at + (auction_record.anti_sniping_minutes || ' minutes')::INTERVAL;
            
            -- تحديث وقت انتهاء المزاد مع شرط تفاؤلي
            UPDATE auctions 
            SET end_at = new_end_time,
                extensions_count = extensions_count + 1
            WHERE id = NEW.auction_id 
            AND end_at = auction_record.end_at; -- شرط تفاؤلي لمنع التمديد المتأخر
            
            GET DIAGNOSTICS update_count = ROW_COUNT;
            
            -- إذا تم التحديث بنجاح، أضف سجل التمديد
            IF update_count > 0 THEN
                INSERT INTO auction_extensions (
                    auction_id, 
                    extended_by_bid_id, 
                    old_end_at, 
                    new_end_at
                ) VALUES (
                    NEW.auction_id,
                    NEW.id,
                    auction_record.end_at,
                    new_end_time
                );
            END IF;
        END IF;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- إضافة trigger لـ Anti-Sniping
CREATE TRIGGER handle_anti_sniping_trigger
    AFTER INSERT ON bids
    FOR EACH ROW EXECUTE FUNCTION handle_anti_sniping();