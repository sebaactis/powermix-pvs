-- 005_reconciler_runs.up.sql
-- Tabla de ejecuciones del reconciler (worker background).
-- Cada fila es una ejecucion periodica que escaneo ordenes colgadas
-- y las corrigio (o intento corregir).
CREATE TABLE reconciler_runs (
    id            BIGSERIAL PRIMARY KEY,
    started_at    TIMESTAMPTZ NOT NULL,
    finished_at   TIMESTAMPTZ,
    scanned_count INT NOT NULL DEFAULT 0,
    fixed_count   INT NOT NULL DEFAULT 0,
    notes         TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
