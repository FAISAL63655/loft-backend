-- 0018_add_address_to_orders.up.sql
-- Add address_id to orders table to track shipping address

ALTER TABLE orders 
ADD COLUMN address_id BIGINT REFERENCES addresses(id);

CREATE INDEX idx_orders_address_id ON orders(address_id);

COMMENT ON COLUMN orders.address_id IS 'عنوان الشحن المستخدم في الطلب';
