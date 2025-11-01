-- Add admin_reason column to verification_requests table
ALTER TABLE verification_requests 
ADD COLUMN IF NOT EXISTS admin_reason TEXT;

COMMENT ON COLUMN verification_requests.admin_reason IS 'سبب الموافقة أو الرفض من قبل المدير';
