-- 006_order_status_history.down.sql
-- Eliminación de trigger, función y tabla.
DROP TRIGGER IF EXISTS trg_order_status_history ON orders;
DROP FUNCTION IF EXISTS log_order_status_transition();
DROP TABLE IF EXISTS order_status_history;
