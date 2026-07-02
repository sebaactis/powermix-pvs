-- 002_idempotency_keys.up.sql
-- Tabla para prevenir procesamiento duplicado de webhooks.
-- Cada webhook entrante genera una clave unica (hash).
-- Si la clave ya existe, es un duplicado y se ignora.
CREATE TABLE idempotency_keys (
    id         BIGSERIAL PRIMARY KEY,
    key_hash   TEXT NOT NULL,                   -- SHA-256 de la clave unica
    order_no   TEXT REFERENCES orders(order_no) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_idempotency_keys_key ON idempotency_keys(key_hash);
CREATE INDEX idx_idempotency_keys_order ON idempotency_keys(order_no);
