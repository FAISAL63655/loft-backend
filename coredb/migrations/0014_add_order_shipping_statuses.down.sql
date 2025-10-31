-- 0014_add_order_shipping_statuses.down.sql
-- Rollback: Remove shipping-related statuses from order_status enum

-- Note: PostgreSQL doesn't support removing enum values directly
-- This would require recreating the enum type, which is complex
-- For now, this is a placeholder for documentation purposes
-- In production, you would need to:
-- 1. Create a new enum without the values
-- 2. Alter the column to use the new enum
-- 3. Drop the old enum
-- 4. Rename the new enum

-- This migration is intentionally left empty as removing enum values
-- requires more complex operations that could affect existing data

