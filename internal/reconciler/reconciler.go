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

// PaymentNotifier avisa a GS un pago confirmado (best-effort).
// OrderService implementa esta interfaz.
type PaymentNotifier interface {
	NotifyPaymentIfNeeded(ctx context.Context, order *domain.Order)
}

// Reconciler es el worker de reconciliacion de ordenes colgadas.
// Corre periodicamente en background hasta que se cancela el ctx.
// Por cada lote escanea ordenes estancadas, consulta a PVS para
// determinar el estado real, y corrige nuestro estado interno.
// Tambien reintenta notify a GS para pagos sin gs_notified_at.
type Reconciler struct {
	store        ports.ReconcilerStore
	orderRepo    ports.OrderRepository
	pvsClient    ports.PVSClient
	notifier     PaymentNotifier // puede ser nil
	interval     time.Duration
	batchSize    int
	notifyMinAge time.Duration // evita carrera con notify inline del webhook
}

// New crea un Reconciler listo para ejecutar.
// notifier puede ser nil (no se reintenta notify a GS).
func New(store ports.ReconcilerStore, orderRepo ports.OrderRepository,
	pvsClient ports.PVSClient, notifier PaymentNotifier, interval time.Duration) *Reconciler {
	return &Reconciler{
		store:        store,
		orderRepo:    orderRepo,
		pvsClient:    pvsClient,
		notifier:     notifier,
		interval:     interval,
		batchSize:    200,
		notifyMinAge: 30 * time.Second,
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

	// Reintentar notify a GS de pagos confirmados sin gs_notified_at.
	r.retryUnnotified(ctx, run)

	run.FinishedAt = time.Now()
	if err := r.store.RecordRun(ctx, run); err != nil {
		slog.Error("reconciler: RecordRun", "error", err)
		return
	}

	slog.Info("reconciler scan completo",
		"scanned", run.ScannedCount, "fixed", run.FixedCount,
		"duracion", time.Since(start))
}

// retryUnnotified reintenta notify a GS para pagos confirmados sin gs_notified_at.
func (r *Reconciler) retryUnnotified(ctx context.Context, run *ports.ReconcilerRun) {
	if r.notifier == nil {
		return
	}
	pendientes, err := r.orderRepo.ListPaymentConfirmedUnnotified(ctx, r.batchSize)
	if err != nil {
		slog.Error("reconciler: ListPaymentConfirmedUnnotified", "error", err)
		return
	}
	run.ScannedCount += len(pendientes)

	for i := range pendientes {
		o := &pendientes[i]
		// Evitar carrera con el notify inline del webhook (recien confirmado).
		if r.notifyMinAge > 0 && !o.UpdatedAt.IsZero() && time.Since(o.UpdatedAt) < r.notifyMinAge {
			continue
		}
		if o.NotifyURL == "" || !o.GsNotifiedAt.IsZero() {
			continue
		}
		antes := o.GsNotifiedAt
		r.notifier.NotifyPaymentIfNeeded(ctx, o)
		if antes.IsZero() && !o.GsNotifiedAt.IsZero() {
			run.FixedCount++
			slog.Info("reconciler: notify GS reintentado ok",
				"thirdOrderNo", o.ThirdOrderNo)
		}
	}
}

// reconcileQRShown corrige una orden QR_SHOWN con QR vencido.
// Consulta PVS para saber el estado real del QR y actualiza segun corresponda.
func (r *Reconciler) reconcileQRShown(ctx context.Context, order *domain.Order, run *ports.ReconcilerRun) {
	pvsResp, err := r.pvsClient.QueryStatus(ctx, order.PvsQrID)
	if err != nil {
		slog.Error("reconciler: PVS QueryStatus QR_SHOWN",
			"orderNo", order.ThirdOrderNo, "error", err)
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
	updated, err := r.orderRepo.UpdateStatusGuardedAndFields(ctx, order.ThirdOrderNo,
		domain.OrderQRShown, nuevoEstado, fields)
	if err != nil {
		slog.Error("reconciler: update QR_SHOWN",
			"orderNo", order.ThirdOrderNo, "error", err)
		return
	}
	if updated {
		run.FixedCount++
		slog.Info("reconciler: orden corregida",
			"orderNo", order.ThirdOrderNo,
			"de", order.Status, "a", nuevoEstado)

		// Webhook perdido: avisar a GS ahora (sin minAge; recien confirmamos).
		if nuevoEstado == domain.OrderPaymentConfirmed && r.notifier != nil {
			order.Status = domain.OrderPaymentConfirmed
			if order.PaymentConfirmedAt.IsZero() {
				order.PaymentConfirmedAt = time.Now()
			}
			r.notifier.NotifyPaymentIfNeeded(ctx, order)
		}
	}
}

// reconcileRefundPending corrige una orden REFUND_PENDING estancada.
// Consulta PVS para saber si el reverse se completo.
func (r *Reconciler) reconcileRefundPending(ctx context.Context, order *domain.Order, run *ports.ReconcilerRun) {
	pvsResp, err := r.pvsClient.QueryStatus(ctx, order.PvsQrID)
	if err != nil {
		slog.Error("reconciler: PVS QueryStatus REFUND_PENDING",
			"orderNo", order.ThirdOrderNo, "error", err)
		return
	}

	if pvsResp.StateID != 4 { // REVERSED
		// Sigue en proceso, no corregimos
		return
	}

	updated, err := r.orderRepo.UpdateStatusGuarded(ctx, order.ThirdOrderNo,
		domain.OrderRefundPending, domain.OrderRefunded)
	if err != nil {
		slog.Error("reconciler: update REFUND_PENDING",
			"orderNo", order.ThirdOrderNo, "error", err)
		return
	}
	if updated {
		run.FixedCount++
		slog.Info("reconciler: refund confirmado",
			"orderNo", order.ThirdOrderNo)
	}
}
