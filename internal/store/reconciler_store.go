package store

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/ports"
)

// PostgresReconcilerStore implementa ports.ReconcilerStore sobre Postgres.
type PostgresReconcilerStore struct {
	db *sqlx.DB
}

// NewPostgresReconcilerStore crea un reconciler store listo para usar.
func NewPostgresReconcilerStore(db *sqlx.DB) *PostgresReconcilerStore {
	return &PostgresReconcilerStore{db: db}
}

// ScanStuckOrders busca ordenes que necesitan reconciliacion.
// Incluye dos tipos:
//   - QR_SHOWN con QR vencido (posiblemente el webhook se perdio)
//   - REFUND_PENDING estancado (esperando confirmacion de PVS)
func (r *PostgresReconcilerStore) ScanStuckOrders(ctx context.Context, batchSize int) ([]domain.Order, error) {
	query := `SELECT ` + columnasOrden + ` FROM orders
		WHERE (status = $1 AND qr_expires_at IS NOT NULL AND qr_expires_at < now())
		   OR (status = $2 AND updated_at < now() - interval '10 minutes')
		ORDER BY updated_at ASC LIMIT $3`

	rows, err := r.db.QueryContext(ctx, query,
		domain.OrderQRShown, domain.OrderRefundPending, batchSize)
	if err != nil {
		return nil, fmt.Errorf("buscando ordenes colgadas: %w", err)
	}
	defer rows.Close()

	var ordenes []domain.Order
	for rows.Next() {
		o, err := scanOrderRow(rows)
		if err != nil {
			return nil, err
		}
		ordenes = append(ordenes, *o)
	}
	return ordenes, rows.Err()
}

// RecordRun persiste una ejecucion del reconciler.
func (r *PostgresReconcilerStore) RecordRun(ctx context.Context, run *ports.ReconcilerRun) error {
	query := `INSERT INTO reconciler_runs (started_at, finished_at, scanned_count, fixed_count, notes)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := r.db.ExecContext(ctx, query,
		run.StartedAt, nilIfZero(run.FinishedAt), run.ScannedCount, run.FixedCount, run.Notes)

	if err != nil {
		return fmt.Errorf("registrando ejecucion de reconciler: %w", err)
	}
	return nil
}
