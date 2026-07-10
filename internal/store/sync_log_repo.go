package store

import (
	"context"
	"log/slog"

	"github.com/jmoiron/sqlx"

	"github.com/seba/vps-powermix/internal/ports"
)

// PostgresSyncLogRepo es la implementacion concreta de ports.SyncLogRepo
// sobre PostgreSQL.
//
// Es best-effort: si la insercion falla, se loguea internamente pero
// NO se propaga el error al caller. El sync log no debe interrumpir
// el flujo principal de procesamiento de pagos.
type PostgresSyncLogRepo struct {
	db *sqlx.DB
}

// NewPostgresSyncLogRepo crea un SyncLogRepo listo para usar.
func NewPostgresSyncLogRepo(db *sqlx.DB) *PostgresSyncLogRepo {
	return &PostgresSyncLogRepo{db: db}
}

// Insert guarda un registro de auditoria. Siempre devuelve nil incluso
// si la base de datos falla (best-effort).
func (r *PostgresSyncLogRepo) Insert(ctx context.Context, entry *ports.SyncLogEntry) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO api_sync_log
			(third_order_no, vendor, direction, endpoint, method,
			 request_body, status_code, latency_ms, error, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())`,
		entry.ThirdOrderNo, entry.Vendor, entry.Direction, entry.Endpoint, entry.Method,
		entry.RequestBody, entry.StatusCode, entry.LatencyMs, entry.Error,
	)
	if err != nil {
		slog.Warn("no se pudo escribir el sync log (best-effort)",
			"error", err,
			"third_order_no", entry.ThirdOrderNo,
			"vendor", entry.Vendor,
		)
	}
	return nil
}

// ensure interface compliance
var _ ports.SyncLogRepo = (*PostgresSyncLogRepo)(nil)
