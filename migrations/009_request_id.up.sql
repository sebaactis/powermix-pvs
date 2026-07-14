-- 009_request_id.up.sql
-- Campo para tracing: vincula cada orden con la request HTTP que la originó.
-- Sin índice (design Q1: no se consulta por request_id en producción).
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS request_id TEXT NOT NULL DEFAULT '';
