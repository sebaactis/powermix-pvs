-- 003_api_sync_log.up.sql
-- Registro de auditoria para todas las llamadas a PVS y GS.
-- Cada fila es una llamada HTTP (request + response).
-- Se escribe siempre, incluso si falla (best-effort).
CREATE TABLE api_sync_log (
    id           BIGSERIAL PRIMARY KEY,
    order_no     TEXT REFERENCES orders(order_no) ON DELETE CASCADE,
    vendor       TEXT NOT NULL,          -- "PVS" o "GS"
    direction    TEXT NOT NULL,          -- "outbound" o "inbound"
    endpoint     TEXT NOT NULL,          -- ruta del endpoint
    method       TEXT NOT NULL DEFAULT '',
    request_body TEXT NOT NULL DEFAULT '', -- body redactado (sin secrets)
    status_code  INT NOT NULL DEFAULT 0,
    latency_ms   INT NOT NULL DEFAULT 0,
    error        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sync_log_order ON api_sync_log(order_no, created_at DESC);
CREATE INDEX idx_sync_log_vendor ON api_sync_log(vendor, endpoint, created_at DESC);
