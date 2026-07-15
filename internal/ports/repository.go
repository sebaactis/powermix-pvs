// Package ports: interfaces hacia DB y clientes externos (adapters fuera).
package ports

import (
	"context"
	"time"

	"github.com/seba/vps-powermix/internal/domain"
)

type OrderRepository interface {
	Create(ctx context.Context, o *domain.Order) error
	GetByThirdOrderNo(ctx context.Context, orderNo string) (*domain.Order, error)
	GetByGsOrderNo(ctx context.Context, gsOrderNo string) (*domain.Order, error)
	GetByPVSQrID(ctx context.Context, qrID string) (*domain.Order, error)
	ListPaymentConfirmedUnnotified(ctx context.Context, limit int) ([]domain.Order, error)
	UpdateStatus(ctx context.Context, orderNo string, status domain.OrderStatus) error
	// UpdateStatusAndFields: estado + campos extra en un solo UPDATE atómico.
	UpdateStatusAndFields(ctx context.Context, orderNo string,
		status domain.OrderStatus, fields map[string]interface{}) error
	GetStaleByStatus(ctx context.Context, status domain.OrderStatus,
		since time.Time, limit int) ([]domain.Order, error)
	// FindRecentDup: mismo device+SKU+precio dentro de ventana; si no hay → ErrOrderNotFound.
	FindRecentDup(ctx context.Context, deviceID, objectID string,
		priceCents int64, since time.Time) (*domain.Order, error)
	// UpdateStatusGuarded: UPDATE solo si status actual == expected (anti-race webhook/reconciler).
	UpdateStatusGuarded(ctx context.Context, orderNo string,
		expectedStatus, newStatus domain.OrderStatus) (updated bool, err error)
	UpdateStatusGuardedAndFields(ctx context.Context, orderNo string,
		expectedStatus, newStatus domain.OrderStatus,
		fields map[string]interface{}) (updated bool, err error)
}

type RefundRepository interface {
	Create(ctx context.Context, r *domain.Refund) error
	GetByRefundNo(ctx context.Context, refundNo string) (*domain.Refund, error)
	GetLatestByThirdOrderNo(ctx context.Context, thirdOrderNo string) (*domain.Refund, error)
	UpdateStatus(ctx context.Context, refundNo string,
		status domain.RefundStatus) error
}

// IdempotencyStore: insert de clave única; false = webhook duplicado.
type IdempotencyStore interface {
	TryInsert(ctx context.Context, key string) (inserted bool, err error)
}

type SyncLogEntry struct {
	ThirdOrderNo string
	Vendor       string // "PVS" | "GS"
	Direction    string // "outbound" | "inbound"
	Endpoint     string
	Method       string
	RequestBody  string
	StatusCode   int
	LatencyMs    int64
	Error        string
	CreatedAt    time.Time
}

// SyncLogRepo: auditoría best-effort (no debe tumbar el flujo principal).
type SyncLogRepo interface {
	Insert(ctx context.Context, entry *SyncLogEntry) error
}

type ReconcilerRun struct {
	StartedAt    time.Time
	FinishedAt   time.Time
	ScannedCount int
	FixedCount   int
	Notes        string
}

type ReconcilerStore interface {
	ScanStuckOrders(ctx context.Context, batchSize int) ([]domain.Order, error)
	RecordRun(ctx context.Context, run *ReconcilerRun) error
}
