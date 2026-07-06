package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/ports"
)

// --- Mocks para tests ---

type mockOrderRepo struct {
	creada          *domain.Order
	statusActual    domain.OrderStatus
	failOnCreate    bool
	failOnUpdate    bool
	updateLlamadas  int
}

func (m *mockOrderRepo) Create(_ context.Context, o *domain.Order) error {
	if m.failOnCreate {
		return errors.New("error simulado en Create")
	}
	m.creada = o
	m.statusActual = o.Status
	return nil
}

func (m *mockOrderRepo) GetByOrderNo(_ context.Context, orderNo string) (*domain.Order, error) {
	if m.creada != nil && m.creada.OrderNo == orderNo {
		o := *m.creada
		o.Status = m.statusActual
		return &o, nil
	}
	return nil, domain.ErrOrderNotFound
}

func (m *mockOrderRepo) GetByPVSQrID(_ context.Context, qrID string) (*domain.Order, error) {
	if m.creada != nil && m.creada.PvsQrID == qrID {
		o := *m.creada
		o.Status = m.statusActual
		return &o, nil
	}
	return nil, domain.ErrOrderNotFound
}

func (m *mockOrderRepo) UpdateStatus(_ context.Context, _ string, status domain.OrderStatus) error {
	if m.failOnUpdate {
		return errors.New("error simulado en UpdateStatus")
	}
	m.statusActual = status
	m.updateLlamadas++
	return nil
}

func (m *mockOrderRepo) UpdateStatusAndFields(_ context.Context, _ string, status domain.OrderStatus, fields map[string]interface{}) error {
	if m.failOnUpdate {
		return errors.New("error simulado en UpdateStatusAndFields")
	}
	m.statusActual = status
	m.updateLlamadas++
	if m.creada != nil {
		if v, ok := fields["pvs_qr_id"]; ok {
			if s, ok := v.(string); ok {
				m.creada.PvsQrID = s
			}
		}
		if v, ok := fields["pvs_qr_image"]; ok {
			if s, ok := v.(string); ok {
				m.creada.PvsQrImage = s
			}
		}
	}
	return nil
}

func (m *mockOrderRepo) UpdateStatusGuarded(_ context.Context, _ string, expectedStatus, newStatus domain.OrderStatus) (bool, error) {
	if m.failOnUpdate {
		return false, errors.New("error simulado en UpdateStatusGuarded")
	}
	if m.statusActual != expectedStatus && expectedStatus != "" {
		return false, nil
	}
	m.statusActual = newStatus
	m.updateLlamadas++
	return true, nil
}

func (m *mockOrderRepo) UpdateStatusGuardedAndFields(_ context.Context, _ string, expectedStatus, newStatus domain.OrderStatus, fields map[string]interface{}) (bool, error) {
	if m.failOnUpdate {
		return false, errors.New("error simulado en UpdateStatusGuardedAndFields")
	}
	if m.statusActual != expectedStatus && expectedStatus != "" {
		return false, nil
	}
	m.statusActual = newStatus
	m.updateLlamadas++
	if m.creada != nil {
		if v, ok := fields["pvs_qr_id"]; ok {
			if s, ok := v.(string); ok {
				m.creada.PvsQrID = s
			}
		}
		if v, ok := fields["pvs_qr_image"]; ok {
			if s, ok := v.(string); ok {
				m.creada.PvsQrImage = s
			}
		}
	}
	return true, nil
}

func (m *mockOrderRepo) GetStaleByStatus(_ context.Context, _ domain.OrderStatus, _ time.Time, _ int) ([]domain.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) GetStaleByStatusTime(_ context.Context, _ domain.OrderStatus, _ time.Time, _ int) ([]domain.Order, error) {
	return nil, nil
}

func (m *mockOrderRepo) FindRecentDup(_ context.Context, deviceID, objectID string, priceCents int64, _ time.Time) (*domain.Order, error) {
	if m.creada != nil && m.creada.DeviceID == deviceID &&
		m.creada.ObjectID == objectID && m.creada.PriceCents == priceCents {
		o := *m.creada
		o.Status = m.statusActual
		return &o, nil
	}
	return nil, domain.ErrOrderNotFound
}

type mockPVSClient struct {
	qrResponse  *ports.PVSQRResponse
	failOnGen   bool
	llamadas    int
	reverseErr  bool
	reverseFail bool
}

func (m *mockPVSClient) GenerateQR(_ context.Context, req *ports.PVSQRRequest) (*ports.PVSQRResponse, error) {
	m.llamadas++
	if m.failOnGen {
		return nil, errors.New("PVS error simulado")
	}
	if m.qrResponse != nil {
		return m.qrResponse, nil
	}
	return &ports.PVSQRResponse{QrID: "qr_test_123", QrImage: "base64_fake_image"}, nil
}

func (m *mockPVSClient) QueryStatus(_ context.Context, _ string) (*ports.PVSQueryResponse, error) {
	return &ports.PVSQueryResponse{StateID: 6}, nil
}

func (m *mockPVSClient) Reverse(_ context.Context, _ string) (*ports.PVSReverseResponse, error) {
	if m.reverseErr {
		return nil, errors.New("PVS reverse error simulado")
	}
	return &ports.PVSReverseResponse{Success: !m.reverseFail}, nil
}

type mockSyncLogRepo struct {
	inserts int
}

func (m *mockSyncLogRepo) Insert(_ context.Context, _ *ports.SyncLogEntry) error {
	m.inserts++
	return nil
}

// --- Tests ---

func TestCreateOrder_HappyPath(t *testing.T) {
	repo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	syncLog := &mockSyncLogRepo{}
	svc := NewOrderService(repo, pvs, syncLog)

	resp, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-fresa-001", Subject: "Batido de Frutilla",
		TotalAmount: "150.00", DeviceID: "device-001", PayMethod: "qr", WayCode: "qr",
	})
	if err != nil {
		t.Fatalf("CreateOrder fallo: %v", err)
	}
	if resp.OrderStatus != 1 {
		t.Errorf("OrderStatus = %d, esperaba 1", resp.OrderStatus)
	}
	if resp.QrURL != "base64_fake_image" {
		t.Errorf("QrURL = %q, esperaba base64_fake_image", resp.QrURL)
	}
	if resp.ThirdOrderNo == "" {
		t.Error("ThirdOrderNo vacio")
	}
	if repo.creada == nil {
		t.Fatal("no se creo orden")
	}
	if repo.creada.PriceCents != 15000 {
		t.Errorf("PriceCents = %d, esperaba 15000", repo.creada.PriceCents)
	}
	if pvs.llamadas != 1 {
		t.Errorf("PVS llamado %d veces, esperaba 1", pvs.llamadas)
	}
	if syncLog.inserts != 1 {
		t.Errorf("syncLog inserts = %d, esperaba 1", syncLog.inserts)
	}
}

func TestCreateOrder_PVSFalla(t *testing.T) {
	repo := &mockOrderRepo{}
	pvs := &mockPVSClient{failOnGen: true}
	svc := NewOrderService(repo, pvs, &mockSyncLogRepo{})

	_, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	if err == nil {
		t.Fatal("se esperaba error de PVS")
	}
	if repo.statusActual != domain.OrderFailed {
		t.Errorf("status = %q, esperaba FAILED", repo.statusActual)
	}
}

func TestCreateOrder_ObjectIdVacio(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{})
	_, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{TotalAmount: "100.00"})
	if err == nil {
		t.Fatal("se esperaba error por objectId vacio")
	}
}

func TestCreateOrder_MontoVacio(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{})
	_, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{ObjectID: "drink-test"})
	if err == nil {
		t.Fatal("se esperaba error por monto vacio")
	}
}

func TestCreateOrder_MontoInvalido(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{})
	_, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "no-es-un-numero",
	})
	if err == nil {
		t.Fatal("se esperaba error por monto invalido")
	}
}

func TestCreateOrder_MontoCero(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{})
	_, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "0.00",
	})
	if err == nil {
		t.Fatal("se esperaba error por monto cero")
	}
}

func TestCreateOrder_Dedup(t *testing.T) {
	repo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	svc := NewOrderService(repo, pvs, &mockSyncLogRepo{})

	resp1, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp2, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp1.ThirdOrderNo != resp2.ThirdOrderNo {
		t.Errorf("dedup: esperaba mismo orderNo, got %q y %q", resp1.ThirdOrderNo, resp2.ThirdOrderNo)
	}
	if pvs.llamadas != 1 {
		t.Errorf("PVS GenerateQR llamado %d veces, esperaba 1 (dedup)", pvs.llamadas)
	}
}

func TestCreateOrder_RepoFalla(t *testing.T) {
	repo := &mockOrderRepo{failOnCreate: true}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})
	_, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	if err == nil {
		t.Fatal("se esperaba error del repositorio")
	}
}

// --- QueryStatus ---

func TestQueryStatus_HappyPath(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	created, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := svc.QueryStatus(context.Background(), &QueryStatusRequest{ThirdOrderNo: created.ThirdOrderNo})
	if err != nil {
		t.Fatalf("QueryStatus fallo: %v", err)
	}
	if resp.OrderStatus != 1 {
		t.Errorf("OrderStatus = %d, esperaba 1 (pending)", resp.OrderStatus)
	}
}

func TestQueryStatus_OrdenInexistente(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{})
	_, err := svc.QueryStatus(context.Background(), &QueryStatusRequest{ThirdOrderNo: "no-existe"})
	if err == nil {
		t.Fatal("se esperaba error por orden inexistente")
	}
}

func TestQueryStatus_ThirdOrderNoVacio(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{})
	_, err := svc.QueryStatus(context.Background(), &QueryStatusRequest{})
	if err == nil {
		t.Fatal("se esperaba error por thirdOrderNo vacio")
	}
}

// --- Webhook ---

func TestHandlePVSWebhook_PagoConfirmado(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	_, err := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 5})
	if err != nil {
		t.Fatalf("HandlePVSWebhook fallo: %v", err)
	}
	if repo.statusActual != domain.OrderPaymentConfirmed {
		t.Errorf("status = %q, esperaba PAYMENT_CONFIRMED", repo.statusActual)
	}
}

func TestHandlePVSWebhook_Rechazado(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	_, _ = svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 3})
	if repo.statusActual != domain.OrderFailed {
		t.Errorf("status = %q, esperaba FAILED", repo.statusActual)
	}
}

func TestHandlePVSWebhook_InProcess(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	_, _ = svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	statusAntes := repo.statusActual
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 6})
	if repo.statusActual != statusAntes {
		t.Errorf("status cambio a %q, esperaba se mantenga %q", repo.statusActual, statusAntes)
	}
}

func TestHandlePVSWebhook_Idempotencia(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	_, _ = svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	if err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 5}); err != nil {
		t.Fatalf("primer webhook fallo: %v", err)
	}
	updatesPrimer := repo.updateLlamadas
	if err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 5}); err != nil {
		t.Fatalf("segundo webhook fallo: %v", err)
	}
	if repo.updateLlamadas != updatesPrimer {
		t.Errorf("segundo webhook genero update extra: %d vs %d", repo.updateLlamadas, updatesPrimer)
	}
}

func TestHandlePVSWebhook_QrIdVacio(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{})
	err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{StateID: 5})
	if err == nil {
		t.Fatal("se esperaba error por qrId vacio")
	}
}

func TestHandlePVSWebhook_StateIdInvalido(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	_, _ = svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 99})
	if err == nil {
		t.Fatal("se esperaba error por stateId invalido")
	}
}

func TestHandlePVSWebhook_QrInexistente(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{})
	err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr-no-existe", StateID: 5})
	if err == nil {
		t.Fatal("se esperaba error por qrId inexistente")
	}
}

// --- CompleteOrder ---

func TestCompleteOrder_HappyPath(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	created, _ := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 5})
	if err := svc.CompleteOrder(context.Background(), created.ThirdOrderNo); err != nil {
		t.Fatalf("CompleteOrder fallo: %v", err)
	}
	if repo.statusActual != domain.OrderDone {
		t.Errorf("status = %q, esperaba DONE", repo.statusActual)
	}
}

func TestCompleteOrder_TransicionInvalida(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	created, _ := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	err := svc.CompleteOrder(context.Background(), created.ThirdOrderNo)
	if err == nil {
		t.Fatal("se esperaba error: QR_SHOWN no puede ir a DONE")
	}
}

// --- CancelOrder ---

func TestCancelOrder_HappyPath(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	created, _ := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	if err := svc.CancelOrder(context.Background(), created.ThirdOrderNo); err != nil {
		t.Fatalf("CancelOrder fallo: %v", err)
	}
	if repo.statusActual != domain.OrderCancelled {
		t.Errorf("status = %q, esperaba CANCELLED", repo.statusActual)
	}
}

func TestCancelOrder_TransicionInvalida(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{})

	created, _ := svc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 5})
	_ = svc.CompleteOrder(context.Background(), created.ThirdOrderNo)
	err := svc.CancelOrder(context.Background(), created.ThirdOrderNo)
	if err == nil {
		t.Fatal("se esperaba error: DONE no puede ir a CANCELLED")
	}
}

// --- Helpers ---

func TestParseMonto(t *testing.T) {
	tests := []struct {
		input   string
		espera  int64
		wantErr bool
	}{
		{"150.00", 15000, false},
		{"150", 15000, false},
		{"150.5", 15050, false},
		{"150,50", 15050, false},
		{"0.01", 1, false},
		{"1000.99", 100099, false},
		{"abc", 0, true},
		{"", 0, true},
		{"-50.00", -5000, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseMonto(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("parseMonto(%q) esperaba error, fue nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parseMonto(%q) error inesperado: %v", tt.input, err)
			}
			if !tt.wantErr && got != tt.espera {
				t.Errorf("parseMonto(%q) = %d, esperaba %d", tt.input, got, tt.espera)
			}
		})
	}
}

func TestGenerarOrderNo(t *testing.T) {
	id1 := generarOrderNo()
	id2 := generarOrderNo()
	if id1 == id2 {
		t.Error("generarOrderNo genero IDs duplicados")
	}
	if len(id1) < 10 {
		t.Errorf("orderNo muy corto: %q", id1)
	}
}
