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
}

func (m *mockOrderRepo) Create(_ context.Context, _ *domain.Order) error { return nil }
func (m *mockOrderRepo) GetByOrderNo(_ context.Context, _ string) (*domain.Order, error) { return nil, domain.ErrOrderNotFound }
func (m *mockOrderRepo) GetByPVSQrID(_ context.Context, _ string) (*domain.Order, error) { return nil, domain.ErrOrderNotFound }
func (m *mockOrderRepo) UpdateStatus(_ context.Context, _ string, _ domain.OrderStatus) error { return nil }
func (m *mockOrderRepo) UpdateStatusAndFields(_ context.Context, _ string, _ domain.OrderStatus, _ map[string]interface{}) error { return nil }
func (m *mockOrderRepo) GetStaleByStatus(_ context.Context, _ domain.OrderStatus, _ time.Time, _ int) ([]domain.Order, error) { return nil, nil }
func (m *mockOrderRepo) FindRecentDup(_ context.Context, _, _ string, _ int64, _ time.Time) (*domain.Order, error) { return nil, domain.ErrOrderNotFound }

func (m *mockOrderRepo) UpdateStatusGuarded(_ context.Context, _ string, expected, new domain.OrderStatus) (bool, error) {
	if m.statusActual != expected {
		return false, nil
	}
	m.statusActual = new
	m.updateCalls++
	return true, nil
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

// --- Tests ---

func TestReconciler_RunCancellation(t *testing.T) {
	r := New(&mockReconcilerStore{}, &mockOrderRepo{}, &mockPVSClient{}, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.Run(ctx)

	if err != context.Canceled {
		t.Errorf("Run devolvio %v, esperaba context.Canceled", err)
	}
}

func TestReconciler_ScanSinOrdenesColgadas(t *testing.T) {
	r := New(&mockReconcilerStore{}, &mockOrderRepo{}, &mockPVSClient{}, time.Hour)
	r.scan(context.Background())
}

func TestReconciler_ScanQRShownVencidaTimeout(t *testing.T) {
	store := &mockReconcilerStore{
		stuck: []domain.Order{{
			OrderNo:  "ord-001",
			Status:   domain.OrderQRShown,
			PvsQrID:  "qr-001",
		}},
	}
	orderRepo := &mockOrderRepo{statusActual: domain.OrderQRShown}
	pvs := &mockPVSClient{queryResp: &ports.PVSQueryResponse{StateID: 6}} // IN_PROCESS

	r := New(store, orderRepo, pvs, time.Hour)
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
			OrderNo:  "ord-002",
			Status:   domain.OrderQRShown,
			PvsQrID:  "qr-002",
		}},
	}
	orderRepo := &mockOrderRepo{statusActual: domain.OrderQRShown}
	pvs := &mockPVSClient{queryResp: &ports.PVSQueryResponse{StateID: 5}} // APPROVED

	r := New(store, orderRepo, pvs, time.Hour)
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
			OrderNo:  "ord-003",
			Status:   domain.OrderRefundPending,
			PvsQrID:  "qr-003",
		}},
	}
	orderRepo := &mockOrderRepo{statusActual: domain.OrderRefundPending}
	pvs := &mockPVSClient{queryResp: &ports.PVSQueryResponse{StateID: 4}} // REVERSED

	r := New(store, orderRepo, pvs, time.Hour)
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
			OrderNo: "ord-004",
			Status:  domain.OrderQRShown,
			PvsQrID: "qr-004",
		}},
	}
	orderRepo := &mockOrderRepo{statusActual: domain.OrderPaymentConfirmed} // ya cambio!
	pvs := &mockPVSClient{queryResp: &ports.PVSQueryResponse{StateID: 6}}

	r := New(store, orderRepo, pvs, time.Hour)
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
