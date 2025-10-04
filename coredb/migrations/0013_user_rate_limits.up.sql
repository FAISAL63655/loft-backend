-- Create user auction rate limits table for custom rate limiting
CREATE TABLE IF NOT EXISTS user_auction_rate_limits (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    bids_per_minute INT NOT NULL DEFAULT 60,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_user_auction_rate_limits_user_id ON user_auction_rate_limits(user_id);

COMMENT ON TABLE user_auction_rate_limits IS 'Custom rate limits for users (overrides system defaults)';
COMMENT ON COLUMN user_auction_rate_limits.bids_per_minute IS 'Maximum bids allowed per minute for this user';
