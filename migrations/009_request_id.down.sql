-- 009_request_id.down.sql
ALTER TABLE orders
DROP COLUMN IF EXISTS request_id;
