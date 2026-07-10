package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/ports"
)

// --- Mocks ---

type mockReconcilerStore struct {
	runs  []ports.ReconcilerRun
	stuck []domain.Order
}

func (m *mockReconcilerStore) ScanStuckOrders(_ context.Context, _ int) ([]domain.Order, error) {
	return m.stuck, nil
}

func (m *mockReconcilerStore) RecordRun(_ context.Context, run *ports.ReconcilerRun) error {
	m.runs = append(m.runs, *run)
	return nil
}

type mockOrderRepo struct {
	statusActual domain.OrderStatus
	updateCalls  int
	unnotified   []domain.Order
}

func (m *mockOrderRepo) Create(_ context.Context, _ *domain.Order) error { return nil }
func (m *mockOrderRepo) GetByThirdOrderNo(_ context.Context, _ string) (*domain.Order, error) {
	return nil, domain.ErrOrderNotFound
}
func (m *mockOrderRepo) GetByPVSQrID(_ context.Context, _ string) (*domain.Order, error) {
	return nil, domain.ErrOrderNotFound
}
func (m *mockOrderRepo) UpdateStatus(_ context.Context, _ string, _ domain.OrderStatus) error {
	return nil
}
func (m *mockOrderRepo) UpdateStatusAndFields(_ context.Context, _ string, _ domain.OrderStatus, _ map[string]interface{}) error {
	return nil
}
func (m *mockOrderRepo) GetStaleByStatus(_ context.Context, _ domain.OrderStatus, _ time.Time, _ int) ([]domain.Order, error) {
	return nil, nil
}
func (m *mockOrderRepo) FindRecentDup(_ context.Context, _, _ string, _ int64, _ time.Time) (*domain.Order, error) {
	return nil, domain.ErrOrderNotFound
}

func (m *mockOrderRepo) UpdateStatusGuarded(_ context.Context, _ string, expected, new domain.OrderStatus) (bool, error) {
	if m.statusActual != expected {
		return false, nil
	}
	m.statusActual = new
	m.updateCalls++
	return true, nil
}

func (m *mockOrderRepo) GetByGsOrderNo(_ context.Context, _ string) (*domain.Order, error) {
	return nil, domain.ErrOrderNotFound
}

func (m *mockOrderRepo) ListPaymentConfirmedUnnotified(_ context.Context, _ int) ([]domain.Order, error) {
	return m.unnotified, nil
}

func (m *mockOrderRepo) UpdateStatusGuardedAndFields(_ context.Context, _ string, expected, new domain.OrderStatus, _ map[string]interface{}) (bool, error) {
	if m.statusActual != expected {
		return false, nil
	}
	m.statusActual = new
	m.updateCalls++
	return true, nil
}

type mockPVSClient struct {
	queryResp *ports.PVSQueryResponse
	queryErr  error
}

func (m *mockPVSClient) QueryStatus(_ context.Context, _ string) (*ports.PVSQueryResponse, error) {
	return m.queryResp, m.queryErr
}

func (m *mockPVSClient) GenerateQR(_ context.Context, _ *ports.PVSQRRequest) (*ports.PVSQRResponse, error) {
	return nil, nil
}
func (m *mockPVSClient) Reverse(_ context.Context, _ string) (*ports.PVSReverseResponse, error) {
	return nil, nil
}

// mockNotifier simula el notify a GS del OrderService.
type mockNotifier struct {
	calls        int
	last         *domain.Order
	markNotified bool
}

func (m *mockNotifier) NotifyPaymentIfNeeded(_ context.Context, o *domain.Order) {
	m.calls++
	cp := *o
	m.last = &cp
	if m.markNotified {
		o.GsNotifiedAt = time.Now()
	}
}

// --- Tests ---

func TestReconciler_RunCancellation(t *testing.T) {
	r := New(&mockReconcilerStore{}, &mockOrderRepo{}, &mockPVSClient{}, nil, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.Run(ctx)

	if err != context.Canceled {
		t.Errorf("Run devolvio %v, esperaba context.Canceled", err)
	}
}

func TestReconciler_ScanSinOrdenesColgadas(t *testing.T) {
	r := New(&mockReconcilerStore{}, &mockOrderRepo{}, &mockPVSClient{}, nil, time.Hour)
	r.scan(context.Background())
}

func TestReconciler_ScanQRShownVencidaTimeout(t *testing.T) {
	store := &mockReconcilerStore{
		stuck: []domain.Order{{
			ThirdOrderNo: "ord-001",
			Status:       domain.OrderQRShown,
			PvsQrID:      "qr-001",
		}},
	}
	orderRepo := &mockOrderRepo{statusActual: domain.OrderQRShown}
	pvs := &mockPVSClient{queryResp: &ports.PVSQueryResponse{StateID: 6}} // IN_PROCESS

	r := New(store, orderRepo, pvs, nil, time.Hour)
	r.scan(context.Background())

	if orderRepo.statusActual != domain.OrderTimeout {
		t.Errorf("status = %q, esperaba TIMEOUT", orderRepo.statusActual)
	}
	if orderRepo.updateCalls != 1 {
		t.Errorf("updateCalls = %d, esperaba 1", orderRepo.updateCalls)
	}
}

func TestReconciler_ScanQRShownVencidaPaymentConfirmed(t *testing.T) {
	store := &mockReconcilerStore{
		stuck: []domain.Order{{
			ThirdOrderNo: "ord-002",
			Status:       domain.OrderQRShown,
			PvsQrID:      "qr-002",
		}},
	}
	orderRepo := &mockOrderRepo{statusActual: domain.OrderQRShown}
	pvs := &mockPVSClient{queryResp: &ports.PVSQueryResponse{StateID: 5}} // APPROVED

	r := New(store, orderRepo, pvs, nil, time.Hour)
	r.scan(context.Background())

	if orderRepo.statusActual != domain.OrderPaymentConfirmed {
		t.Errorf("status = %q, esperaba PAYMENT_CONFIRMED", orderRepo.statusActual)
	}
	if orderRepo.updateCalls != 1 {
		t.Errorf("updateCalls = %d, esperaba 1", orderRepo.updateCalls)
	}
}

func TestReconciler_ScanRefundPendingConfirmado(t *testing.T) {
	store := &mockReconcilerStore{
		stuck: []domain.Order{{
			ThirdOrderNo: "ord-003",
			Status:       domain.OrderRefundPending,
			PvsQrID:      "qr-003",
		}},
	}
	orderRepo := &mockOrderRepo{statusActual: domain.OrderRefundPending}
	pvs := &mockPVSClient{queryResp: &ports.PVSQueryResponse{StateID: 4}} // REVERSED

	r := New(store, orderRepo, pvs, nil, time.Hour)
	r.scan(context.Background())

	if orderRepo.statusActual != domain.OrderRefunded {
		t.Errorf("status = %q, esperaba REFUNDED", orderRepo.statusActual)
	}
	if orderRepo.updateCalls != 1 {
		t.Errorf("updateCalls = %d, esperaba 1", orderRepo.updateCalls)
	}
}

func TestReconciler_ScanRacePerdida(t *testing.T) {
	// Simula race: el webhook ya cambio la orden antes que el reconciler
	store := &mockReconcilerStore{
		stuck: []domain.Order{{
			ThirdOrderNo: "ord-004",
			Status:       domain.OrderQRShown,
			PvsQrID:      "qr-004",
		}},
	}
	orderRepo := &mockOrderRepo{statusActual: domain.OrderPaymentConfirmed} // ya cambio!
	pvs := &mockPVSClient{queryResp: &ports.PVSQueryResponse{StateID: 6}}

	r := New(store, orderRepo, pvs, nil, time.Hour)
	r.scan(context.Background())

	// El status NO deberia cambiar (guarded update lo rechazo)
	if orderRepo.statusActual != domain.OrderPaymentConfirmed {
		t.Errorf("status cambio a %q, esperaba PAYMENT_CONFIRMED (debia rechazar race)",
			orderRepo.statusActual)
	}
	if orderRepo.updateCalls != 0 {
		t.Errorf("updateCalls = %d, esperaba 0 (guarded rechazo)", orderRepo.updateCalls)
	}
}

// AT-07 path: reintenta notify de pagos confirmados sin gs_notified_at.
func TestReconciler_RetryUnnotified(t *testing.T) {
	store := &mockReconcilerStore{}
	orderRepo := &mockOrderRepo{
		unnotified: []domain.Order{{
			ThirdOrderNo: "T-RETRY-1",
			GsOrderNo:    "GS-RETRY-1",
			Status:       domain.OrderPaymentConfirmed,
			NotifyURL:    "https://gs.example/notify",
			UpdatedAt:    time.Now().Add(-2 * time.Minute), // mas vieja que minAge
		}},
	}
	notifier := &mockNotifier{markNotified: true}

	r := New(store, orderRepo, &mockPVSClient{}, notifier, time.Hour)
	r.scan(context.Background())

	if notifier.calls != 1 {
		t.Fatalf("notifier.calls = %d, esperaba 1", notifier.calls)
	}
	if notifier.last == nil || notifier.last.ThirdOrderNo != "T-RETRY-1" {
		t.Fatalf("last notify incorrecto: %+v", notifier.last)
	}
	if len(store.runs) != 1 {
		t.Fatalf("runs = %d, esperaba 1", len(store.runs))
	}
	if store.runs[0].FixedCount < 1 {
		t.Fatalf("FixedCount = %d, esperaba >= 1", store.runs[0].FixedCount)
	}
}

func TestReconciler_RetryUnnotifiedSkipsRecent(t *testing.T) {
	store := &mockReconcilerStore{}
	orderRepo := &mockOrderRepo{
		unnotified: []domain.Order{{
			ThirdOrderNo: "T-RECENT",
			Status:       domain.OrderPaymentConfirmed,
			NotifyURL:    "https://gs.example/notify",
			UpdatedAt:    time.Now(), // recien actualizada
		}},
	}
	notifier := &mockNotifier{markNotified: true}

	r := New(store, orderRepo, &mockPVSClient{}, notifier, time.Hour)
	r.scan(context.Background())

	if notifier.calls != 0 {
		t.Fatalf("notifier.calls = %d, esperaba 0 (minAge)", notifier.calls)
	}
}

func TestReconciler_QRShownToPaidAlsoNotifies(t *testing.T) {
	// Webhook perdido: PVS ya aprobo, reconciler confirma y notifica a GS.
	store := &mockReconcilerStore{
		stuck: []domain.Order{{
			ThirdOrderNo: "T-LOST-WH",
			GsOrderNo:    "GS-LOST",
			Status:       domain.OrderQRShown,
			PvsQrID:      "qr-lost",
			NotifyURL:    "https://gs.example/notify",
			CreatedAt:    time.Now().Add(-5 * time.Minute),
		}},
	}
	orderRepo := &mockOrderRepo{statusActual: domain.OrderQRShown}
	pvs := &mockPVSClient{queryResp: &ports.PVSQueryResponse{StateID: 5}}
	notifier := &mockNotifier{markNotified: true}

	r := New(store, orderRepo, pvs, notifier, time.Hour)
	r.scan(context.Background())

	if orderRepo.statusActual != domain.OrderPaymentConfirmed {
		t.Fatalf("status = %q, esperaba PAYMENT_CONFIRMED", orderRepo.statusActual)
	}
	if notifier.calls != 1 {
		t.Fatalf("notifier.calls = %d, esperaba 1", notifier.calls)
	}
	if notifier.last == nil || notifier.last.ThirdOrderNo != "T-LOST-WH" {
		t.Fatalf("last notify incorrecto: %+v", notifier.last)
	}
}

func TestReconciler_NilNotifierNoPanic(t *testing.T) {
	store := &mockReconcilerStore{}
	orderRepo := &mockOrderRepo{
		unnotified: []domain.Order{{
			ThirdOrderNo: "T-NIL",
			Status:       domain.OrderPaymentConfirmed,
			NotifyURL:    "https://gs.example/notify",
			UpdatedAt:    time.Now().Add(-time.Hour),
		}},
	}
	// notifier nil no debe paniquear
	r := New(store, orderRepo, &mockPVSClient{}, nil, time.Hour)
	r.scan(context.Background())
}
