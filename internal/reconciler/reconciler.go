// Package reconciler contiene el worker background que escanea ordenes
// colgadas y las corrige consultando a PVS cuando es necesario.
package reconciler

import (
	"context"
	"log/slog"
	"time"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/ports"
)

// Reconciler es el worker de reconciliacion de ordenes colgadas.
// Corre periodicamente en background hasta que se cancela el ctx.
// Por cada lote escanea ordenes estancadas, consulta a PVS para
// determinar el estado real, y corrige nuestro estado interno.
type Reconciler struct {
	store     ports.ReconcilerStore
	orderRepo ports.OrderRepository
	pvsClient ports.PVSClient
	interval  time.Duration
	batchSize int
}

// New crea un Reconciler listo para ejecutar.
func New(store ports.ReconcilerStore, orderRepo ports.OrderRepository,
	pvsClient ports.PVSClient, interval time.Duration) *Reconciler {
	return &Reconciler{
		store:     store,
		orderRepo: orderRepo,
		pvsClient: pvsClient,
		interval:  interval,
		batchSize: 200,
	}
}

// Run ejecuta el loop de reconciliacion. Bloquea hasta que ctx se cancele.
// Llamar como goroutine: go rec.Run(ctx)
func (r *Reconciler) Run(ctx context.Context) error {
	slog.Info("reconciler iniciado", "interval", r.interval, "batchSize", r.batchSize)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reconciler detenido")
			return ctx.Err()
		case <-ticker.C:
			r.scan(ctx)
		}
	}
}

// scan ejecuta una pasada completa del reconciler.
// Escanea ordenes colgadas y despacha segun su estado.
func (r *Reconciler) scan(ctx context.Context) {
	start := time.Now()
	slog.Debug("reconciler scan empezando")

	stuck, err := r.store.ScanStuckOrders(ctx, r.batchSize)
	if err != nil {
		slog.Error("reconciler: ScanStuckOrders", "error", err)
		return
	}

	run := &ports.ReconcilerRun{
		StartedAt:    start,
		ScannedCount: len(stuck),
	}

	for i := range stuck {
		switch stuck[i].Status {
		case domain.OrderQRShown:
			r.reconcileQRShown(ctx, &stuck[i], run)
		case domain.OrderRefundPending:
			r.reconcileRefundPending(ctx, &stuck[i], run)
		}
	}

	run.FinishedAt = time.Now()
	if err := r.store.RecordRun(ctx, run); err != nil {
		slog.Error("reconciler: RecordRun", "error", err)
		return
	}

	slog.Info("reconciler scan completo",
		"scanned", run.ScannedCount, "fixed", run.FixedCount,
		"duracion", time.Since(start))
}

// reconcileQRShown corrige una orden QR_SHOWN con QR vencido.
// Consulta PVS para saber el estado real del QR y actualiza segun corresponda.
func (r *Reconciler) reconcileQRShown(ctx context.Context, order *domain.Order, run *ports.ReconcilerRun) {
	pvsResp, err := r.pvsClient.QueryStatus(ctx, order.PvsQrID)
	if err != nil {
		slog.Error("reconciler: PVS QueryStatus QR_SHOWN",
			"orderNo", order.OrderNo, "error", err)
		return
	}

	var nuevoEstado domain.OrderStatus
	switch pvsResp.StateID {
	case 6: // IN_PROCESS — el QR vencio sin pago
		nuevoEstado = domain.OrderTimeout
	case 5: // APPROVED — el pago se confirmo pero el webhook se perdio
		nuevoEstado = domain.OrderPaymentConfirmed
	case 3: // REJECTED
		nuevoEstado = domain.OrderFailed
	case 4: // REVERSED
		nuevoEstado = domain.OrderRefundPending
	default:
		return
	}

	// Guarded update: solo si sigue en QR_SHOWN (protege contra race con webhook)
	fields := map[string]interface{}{}
	if nuevoEstado == domain.OrderPaymentConfirmed {
		fields["payment_confirmed_at"] = time.Now()
	}
	updated, err := r.orderRepo.UpdateStatusGuardedAndFields(ctx, order.OrderNo,
		domain.OrderQRShown, nuevoEstado, fields)
	if err != nil {
		slog.Error("reconciler: update QR_SHOWN",
			"orderNo", order.OrderNo, "error", err)
		return
	}
	if updated {
		run.FixedCount++
		slog.Info("reconciler: orden corregida",
			"orderNo", order.OrderNo,
			"de", order.Status, "a", nuevoEstado)
	}
}

// reconcileRefundPending corrige una orden REFUND_PENDING estancada.
// Consulta PVS para saber si el reverse se completo.
func (r *Reconciler) reconcileRefundPending(ctx context.Context, order *domain.Order, run *ports.ReconcilerRun) {
	pvsResp, err := r.pvsClient.QueryStatus(ctx, order.PvsQrID)
	if err != nil {
		slog.Error("reconciler: PVS QueryStatus REFUND_PENDING",
			"orderNo", order.OrderNo, "error", err)
		return
	}

	if pvsResp.StateID != 4 { // REVERSED
		// Sigue en proceso, no corregimos
		return
	}

	updated, err := r.orderRepo.UpdateStatusGuarded(ctx, order.OrderNo,
		domain.OrderRefundPending, domain.OrderRefunded)
	if err != nil {
		slog.Error("reconciler: update REFUND_PENDING",
			"orderNo", order.OrderNo, "error", err)
		return
	}
	if updated {
		run.FixedCount++
		slog.Info("reconciler: refund confirmado",
			"orderNo", order.OrderNo)
	}
}
