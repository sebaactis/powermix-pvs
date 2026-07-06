package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/seba/vps-powermix/internal/domain"
)

// PostgresRefundRepository implementa ports.RefundRepository sobre Postgres.
type PostgresRefundRepository struct {
	db *sqlx.DB
}

// NewPostgresRefundRepository crea un repositorio de reembolsos listo para usar.
func NewPostgresRefundRepository(db *sqlx.DB) *PostgresRefundRepository {
	return &PostgresRefundRepository{db: db}
}

const columnasRefund = `refund_no, order_no, price_cents, motivo, status,
	pvs_reverse_id, gs_refund_no, requested_at, completed_at, error,
	created_at, updated_at`

// Create persiste un nuevo reembolso.
func (r *PostgresRefundRepository) Create(ctx context.Context, rf *domain.Refund) error {
	query := `INSERT INTO refunds (refund_no, order_no, price_cents, motivo, status, gs_refund_no)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, requested_at, created_at`

	err := r.db.QueryRowContext(ctx, query,
		rf.RefundNo, rf.OrderNo, rf.PriceCents, rf.Motivo, rf.Status, rf.GSRefundNo,
	).Scan(&rf.ID, &rf.RequestedAt, &rf.CreatedAt)

	if err != nil {
		return fmt.Errorf("creando reembolso %s: %w", rf.RefundNo, err)
	}
	return nil
}

// GetByRefundNo busca un reembolso por su numero (idempotencia).
func (r *PostgresRefundRepository) GetByRefundNo(ctx context.Context, refundNo string) (*domain.Refund, error) {
	var (
		rf          domain.Refund
		pvsRevID    sql.NullString
		reqAt       sql.NullString
		completedAt sql.NullString
	)

	query := `SELECT id, ` + columnasRefund + ` FROM refunds WHERE refund_no = $1`

	err := r.db.QueryRowContext(ctx, query, refundNo).Scan(
		&rf.ID,
		&rf.RefundNo, &rf.OrderNo, &rf.PriceCents, &rf.Motivo, &rf.Status,
		&pvsRevID, &rf.GSRefundNo, &reqAt, &completedAt, &rf.Error,
		&rf.CreatedAt, &rf.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrRefundNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscando reembolso %s: %w", refundNo, err)
	}

	rf.PvsReverseID = pvsRevID.String
	rf.RequestedAt = parseNullableTime(reqAt)
	rf.CompletedAt = parseNullableTime(completedAt)
	return &rf, nil
}

// UpdateStatus actualiza el estado de un reembolso. Marca updated_at automaticamente.
func (r *PostgresRefundRepository) UpdateStatus(ctx context.Context, refundNo string, status domain.RefundStatus) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE refunds SET status = $1, updated_at = now() WHERE refund_no = $2`,
		status, refundNo)
	if err != nil {
		return fmt.Errorf("actualizando reembolso %s: %w", refundNo, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return domain.ErrRefundNotFound
	}
	return nil
}
