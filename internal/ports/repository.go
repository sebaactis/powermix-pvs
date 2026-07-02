// Package ports define las interfaces (puertos) que conectan el dominio
// con el mundo exterior. Las implementaciones (adapters) viven en
// internal/store (Postgres) e internal/client/{gs,pvs} (HTTP).
package ports

import (
	"context"
	"time"

	"github.com/seba/vps-powermix/internal/domain"
)

// OrderRepository: acceso a la tabla de ordenes en la base de datos.
type OrderRepository interface {
	// Create persiste una nueva orden.
	Create(ctx context.Context, o *domain.Order) error

	// GetByOrderNo busca una orden por nuestro identificador unico.
	GetByOrderNo(ctx context.Context, orderNo string) (*domain.Order, error)

	// GetByPVSQrID busca una orden por el ID del QR de PVS.
	GetByPVSQrID(ctx context.Context, qrID string) (*domain.Order, error)

	// UpdateStatus actualiza el estado interno de una orden.
	// Usa SELECT ... FOR UPDATE si se pasa una transaccion explicita.
	UpdateStatus(ctx context.Context, orderNo string, status domain.OrderStatus) error

	// GetStaleByStatus devuelve ordenes en un estado dado que llevan
	// mas de since sin cambios. Para el reconciler.
	GetStaleByStatus(ctx context.Context, status domain.OrderStatus,
		since time.Time, limit int) ([]domain.Order, error)
}

// RefundRepository: acceso a la tabla de reembolsos.
type RefundRepository interface {
	Create(ctx context.Context, r *domain.Refund) error

	GetByRefundNo(ctx context.Context, refundNo string) (*domain.Refund, error)

	UpdateStatus(ctx context.Context, refundNo string,
		status domain.RefundStatus) error
}

// IdempotencyStore: tabla de claves de idempotencia para webhooks.
// Cada webhook entrante intenta insertar una clave unica.
// Si la clave ya existe, es un duplicado y se ignora.
type IdempotencyStore interface {
	// TryInsert intenta insertar la clave. Devuelve true si se inserto
	// (primera vez), false si ya existia (duplicado).
	TryInsert(ctx context.Context, key string) (inserted bool, err error)
}

// SyncLogEntry es un registro de auditoria de una llamada a PVS o GS.
type SyncLogEntry struct {
	OrderNo     string // orden asociada
	Vendor      string // "PVS" o "GS"
	Direction   string // "outbound" o "inbound"
	Endpoint    string // ruta del endpoint
	Method      string // GET, POST
	RequestBody string // body del request (redactado)
	StatusCode  int    // codigo HTTP de respuesta
	LatencyMs   int64  // duracion en milisegundos
	Error       string // mensaje de error si lo hubo
	CreatedAt   time.Time
}

// SyncLogRepo: registro de auditoria para todas las llamadas a
// PVS y GS. Se escribe siempre, incluso si falla (best-effort).
type SyncLogRepo interface {
	Insert(ctx context.Context, entry *SyncLogEntry) error
}

// ReconcilerRun es un registro de una ejecucion del reconciler.
type ReconcilerRun struct {
	StartedAt    time.Time
	FinishedAt   time.Time
	ScannedCount int // ordenes examinadas
	FixedCount   int // ordenes corregidas
	Notes        string
}

// ReconcilerStore: registro de ejecuciones del worker de reconciliacion.
type ReconcilerStore interface {
	// ScanStuckOrders busca ordenes que necesitan reconciliacion.
	ScanStuckOrders(ctx context.Context, batchSize int) ([]domain.Order, error)

	// RecordRun persiste una ejecucion del reconciler.
	RecordRun(ctx context.Context, run *ReconcilerRun) error
}
