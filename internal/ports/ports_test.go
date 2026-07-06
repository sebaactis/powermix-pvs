package ports

import (
	"context"
	"testing"
	"time"

	"github.com/seba/vps-powermix/internal/domain"
)

// Compile-time checks: verificamos que los mocks vacios satisfacen
// las interfaces. Si alguien cambia una interfaz y no actualiza el
// mock, esto falla en compilacion. No en runtime.
type mockOrderRepo struct{}

var _ OrderRepository = (*mockOrderRepo)(nil)

func (m *mockOrderRepo) Create(_ context.Context, _ *domain.Order) error {
	return nil
}
func (m *mockOrderRepo) GetByOrderNo(_ context.Context, _ string) (*domain.Order, error) {
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

func (m *mockOrderRepo) UpdateStatusGuarded(_ context.Context, _ string, _, _ domain.OrderStatus) (bool, error) {
	return true, nil
}

func (m *mockOrderRepo) UpdateStatusGuardedAndFields(_ context.Context, _ string, _, _ domain.OrderStatus, _ map[string]interface{}) (bool, error) {
	return true, nil
}

// ... similar mocks para las otras interfaces ...

// TestCompileTimeInterfaces verifica que los mocks compilan como
// implementaciones validas de las interfaces.
// No necesita ninguna asercion; compilar es suficiente.
func TestCompileTimeInterfaces(t *testing.T) {
	_ = &mockOrderRepo{}
	_ = &mockRefundRepo{}
	_ = &mockIdempotencyStore{}
	_ = &mockSyncLogRepo{}
	_ = &mockGSClient{}
	_ = &mockPVSClient{}
	_ = &mockTokenCache{}
	_ = &mockReconcilerStore{}
	_ = &mockHealthChecker{}
}

// Mocks minimos para las demas interfaces.

type mockRefundRepo struct{}

var _ RefundRepository = (*mockRefundRepo)(nil)

func (m *mockRefundRepo) Create(_ context.Context, _ *domain.Refund) error {
	return nil
}
func (m *mockRefundRepo) GetByRefundNo(_ context.Context, _ string) (*domain.Refund, error) {
	return nil, domain.ErrRefundNotFound
}
func (m *mockRefundRepo) UpdateStatus(_ context.Context, _ string, _ domain.RefundStatus) error {
	return nil
}

type mockIdempotencyStore struct{}

var _ IdempotencyStore = (*mockIdempotencyStore)(nil)

func (m *mockIdempotencyStore) TryInsert(_ context.Context, _ string) (bool, error) {
	return true, nil
}

type mockSyncLogRepo struct{}

var _ SyncLogRepo = (*mockSyncLogRepo)(nil)

func (m *mockSyncLogRepo) Insert(_ context.Context, _ *SyncLogEntry) error {
	return nil
}

type mockGSClient struct{}

var _ GSClient = (*mockGSClient)(nil)

func (m *mockGSClient) QueryStatus(_ context.Context, _ *GSQueryRequest) (*GSQueryResponse, error) {
	return &GSQueryResponse{OrderStatus: 1}, nil
}
func (m *mockGSClient) Refund(_ context.Context, _ *GSRefundRequest) (*GSRefundResponse, error) {
	return &GSRefundResponse{RefundStatus: "success"}, nil
}

type mockPVSClient struct{}

var _ PVSClient = (*mockPVSClient)(nil)

func (m *mockPVSClient) GenerateQR(_ context.Context, _ *PVSQRRequest) (*PVSQRResponse, error) {
	return &PVSQRResponse{QrID: "qr_test", QrImage: "base64_fake"}, nil
}
func (m *mockPVSClient) QueryStatus(_ context.Context, _ string) (*PVSQueryResponse, error) {
	return &PVSQueryResponse{StateID: 5, Status: "APPROVED"}, nil
}
func (m *mockPVSClient) Reverse(_ context.Context, _ string) (*PVSReverseResponse, error) {
	return &PVSReverseResponse{Success: true}, nil
}

type mockTokenCache struct{}

var _ TokenCache = (*mockTokenCache)(nil)

func (m *mockTokenCache) Get(_ context.Context) (string, error) {
	return "mock_token", nil
}
func (m *mockTokenCache) Invalidate(_ context.Context) {}

type mockReconcilerStore struct{}

var _ ReconcilerStore = (*mockReconcilerStore)(nil)

func (m *mockReconcilerStore) ScanStuckOrders(_ context.Context, _ int) ([]domain.Order, error) {
	return nil, nil
}
func (m *mockReconcilerStore) RecordRun(_ context.Context, _ *ReconcilerRun) error {
	return nil
}

type mockHealthChecker struct{}

var _ HealthChecker = (*mockHealthChecker)(nil)

func (m *mockHealthChecker) PingDB(_ context.Context) error {
	return nil
}
func (m *mockHealthChecker) CheckClockDrift(_ context.Context) (time.Duration, error) {	return 0, nil
}
