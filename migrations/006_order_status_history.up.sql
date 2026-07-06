-- 006_order_status_history.up.sql
-- Tabla para registrar el historial de transiciones de estado de las órdenes.
CREATE TABLE order_status_history (
    id          BIGSERIAL PRIMARY KEY,
    order_no    TEXT NOT NULL,
    from_status TEXT, -- NULL indica el estado inicial al crear la orden
    to_status   TEXT NOT NULL,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_order_status_history_order_no FOREIGN KEY (order_no) REFERENCES orders(order_no) ON DELETE CASCADE
);

-- Índices para optimizar búsquedas por orden y tiempo
CREATE INDEX idx_order_status_history_order_no ON order_status_history(order_no);
CREATE INDEX idx_order_status_history_changed_at ON order_status_history(changed_at);

-- Función del trigger para registrar cambios de estado
CREATE OR REPLACE FUNCTION log_order_status_transition()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO order_status_history (order_no, from_status, to_status, changed_at)
        VALUES (NEW.order_no, NULL, NEW.status, now());
    ELSIF TG_OP = 'UPDATE' THEN
        -- Solo registramos si el estado ha cambiado realmente
        IF OLD.status IS DISTINCT FROM NEW.status THEN
            INSERT INTO order_status_history (order_no, from_status, to_status, changed_at)
            VALUES (NEW.order_no, OLD.status, NEW.status, now());
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger asociado a la tabla orders
CREATE TRIGGER trg_order_status_history
AFTER INSERT OR UPDATE ON orders
FOR EACH ROW
EXECUTE FUNCTION log_order_status_transition();
