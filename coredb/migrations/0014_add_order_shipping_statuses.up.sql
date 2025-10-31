-- 0014_add_order_shipping_statuses.up.sql
-- Add shipping-related statuses to order_status enum

-- Add new values to order_status enum
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'processing';
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'shipped';
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'delivered';

