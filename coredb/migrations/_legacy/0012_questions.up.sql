-- 0012_questions.up.sql
-- Q&A for products and auctions

-- Enum for question status
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'question_status') THEN
        CREATE TYPE question_status AS ENUM ('pending','approved','rejected');
    END IF;
END $$;

-- Product questions
CREATE TABLE IF NOT EXISTS product_questions (
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

CREATE INDEX IF NOT EXISTS idx_product_questions_product_id ON product_questions(product_id);
CREATE INDEX IF NOT EXISTS idx_product_questions_status ON product_questions(status);

-- Auction questions
CREATE TABLE IF NOT EXISTS auction_questions (
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

CREATE INDEX IF NOT EXISTS idx_auction_questions_auction_id ON auction_questions(auction_id);
CREATE INDEX IF NOT EXISTS idx_auction_questions_status ON auction_questions(status);

-- trigger to auto-update updated_at (uses function defined in 0010_triggers_rules_enforcement.up.sql)
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_product_questions_updated_at'
    ) THEN
        CREATE TRIGGER update_product_questions_updated_at
            BEFORE UPDATE ON product_questions
            FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_auction_questions_updated_at'
    ) THEN
        CREATE TRIGGER update_auction_questions_updated_at
            BEFORE UPDATE ON auction_questions
            FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;
