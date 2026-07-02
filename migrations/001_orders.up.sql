-- 001_orders.up.sql
-- Tabla principal del sistema. Cada fila es un pedido entre GS y PVS.
-- Arranca sin FK a otras tablas porque orders es la tabla raiz.
CREATE TABLE orders (
    id                BIGSERIAL PRIMARY KEY,
    order_no          TEXT UNIQUE NOT NULL,           -- UUIDv7, nuestro identificador
    device_id         TEXT NOT NULL,                   -- ID de la maquina GS
    device_no         TEXT NOT NULL DEFAULT '',         -- Numero de serie
    object_id         TEXT NOT NULL,                    -- SKU del producto
    price_cents       BIGINT NOT NULL,                  -- Precio en centavos, nunca float
    pay_method        TEXT NOT NULL DEFAULT '',          -- ej: "wxpay"
    way_code          TEXT NOT NULL DEFAULT '',          -- ej: "qr"
    status            TEXT NOT NULL DEFAULT 'RECEIVED', -- nuestro estado interno
    gs_order_status   INT NOT NULL DEFAULT 0,           -- 1-6 de GS (0 = no disponible)
    pvs_status        TEXT NOT NULL DEFAULT '',          -- estado de PVS

    -- QR
    pvs_qr_id         TEXT,                              -- ID del QR en PVS (nullable)
    pvs_qr_image      TEXT NOT NULL DEFAULT '',           -- QR en base64

    -- Timestamps del ciclo de vida
    qr_generated_at       TIMESTAMPTZ,
    qr_expires_at         TIMESTAMPTZ,
    payment_confirmed_at  TIMESTAMPTZ,
    gs_completed_at       TIMESTAMPTZ,  -- outStockStatus=2 recibido
    gs_cancelled_at       TIMESTAMPTZ,  -- cancelacion recibida
    refunded_at           TIMESTAMPTZ,  -- reembolso completado
    failure_reason        TEXT NOT NULL DEFAULT '',

    -- Metadata
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indices
CREATE UNIQUE INDEX idx_orders_order_no ON orders(order_no);
CREATE UNIQUE INDEX idx_orders_pvs_qr_id ON orders(pvs_qr_id) WHERE pvs_qr_id IS NOT NULL;
CREATE INDEX idx_orders_status_created ON orders(status, created_at);
