-- 0011_pigeon_pickup_only.up.sql
-- Waive shipping for pigeon-only orders (pickup from Loft Aldughairi)
-- Redefines calculate_order_totals() to set shipping_fee_gross=0 when order has no supplies

CREATE OR REPLACE FUNCTION calculate_order_totals() RETURNS TRIGGER AS $$
DECLARE
    subtotal             NUMERIC(12,2);
    vat_rate             DECIMAL(4,3);
    vat_enabled          BOOLEAN;
    shipping_fee_net     NUMERIC(12,2);
    shipping_fee_gross   NUMERIC(12,2);
    shipping_vat         NUMERIC(12,2);
    user_city_id         BIGINT;
    free_threshold       NUMERIC(12,2);
    has_supplies         BOOLEAN;
BEGIN
    -- Sum items gross
    SELECT COALESCE(SUM(line_total_gross),0)
      INTO subtotal
      FROM order_items
     WHERE order_id = NEW.id;

    -- VAT settings
    SELECT CAST(value AS BOOLEAN) INTO vat_enabled FROM system_settings WHERE key='vat.enabled';
    SELECT CAST(value AS DECIMAL) INTO vat_rate FROM system_settings WHERE key='vat.rate';
    IF NOT vat_enabled THEN vat_rate := 0; END IF;

    -- Check if the order contains any supply items (which require shipping)
    SELECT EXISTS(
        SELECT 1
          FROM order_items oi
          JOIN products p ON p.id = oi.product_id
         WHERE oi.order_id = NEW.id AND p.type = 'supply'
    ) INTO has_supplies;

    IF has_supplies THEN
        -- Compute shipping fee from user's city as before
        SELECT city_id INTO user_city_id FROM users WHERE id = NEW.user_id;
        SELECT c.shipping_fee_net INTO shipping_fee_net FROM cities c WHERE c.id = user_city_id;

        -- Free shipping threshold
        SELECT CAST(value AS NUMERIC) INTO free_threshold FROM system_settings WHERE key='shipping.free_shipping_threshold';

        shipping_fee_gross := ROUND(COALESCE(shipping_fee_net,0) * (1 + vat_rate), 2);
        shipping_vat := shipping_fee_gross - COALESCE(shipping_fee_net,0);
        IF subtotal >= COALESCE(free_threshold, 999999999.99) THEN
            shipping_fee_gross := 0;
            shipping_vat := 0;
        END IF;
    ELSE
        -- Pigeon-only order: no shipping fee
        shipping_fee_gross := 0;
        shipping_vat := 0;
    END IF;

    NEW.subtotal_gross := subtotal;
    NEW.vat_amount := (
        SELECT COALESCE(SUM(ROUND(oi.line_total_gross * vat_rate / (1 + vat_rate), 2)), 0)
          FROM order_items oi
         WHERE oi.order_id = NEW.id
    ) + COALESCE(shipping_vat,0);
    NEW.shipping_fee_gross := COALESCE(shipping_fee_gross,0);
    NEW.grand_total := COALESCE(subtotal,0) + COALESCE(shipping_fee_gross,0);
    RETURN NEW;
END; $$ LANGUAGE plpgsql;

-- The trigger binding remains the same (already created in 0005)
-- CREATE TRIGGER calculate_order_totals_trigger BEFORE INSERT OR UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION calculate_order_totals();
