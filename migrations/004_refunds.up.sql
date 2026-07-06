-- 004_refunds.up.sql
-- Tabla de reembolsos. Cada fila es un pedido de devolucion de GS.
CREATE TABLE refunds (
    id              BIGSERIAL PRIMARY KEY,
    refund_no       TEXT UNIQUE NOT NULL,         -- ID de reembolso de GS (idempotencia)
    order_no        TEXT NOT NULL,                 -- orden original
    price_cents     BIGINT NOT NULL,               -- monto en centavos
    motivo          TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'PENDING',
    pvs_reverse_id  TEXT,                           -- ID del reverse en PVS (nullable)
    gs_refund_no    TEXT NOT NULL DEFAULT '',
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    error           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_refunds_refund_no ON refunds(refund_no);
CREATE INDEX idx_refunds_order_no ON refunds(order_no);
