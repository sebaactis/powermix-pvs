-- 008_rename_to_third_order_no.down.sql
-- Revierte third_order_no -> order_no

CREATE OR REPLACE FUNCTION log_order_status_transition()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO order_status_history (order_no, from_status, to_status, changed_at)
        VALUES (NEW.order_no, NULL, NEW.status, now());
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.status IS DISTINCT FROM NEW.status THEN
            INSERT INTO order_status_history (order_no, from_status, to_status, changed_at)
            VALUES (NEW.order_no, OLD.status, NEW.status, now());
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_order_status_history_third_order_no'
    ) THEN
        ALTER TABLE order_status_history
            RENAME CONSTRAINT fk_order_status_history_third_order_no
            TO fk_order_status_history_order_no;
    END IF;
END $$;

ALTER TABLE order_status_history RENAME COLUMN third_order_no TO order_no;
ALTER INDEX IF EXISTS idx_order_status_history_third_order_no RENAME TO idx_order_status_history_order_no;

ALTER TABLE refunds RENAME COLUMN third_order_no TO order_no;
ALTER INDEX IF EXISTS idx_refunds_third_order_no RENAME TO idx_refunds_order_no;

ALTER TABLE api_sync_log RENAME COLUMN third_order_no TO order_no;
ALTER INDEX IF EXISTS idx_sync_log_third_order_no RENAME TO idx_sync_log_order;

ALTER TABLE idempotency_keys RENAME COLUMN third_order_no TO order_no;
ALTER INDEX IF EXISTS idx_idempotency_keys_third_order_no RENAME TO idx_idempotency_keys_order;

ALTER TABLE orders RENAME COLUMN third_order_no TO order_no;
ALTER INDEX IF EXISTS idx_orders_third_order_no RENAME TO idx_orders_order_no;
