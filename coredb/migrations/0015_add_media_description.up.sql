-- Add description column to media table
ALTER TABLE media ADD COLUMN description TEXT;

-- Add comment to the column
COMMENT ON COLUMN media.description IS 'وصف الصورة أو الملف';

