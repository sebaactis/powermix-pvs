-- 007_gs_v2_fields.down.sql
DROP INDEX IF EXISTS idx_orders_notify_pending;
DROP INDEX IF EXISTS idx_orders_gs_order_no;

ALTER TABLE orders
DROP COLUMN IF EXISTS gs_notified_at,
DROP COLUMN IF EXISTS notify_url,
DROP COLUMN IF EXISTS gs_order_no;
