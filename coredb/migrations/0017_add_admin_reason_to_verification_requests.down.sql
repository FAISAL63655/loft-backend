-- Remove admin_reason column from verification_requests table
ALTER TABLE verification_requests 
DROP COLUMN IF EXISTS admin_reason;
