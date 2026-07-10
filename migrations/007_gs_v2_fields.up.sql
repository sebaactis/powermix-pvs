-- 007_gs_v2_fields.up.sql
-- Campos GS Open API v2: orderNo de la maquina, notifyUrl y marca de notify saliente.
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS gs_order_no TEXT,
    ADD COLUMN IF NOT EXISTS notify_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gs_notified_at TIMESTAMPTZ;

-- Unicidad del serial GS (filas legacy pueden quedar NULL/vacias).
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_gs_order_no
    ON orders (gs_order_no)
    WHERE gs_order_no IS NOT NULL AND gs_order_no <> '';

-- Reconciler: pagos confirmados pendientes de avisar a GS.
CREATE INDEX IF NOT EXISTS idx_orders_notify_pending
    ON orders (status, gs_notified_at)
    WHERE status = 'PAYMENT_CONFIRMED' AND gs_notified_at IS NULL;
