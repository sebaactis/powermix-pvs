-- 008_rename_to_third_order_no.up.sql
-- Alinea el nombre del id nuestro con GS Open API v2 (thirdOrderNo).
-- order_no -> third_order_no en todas las tablas relacionadas.

-- orders (PK logica / unique)
ALTER TABLE orders RENAME COLUMN order_no TO third_order_no;
ALTER INDEX IF EXISTS idx_orders_order_no RENAME TO idx_orders_third_order_no;

-- idempotency_keys
ALTER TABLE idempotency_keys RENAME COLUMN order_no TO third_order_no;
ALTER INDEX IF EXISTS idx_idempotency_keys_order RENAME TO idx_idempotency_keys_third_order_no;

-- api_sync_log
ALTER TABLE api_sync_log RENAME COLUMN order_no TO third_order_no;
ALTER INDEX IF EXISTS idx_sync_log_order RENAME TO idx_sync_log_third_order_no;

-- refunds
ALTER TABLE refunds RENAME COLUMN order_no TO third_order_no;
ALTER INDEX IF EXISTS idx_refunds_order_no RENAME TO idx_refunds_third_order_no;

-- order_status_history
ALTER TABLE order_status_history RENAME COLUMN order_no TO third_order_no;
ALTER INDEX IF EXISTS idx_order_status_history_order_no RENAME TO idx_order_status_history_third_order_no;

-- Nombre de constraint FK (si existe con el nombre original de 006)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_order_status_history_order_no'
    ) THEN
        ALTER TABLE order_status_history
            RENAME CONSTRAINT fk_order_status_history_order_no
            TO fk_order_status_history_third_order_no;
    END IF;
END $$;

-- Trigger: debe leer NEW/OLD.third_order_no
CREATE OR REPLACE FUNCTION log_order_status_transition()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO order_status_history (third_order_no, from_status, to_status, changed_at)
        VALUES (NEW.third_order_no, NULL, NEW.status, now());
    ELSIF TG_OP = 'UPDATE' THEN
        IF OLD.status IS DISTINCT FROM NEW.status THEN
            INSERT INTO order_status_history (third_order_no, from_status, to_status, changed_at)
            VALUES (NEW.third_order_no, OLD.status, NEW.status, now());
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
