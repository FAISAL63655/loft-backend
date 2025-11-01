-- 0018_add_address_to_orders.down.sql
-- Rollback: Remove address_id from orders table

DROP INDEX IF EXISTS idx_orders_address_id;

ALTER TABLE orders 
DROP COLUMN IF EXISTS address_id;
