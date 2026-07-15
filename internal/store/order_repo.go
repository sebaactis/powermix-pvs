// Package store contiene las implementaciones Postgres de las
// interfaces definidas en internal/ports.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/seba/vps-powermix/internal/domain"
)

// PostgresOrderRepository es la implementacion concreta de
// ports.OrderRepository sobre PostgreSQL.
type PostgresOrderRepository struct {
	db *sqlx.DB
}

func NewPostgresOrderRepository(db *sqlx.DB) *PostgresOrderRepository {
	return &PostgresOrderRepository{db: db}
}

// columnas usadas en SELECT y RETURNING.
const columnasOrden = `third_order_no, gs_order_no, device_id, device_no, object_id, price_cents,
	pay_method, way_code, status, gs_order_status, pvs_status,
	pvs_qr_id, pvs_qr_image, notify_url, gs_notified_at,
	qr_generated_at, qr_expires_at, payment_confirmed_at,
	gs_completed_at, gs_cancelled_at, refunded_at,
	failure_reason, request_id, created_at, updated_at`

func (r *PostgresOrderRepository) Create(ctx context.Context, o *domain.Order) error {
	query := `INSERT INTO orders (` + columnasOrden + `)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,now(),now())
		RETURNING id, created_at`

	err := r.db.QueryRowContext(ctx, query,
		o.ThirdOrderNo, nilIfVacio(o.GsOrderNo), o.DeviceID, o.DeviceNo, o.ObjectID, o.PriceCents,
		o.PayMethod, o.WayCode, o.Status, o.GsOrderStatus, o.PvsStatus,
		nilIfVacio(o.PvsQrID), o.PvsQrImage, o.NotifyURL, nilIfZero(o.GsNotifiedAt),
		nilIfZero(o.QrGeneratedAt), nilIfZero(o.QrExpiresAt),
		nilIfZero(o.PaymentConfirmedAt), nilIfZero(o.GsCompletedAt),
		nilIfZero(o.GsCancelledAt), nilIfZero(o.RefundedAt),
		o.FailureReason, nilIfVacio(o.RequestID),
	).Scan(&o.ID, &o.CreatedAt)

	if err != nil {
		return fmt.Errorf("creando orden %s: %w", o.ThirdOrderNo, err)
	}
	return nil
}

// GetByThirdOrderNo busca una orden por nuestro identificador unico.
func (r *PostgresOrderRepository) GetByThirdOrderNo(ctx context.Context, orderNo string) (*domain.Order, error) {
	query := `SELECT ` + columnasOrden + ` FROM orders WHERE third_order_no = $1`
	o, err := r.escanearOrden(r.db.QueryRowContext(ctx, query, orderNo))
	if err != nil {
		return nil, fmt.Errorf("buscando orden %s: %w", orderNo, err)
	}
	return o, nil
}

// GetByGsOrderNo busca una orden por el serial orderNo de GS.
func (r *PostgresOrderRepository) GetByGsOrderNo(ctx context.Context, gsOrderNo string) (*domain.Order, error) {
	query := `SELECT ` + columnasOrden + ` FROM orders WHERE gs_order_no = $1`
	o, err := r.escanearOrden(r.db.QueryRowContext(ctx, query, gsOrderNo))
	if err != nil {
		return nil, fmt.Errorf("buscando orden por gs_order_no %s: %w", gsOrderNo, err)
	}
	return o, nil
}

func (r *PostgresOrderRepository) ListPaymentConfirmedUnnotified(ctx context.Context, limit int) ([]domain.Order, error) {
	query := `SELECT ` + columnasOrden + ` FROM orders
		WHERE status = $1 AND gs_notified_at IS NULL
		ORDER BY updated_at ASC LIMIT $2`

	rows, err := r.db.QueryContext(ctx, query, domain.OrderPaymentConfirmed, limit)
	if err != nil {
		return nil, fmt.Errorf("listando ordenes sin notify GS: %w", err)
	}
	defer rows.Close()

	var ordenes []domain.Order
	for rows.Next() {
		o, err := r.escanearOrden(rows)
		if err != nil {
			return nil, err
		}
		ordenes = append(ordenes, *o)
	}
	return ordenes, rows.Err()
}

func (r *PostgresOrderRepository) GetByPVSQrID(ctx context.Context, qrID string) (*domain.Order, error) {
	query := `SELECT ` + columnasOrden + ` FROM orders WHERE pvs_qr_id = $1`
	o, err := r.escanearOrden(r.db.QueryRowContext(ctx, query, qrID))
	if err != nil {
		return nil, fmt.Errorf("buscando orden por QR %s: %w", qrID, err)
	}
	return o, nil
}

// UpdateStatus actualiza el estado interno de una orden.
func (r *PostgresOrderRepository) UpdateStatus(ctx context.Context, orderNo string, status domain.OrderStatus) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE orders SET status = $1, updated_at = now() WHERE third_order_no = $2`,
		status, orderNo)
	if err != nil {
		return fmt.Errorf("actualizando estado de %s a %s: %w", orderNo, status, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return domain.ErrOrderNotFound
	}
	return nil
}

// UpdateStatusAndFields actualiza el estado junto con otros
// campos (ej: payment_confirmed_at, pvs_qr_id, etc) en una sola query.
// Implementa ports.OrderRepository.UpdateStatusAndFields.
func (r *PostgresOrderRepository) UpdateStatusAndFields(ctx context.Context, orderNo string,
	status domain.OrderStatus, actualizaciones map[string]interface{}) error {

	if len(actualizaciones) == 0 {
		return r.UpdateStatus(ctx, orderNo, status)
	}

	setParts := []string{"status = $1", "updated_at = now()"}
	args := []interface{}{status}
	i := 2
	for col, val := range actualizaciones {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}
	args = append(args, orderNo)

	query := fmt.Sprintf("UPDATE orders SET %s WHERE third_order_no = $%d",
		joinString(setParts, ", "), i)

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("actualizando %s: %w", orderNo, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return domain.ErrOrderNotFound
	}
	return nil
}

// UpdateStatusGuarded actualiza el estado SOLO si el estado actual
// coincide con expectedStatus. Devuelve updated=false si otra
// transaccion ya cambio el estado (condicion de carrera).
func (r *PostgresOrderRepository) UpdateStatusGuarded(ctx context.Context,
	orderNo string, expectedStatus, newStatus domain.OrderStatus) (bool, error) {

	result, err := r.db.ExecContext(ctx,
		`UPDATE orders SET status = $1, updated_at = now()
		 WHERE third_order_no = $2 AND status = $3`,
		newStatus, orderNo, expectedStatus)
	if err != nil {
		return false, fmt.Errorf("update guarded %s: %w", orderNo, err)
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// UpdateStatusGuardedAndFields es como UpdateStatusGuarded pero actualiza
// campos adicionales en la misma operacion atomica.
func (r *PostgresOrderRepository) UpdateStatusGuardedAndFields(ctx context.Context,
	orderNo string, expectedStatus, newStatus domain.OrderStatus,
	fields map[string]interface{}) (bool, error) {

	if len(fields) == 0 {
		return r.UpdateStatusGuarded(ctx, orderNo, expectedStatus, newStatus)
	}

	setParts := []string{"status = $1", "updated_at = now()"}
	args := []interface{}{newStatus}
	i := 2
	for col, val := range fields {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}
	args = append(args, orderNo, expectedStatus)

	query := fmt.Sprintf("UPDATE orders SET %s WHERE third_order_no = $%d AND status = $%d",
		joinString(setParts, ", "), i, i+1)

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("update guarded+fields %s: %w", orderNo, err)
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// GetStaleByStatus devuelve ordenes en un estado dado que no se
// actualizaron desde 'desde'. Para el reconciler.
func (r *PostgresOrderRepository) GetStaleByStatus(ctx context.Context,
	status domain.OrderStatus, desde time.Time, limite int) ([]domain.Order, error) {

	query := `SELECT ` + columnasOrden + ` FROM orders
		WHERE status = $1 AND updated_at < $2
		ORDER BY updated_at ASC LIMIT $3`

	var ordenes []domain.Order
	rows, err := r.db.QueryContext(ctx, query, status, desde, limite)
	if err != nil {
		return nil, fmt.Errorf("buscando ordenes estancadas: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		o, err := r.escanearOrden(rows)
		if err != nil {
			return nil, err
		}
		ordenes = append(ordenes, *o)
	}
	return ordenes, rows.Err()
}

// FindRecentDup busca una orden duplicada reciente: mismo dispositivo,
// producto y precio, creada dentro de la ventana since. Para deduplicacion.
// Si no encuentra ninguna, devuelve ErrOrderNotFound (no es error de sistema).
func (r *PostgresOrderRepository) FindRecentDup(ctx context.Context,
	deviceID, objectID string, priceCents int64, since time.Time) (*domain.Order, error) {

	query := `SELECT ` + columnasOrden + ` FROM orders
		WHERE device_id = $1 AND object_id = $2 AND price_cents = $3
		  AND created_at > $4
		ORDER BY created_at DESC LIMIT 1`

	o, err := r.escanearOrden(r.db.QueryRowContext(ctx, query, deviceID, objectID, priceCents, since))
	if err != nil {
		return nil, fmt.Errorf("buscando orden duplicada: %w", err)
	}
	return o, nil
}

// escanearOrden lee una fila de la base de datos y la mapea a domain.Order.
// Delega en scanOrderRow, que es compartido con PostgresReconcilerStore.
func (r *PostgresOrderRepository) escanearOrden(scanner interface {
	Scan(dest ...interface{}) error
}) (*domain.Order, error) {
	return scanOrderRow(scanner)
}


// nilIfVacio devuelve nil si s esta vacio, o s si no.
// Para pasar NULL a la base de datos en lugar de string vacio.
func nilIfVacio(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nilIfZero(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// parseNullableTime convierte sql.NullString a time.Time.
// Si la string esta vacia o invalida, devuelve time.Time{}.
func parseNullableTime(ns sql.NullString) time.Time {
	if !ns.Valid || ns.String == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, ns.String)
	if err != nil {
		return time.Time{}
	}
	return t
}

// joinString une strings con un separador. Alternativa a strings.Join
// para evitar importar strings solo para esto.
func joinString(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
