-- 0007_notifications_audit.up.sql
-- Baseline (7/10): notifications and audit logs

CREATE TYPE notification_channel AS ENUM ('internal','email');
CREATE TYPE notification_status AS ENUM ('queued','sending','sent','failed','archived');

CREATE TABLE notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel notification_channel NOT NULL,
    template_id TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    status notification_status NOT NULL DEFAULT 'queued',
    sent_at TIMESTAMPTZ NULL,
    failed_reason TEXT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    next_retry_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_notifications_user_id ON notifications(user_id);
CREATE INDEX idx_notifications_channel ON notifications(channel);
CREATE INDEX idx_notifications_status ON notifications(status);
CREATE INDEX idx_notifications_template_id ON notifications(template_id);
CREATE INDEX idx_notifications_created_at ON notifications(created_at DESC);
CREATE INDEX idx_notifications_retry ON notifications(next_retry_at) WHERE next_retry_at IS NOT NULL;
CREATE INDEX idx_notifications_pending ON notifications(channel, status, next_retry_at);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    reason TEXT,
    meta JSONB NOT NULL DEFAULT '{}',
    ip_address INET,
    user_agent TEXT,
    correlation_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_logs_actor_user_id ON audit_logs(actor_user_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_logs_correlation_id ON audit_logs(correlation_id) WHERE correlation_id IS NOT NULL;
CREATE INDEX idx_audit_logs_date_range ON audit_logs(created_at DESC, action, entity_type);

CREATE TRIGGER update_notifications_updated_at BEFORE UPDATE ON notifications FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION create_audit_log(p_actor_user_id BIGINT, p_action TEXT, p_entity_type TEXT, p_entity_id TEXT, p_reason TEXT DEFAULT NULL, p_meta JSONB DEFAULT '{}', p_correlation_id TEXT DEFAULT NULL) RETURNS BIGINT AS $$
DECLARE log_id BIGINT; BEGIN
    INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, reason, meta, correlation_id)
    VALUES (p_actor_user_id, p_action, p_entity_type, p_entity_id, p_reason, p_meta, p_correlation_id)
    RETURNING id INTO log_id;
    RETURN log_id;
END; $$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION cleanup_old_notifications() RETURNS INTEGER AS $$
DECLARE retention_days INTEGER; deleted_count INTEGER; BEGIN
    SELECT CAST(value AS INTEGER) INTO retention_days FROM system_settings WHERE key='notifications.email.retention_days';
    DELETE FROM notifications WHERE created_at < NOW() - (retention_days || ' days')::INTERVAL AND status IN ('sent','failed','archived');
    GET DIAGNOSTICS deleted_count = ROW_COUNT; RETURN deleted_count;
END; $$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION schedule_notification_retry() RETURNS TRIGGER AS $$
DECLARE backoff_minutes INTEGER; BEGIN
    IF NEW.status='failed' AND NEW.retry_count < NEW.max_retries THEN
        backoff_minutes := POWER(2, NEW.retry_count) * 5;
        NEW.next_retry_at := NOW() + (backoff_minutes || ' minutes')::INTERVAL;
    ELSE
        NEW.next_retry_at := NULL;
    END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER schedule_notification_retry_trigger BEFORE UPDATE ON notifications FOR EACH ROW EXECUTE FUNCTION schedule_notification_retry();
