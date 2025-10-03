-- 0008_rules_sync_audit.up.sql
-- Baseline (8/10): product/auction/invoice sync, auto audit, user deletion guard (no reservation refs)

CREATE OR REPLACE FUNCTION enforce_product_state_transitions() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.type='supply' AND NEW.status IN ('auction_hold','in_auction') THEN
        RAISE EXCEPTION 'Status % not valid for supply products', NEW.status;
    END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER enforce_product_state_transitions_trigger BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION enforce_product_state_transitions();

CREATE OR REPLACE FUNCTION sync_product_auction_status() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status!='live' AND NEW.status='live' THEN
        UPDATE products SET status='in_auction' WHERE id=NEW.product_id;
    END IF;
    IF OLD.status='live' AND NEW.status='ended' THEN
        IF EXISTS (SELECT 1 FROM bids WHERE auction_id=NEW.id AND amount >= COALESCE(NEW.reserve_price,0)) THEN
            UPDATE products SET status='auction_hold' WHERE id=NEW.product_id;
        ELSE
            UPDATE products SET status='available' WHERE id=NEW.product_id;
        END IF;
    END IF;
    IF NEW.status IN ('cancelled','winner_unpaid') THEN
        UPDATE products SET status='available' WHERE id=NEW.product_id;
    END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER sync_product_auction_status_trigger AFTER UPDATE ON auctions FOR EACH ROW EXECUTE FUNCTION sync_product_auction_status();

CREATE OR REPLACE FUNCTION sync_order_invoice_status() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status!='paid' AND NEW.status='paid' THEN
        UPDATE orders SET status='paid' WHERE id=NEW.order_id;
    END IF;
    IF NEW.status='failed' THEN
        UPDATE orders SET status='cancelled' WHERE id=NEW.order_id;
        UPDATE products SET status='available'
        WHERE id IN (SELECT oi.product_id FROM order_items oi WHERE oi.order_id=NEW.order_id)
          AND type='pigeon' AND status='auction_hold';
    END IF;
    IF NEW.status='refund_required' THEN
        UPDATE orders SET status='refund_required' WHERE id=NEW.order_id;
    END IF;
    IF NEW.status='refunded' THEN
        UPDATE orders SET status='refunded' WHERE id=NEW.order_id;
    END IF;
    RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER sync_order_invoice_status_trigger AFTER UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION sync_order_invoice_status();

CREATE OR REPLACE FUNCTION auto_audit_sensitive_operations() RETURNS TRIGGER AS $$
DECLARE action_name TEXT; entity_type_name TEXT; entity_id_val TEXT; actor_id BIGINT; old_status_val TEXT; new_status_val TEXT; BEGIN
    CASE TG_TABLE_NAME
        WHEN 'bids' THEN
            entity_type_name := 'bid';
            IF TG_OP='INSERT' THEN entity_id_val := NEW.id::TEXT; actor_id := NEW.user_id; action_name := 'bid_placed';
            ELSIF TG_OP='DELETE' THEN entity_id_val := OLD.id::TEXT; actor_id := NULL; action_name := 'bid_removed'; END IF;
        WHEN 'auctions' THEN
            entity_type_name := 'auction';
            IF TG_OP='UPDATE' THEN entity_id_val := NEW.id::TEXT; IF OLD.status != NEW.status THEN action_name := 'auction_status_changed'; old_status_val := OLD.status; new_status_val := NEW.status; END IF; END IF;
        WHEN 'payments' THEN
            entity_type_name := 'payment';
            IF TG_OP='UPDATE' THEN entity_id_val := NEW.id::TEXT; IF OLD.status != NEW.status THEN action_name := 'payment_status_changed'; old_status_val := OLD.status; new_status_val := NEW.status; END IF; END IF;
        ELSE RETURN COALESCE(NEW, OLD);
    END CASE;
    IF action_name IS NOT NULL THEN
        INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, meta, created_at)
        VALUES (actor_id, action_name, entity_type_name, entity_id_val,
            jsonb_build_object('old_status',old_status_val,'new_status',new_status_val,'table',TG_TABLE_NAME,'operation',TG_OP), NOW());
    END IF;
    RETURN COALESCE(NEW, OLD);
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER auto_audit_bids_trigger AFTER INSERT OR DELETE ON bids FOR EACH ROW EXECUTE FUNCTION auto_audit_sensitive_operations();
CREATE TRIGGER auto_audit_auctions_trigger AFTER UPDATE ON auctions FOR EACH ROW EXECUTE FUNCTION auto_audit_sensitive_operations();
CREATE TRIGGER auto_audit_payments_trigger AFTER UPDATE ON payments FOR EACH ROW EXECUTE FUNCTION auto_audit_sensitive_operations();

CREATE OR REPLACE FUNCTION prevent_user_deletion_with_active_data() RETURNS TRIGGER AS $$
DECLARE active_orders INTEGER; live_bids INTEGER; BEGIN
    SELECT COUNT(*) INTO active_orders FROM orders WHERE user_id=OLD.id AND status NOT IN ('paid','cancelled','refunded');
    SELECT COUNT(*) INTO live_bids FROM bids b JOIN auctions a ON b.auction_id=a.id WHERE b.user_id=OLD.id AND a.status='live';
    IF active_orders > 0 OR live_bids > 0 THEN RAISE EXCEPTION 'Cannot delete user with active orders or live bids'; END IF;
    RETURN OLD;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER prevent_user_deletion_with_active_data_trigger BEFORE DELETE ON users FOR EACH ROW EXECUTE FUNCTION prevent_user_deletion_with_active_data();
