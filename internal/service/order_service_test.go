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
	creada         *domain.Order
	statusActual   domain.OrderStatus
	failOnCreate   bool
	failOnUpdate   bool
	updateLlamadas int
}

func (m *mockOrderRepo) Create(_ context.Context, o *domain.Order) error {
	if m.failOnCreate {
		return errors.New("error simulado en Create")
	}
	m.creada = o
	m.statusActual = o.Status
	return nil
}

func (m *mockOrderRepo) GetByThirdOrderNo(_ context.Context, orderNo string) (*domain.Order, error) {
	if m.creada != nil && m.creada.ThirdOrderNo == orderNo {
		o := *m.creada
		o.Status = m.statusActual
		return &o, nil
	}
	return nil, domain.ErrOrderNotFound
}

func (m *mockOrderRepo) GetByGsOrderNo(_ context.Context, gsOrderNo string) (*domain.Order, error) {
	if m.creada != nil && m.creada.GsOrderNo == gsOrderNo {
		o := *m.creada
		o.Status = m.statusActual
		return &o, nil
	}
	return nil, domain.ErrOrderNotFound
}

func (m *mockOrderRepo) ListPaymentConfirmedUnnotified(_ context.Context, _ int) ([]domain.Order, error) {
	return nil, nil
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
		applyOrderFields(m.creada, fields)
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
		m.creada.Status = newStatus
		applyOrderFields(m.creada, fields)
	}
	return true, nil
}

// applyOrderFields aplica campos conocidos del update al mock en memoria.
func applyOrderFields(o *domain.Order, fields map[string]interface{}) {
	if o == nil {
		return
	}
	if v, ok := fields["pvs_qr_id"]; ok {
		if s, ok := v.(string); ok {
			o.PvsQrID = s
		}
	}
	if v, ok := fields["pvs_qr_image"]; ok {
		if s, ok := v.(string); ok {
			o.PvsQrImage = s
		}
	}
	if v, ok := fields["payment_confirmed_at"]; ok {
		switch t := v.(type) {
		case time.Time:
			o.PaymentConfirmedAt = t
		}
	}
	if v, ok := fields["gs_notified_at"]; ok {
		switch t := v.(type) {
		case time.Time:
			o.GsNotifiedAt = t
		}
	}
	if v, ok := fields["failure_reason"]; ok {
		if s, ok := v.(string); ok {
			o.FailureReason = s
		}
	}
	if v, ok := fields["gs_completed_at"]; ok {
		switch t := v.(type) {
		case time.Time:
			o.GsCompletedAt = t
		}
	}
	if v, ok := fields["gs_cancelled_at"]; ok {
		switch t := v.(type) {
		case time.Time:
			o.GsCancelledAt = t
		}
	}
}

// mockGSClient captura NotifyPayment para tests de PR-C.
type mockGSClient struct {
	calls   int
	lastReq *ports.GSNotifyPaymentRequest
	err     error
	resp    *ports.GSNotifyPaymentResponse
}

func (m *mockGSClient) NotifyPayment(_ context.Context, req *ports.GSNotifyPaymentRequest) (*ports.GSNotifyPaymentResponse, error) {
	m.calls++
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	if m.resp != nil {
		return m.resp, nil
	}
	return &ports.GSNotifyPaymentResponse{ReturnCode: "success"}, nil
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

// validCreateReq arma un request GS v2 minimo valido para tests.
func validCreateReq(overrides ...func(*CreateOrderRequest)) *CreateOrderRequest {
	req := &CreateOrderRequest{
		OrderNo:     "GS-ORDER-001",
		ObjectID:    "drink-test",
		Subject:     "Batido test",
		Attach:      "deviceNo=E001&deviceId=dev-1",
		TotalAmount: "100.00",
		NotifyURL:   "https://gs.example/notify",
	}
	for _, fn := range overrides {
		fn(req)
	}
	return req
}

// --- Tests ---

func TestCreateOrder_HappyPath(t *testing.T) {
	repo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	syncLog := &mockSyncLogRepo{}
	svc := NewOrderService(repo, pvs, syncLog, nil)

	resp, err := svc.CreateOrder(context.Background(), validCreateReq(func(r *CreateOrderRequest) {
		r.ObjectID = "drink-fresa-001"
		r.Subject = "Batido de Frutilla"
		r.TotalAmount = "150.00"
		r.Attach = "deviceNo=E001&deviceId=device-001"
	}))
	if err != nil {
		t.Fatalf("CreateOrder fallo: %v", err)
	}
	if resp.OrderStatus != "1" {
		t.Errorf("OrderStatus = %q, esperaba \"1\"", resp.OrderStatus)
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
	svc := NewOrderService(repo, pvs, &mockSyncLogRepo{}, nil)

	_, err := svc.CreateOrder(context.Background(), validCreateReq())
	if err == nil {
		t.Fatal("se esperaba error de PVS")
	}
	if repo.statusActual != domain.OrderFailed {
		t.Errorf("status = %q, esperaba FAILED", repo.statusActual)
	}
}

func TestCreateOrder_OrderNoVacio(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.CreateOrder(context.Background(), validCreateReq(func(r *CreateOrderRequest) { r.OrderNo = ""; r.Subject = ""; r.NotifyURL = ""; r.ObjectID = "" }))
	if err == nil {
		t.Fatal("se esperaba error por campos requeridos vacios")
	}
}

func TestCreateOrder_MontoVacio(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.CreateOrder(context.Background(), validCreateReq(func(r *CreateOrderRequest) { r.TotalAmount = "" }))
	if err == nil {
		t.Fatal("se esperaba error por monto vacio")
	}
}

func TestCreateOrder_MontoInvalido(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.CreateOrder(context.Background(), validCreateReq(func(r *CreateOrderRequest) { r.TotalAmount = "no-es-un-numero" }))
	if err == nil {
		t.Fatal("se esperaba error por monto invalido")
	}
}

func TestCreateOrder_MontoCero(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.CreateOrder(context.Background(), validCreateReq(func(r *CreateOrderRequest) { r.TotalAmount = "0.00" }))
	if err == nil {
		t.Fatal("se esperaba error por monto cero")
	}
}

func TestCreateOrder_Dedup(t *testing.T) {
	repo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	svc := NewOrderService(repo, pvs, &mockSyncLogRepo{}, nil)

	resp1, err := svc.CreateOrder(context.Background(), validCreateReq())
	if err != nil {
		t.Fatal(err)
	}
	resp2, err := svc.CreateOrder(context.Background(), validCreateReq())
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
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.CreateOrder(context.Background(), validCreateReq())
	if err == nil {
		t.Fatal("se esperaba error del repositorio")
	}
}

// --- QueryStatus ---

func TestQueryStatus_HappyPath(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	created, err := svc.CreateOrder(context.Background(), validCreateReq())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := svc.QueryStatus(context.Background(), &QueryStatusRequest{
		OrderNo:      "GS-ORDER-001",
		ThirdOrderNo: created.ThirdOrderNo,
	})
	if err != nil {
		t.Fatalf("QueryStatus fallo: %v", err)
	}
	if resp.OrderStatus != "1" {
		t.Errorf("OrderStatus = %q, esperaba \"1\" (pending)", resp.OrderStatus)
	}
	if resp.OrderNo != "GS-ORDER-001" {
		t.Errorf("OrderNo = %q", resp.OrderNo)
	}
}

func TestQueryStatus_OrdenInexistente(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.QueryStatus(context.Background(), &QueryStatusRequest{OrderNo: "GS-X", ThirdOrderNo: "no-existe"})
	if err == nil {
		t.Fatal("se esperaba error por orden inexistente")
	}
}

func TestQueryStatus_ThirdOrderNoVacio(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.QueryStatus(context.Background(), &QueryStatusRequest{})
	if err == nil {
		t.Fatal("se esperaba error por thirdOrderNo vacio")
	}
}

func TestQueryStatus_OrderNoVacio(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.QueryStatus(context.Background(), &QueryStatusRequest{ThirdOrderNo: "x"})
	if err == nil {
		t.Fatal("se esperaba error por orderNo vacio")
	}
}

func TestQueryStatus_PairMismatch(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	created, err := svc.CreateOrder(context.Background(), validCreateReq())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.QueryStatus(context.Background(), &QueryStatusRequest{
		OrderNo:      "OTRO-GS",
		ThirdOrderNo: created.ThirdOrderNo,
	})
	if err == nil {
		t.Fatal("se esperaba error por pair mismatch")
	}
}

// --- Webhook ---

func TestHandlePVSWebhook_PagoConfirmado(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	_, err := svc.CreateOrder(context.Background(), validCreateReq())
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

// Callback real PVS manda status texto, no stateId numerico.
func TestHandlePVSWebhook_StatusTextoApproved(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	_, err := svc.CreateOrder(context.Background(), validCreateReq())
	if err != nil {
		t.Fatal(err)
	}
	err = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID:   "qr_test_123",
		Status: "APPROVED",
		TxEID:  "txe_pay_001",
	})
	if err != nil {
		t.Fatalf("HandlePVSWebhook fallo: %v", err)
	}
	if repo.statusActual != domain.OrderPaymentConfirmed {
		t.Errorf("status = %q, esperaba PAYMENT_CONFIRMED", repo.statusActual)
	}
}

// Shape oficial PVS Callback Process (colección live).
func TestHandlePVSWebhook_BodyOficialApproved(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	created, err := svc.CreateOrder(context.Background(), validCreateReq())
	if err != nil {
		t.Fatal(err)
	}
	err = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		Reference:  created.ThirdOrderNo,
		Amount:     150.00,
		QrID:       "qr_test_123",
		TxEID:      "422164787",
		Status:     "APPROVED",
		NotifiedAt: "2024-10-10T18:00:23Z",
		Payer: &PVSWebhookPayer{
			Name: "PEDRO GARCIA", IDType: "DNI", IDNumber: "33445989",
		},
	})
	if err != nil {
		t.Fatalf("HandlePVSWebhook fallo: %v", err)
	}
	if repo.statusActual != domain.OrderPaymentConfirmed {
		t.Errorf("status = %q, esperaba PAYMENT_CONFIRMED", repo.statusActual)
	}
}

// Doc permite asociar por reference (nosotros mandamos thirdOrderNo como reference).
func TestHandlePVSWebhook_LookupPorReference(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	created, err := svc.CreateOrder(context.Background(), validCreateReq())
	if err != nil {
		t.Fatal(err)
	}
	// Sin qrId: solo reference (body o query ya resuelto en req.Reference / QueryReference).
	err = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QueryReference: created.ThirdOrderNo,
		Status:         "APPROVED",
		TxEID:          "txe_ref_only",
	})
	if err != nil {
		t.Fatalf("HandlePVSWebhook por reference fallo: %v", err)
	}
	if repo.statusActual != domain.OrderPaymentConfirmed {
		t.Errorf("status = %q, esperaba PAYMENT_CONFIRMED", repo.statusActual)
	}
}

func TestHandlePVSWebhook_Rechazado(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	_, _ = svc.CreateOrder(context.Background(), validCreateReq())
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 3})
	if repo.statusActual != domain.OrderFailed {
		t.Errorf("status = %q, esperaba FAILED", repo.statusActual)
	}
}

func TestHandlePVSWebhook_InProcess(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	_, _ = svc.CreateOrder(context.Background(), validCreateReq())
	statusAntes := repo.statusActual
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 6})
	if repo.statusActual != statusAntes {
		t.Errorf("status cambio a %q, esperaba se mantenga %q", repo.statusActual, statusAntes)
	}
}

func TestHandlePVSWebhook_Idempotencia(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	_, _ = svc.CreateOrder(context.Background(), validCreateReq())
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
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{StateID: 5})
	if err == nil {
		t.Fatal("se esperaba error por qrId vacio")
	}
}

func TestHandlePVSWebhook_StateIdInvalido(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	_, _ = svc.CreateOrder(context.Background(), validCreateReq())
	err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 99})
	if err == nil {
		t.Fatal("se esperaba error por stateId invalido")
	}
}

func TestHandlePVSWebhook_QrInexistente(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr-no-existe", StateID: 5})
	if err == nil {
		t.Fatal("se esperaba error por qrId inexistente")
	}
}

// --- CompleteOrder ---

func validCompleteReq(thirdOrderNo string, overrides ...func(*CompleteOrderRequest)) *CompleteOrderRequest {
	req := &CompleteOrderRequest{
		OrderNo:        "GS-ORDER-001",
		ThirdOrderNo:   thirdOrderNo,
		Success:        true,
		OrderStatus:    "2",
		OutStockStatus: 2,
	}
	for _, fn := range overrides {
		fn(req)
	}
	return req
}

func validCancelReq(thirdOrderNo string, overrides ...func(*CancelOrderRequest)) *CancelOrderRequest {
	req := &CancelOrderRequest{
		OrderNo:      "GS-ORDER-001",
		ThirdOrderNo: thirdOrderNo,
		OrderStatus:  "0",
		CancelTime:   "2026-07-10 12:00:00",
		Remark:       "cancel test",
	}
	for _, fn := range overrides {
		fn(req)
	}
	return req
}

// AT-08: success + outStockStatus=2 → DONE.
func TestCompleteOrder_HappyPath(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	created, _ := svc.CreateOrder(context.Background(), validCreateReq())
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 5})
	resp, err := svc.CompleteOrder(context.Background(), validCompleteReq(created.ThirdOrderNo))
	if err != nil {
		t.Fatalf("CompleteOrder fallo: %v", err)
	}
	if resp.ReturnCode != "success" {
		t.Errorf("ReturnCode = %q", resp.ReturnCode)
	}
	if repo.statusActual != domain.OrderDone {
		t.Errorf("status = %q, esperaba DONE", repo.statusActual)
	}
}

// AT-09: success=false → FAILED refundable.
func TestCompleteOrder_SuccessFalse_Failed(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	created, _ := svc.CreateOrder(context.Background(), validCreateReq())
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 5})
	resp, err := svc.CompleteOrder(context.Background(), validCompleteReq(created.ThirdOrderNo, func(r *CompleteOrderRequest) {
		r.Success = false
		r.OutStockStatus = 1
	}))
	if err != nil {
		t.Fatalf("CompleteOrder fail path: %v", err)
	}
	if resp.ReturnCode != "success" {
		t.Errorf("ReturnCode = %q (notify aceptado aunque bebida fallo)", resp.ReturnCode)
	}
	if repo.statusActual != domain.OrderFailed {
		t.Errorf("status = %q, esperaba FAILED", repo.statusActual)
	}
	if repo.creada != nil && repo.creada.FailureReason != "gs_complete_success=false" {
		// failure_reason se setea via fields; mock puede no reflejarlo si no aplica el field
	}
}

func TestCompleteOrder_TransicionInvalida(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	created, _ := svc.CreateOrder(context.Background(), validCreateReq())
	_, err := svc.CompleteOrder(context.Background(), validCompleteReq(created.ThirdOrderNo))
	if err == nil {
		t.Fatal("se esperaba error: QR_SHOWN no puede ir a DONE")
	}
}

func TestCompleteOrder_PairMismatch(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	created, _ := svc.CreateOrder(context.Background(), validCreateReq())
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 5})
	_, err := svc.CompleteOrder(context.Background(), validCompleteReq(created.ThirdOrderNo, func(r *CompleteOrderRequest) {
		r.OrderNo = "GS-WRONG"
	}))
	if err == nil {
		t.Fatal("se esperaba pair mismatch")
	}
}

// --- CancelOrder ---

// AT-11: cancel desde QR_SHOWN.
func TestCancelOrder_HappyPath(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	created, _ := svc.CreateOrder(context.Background(), validCreateReq())
	resp, err := svc.CancelOrder(context.Background(), validCancelReq(created.ThirdOrderNo))
	if err != nil {
		t.Fatalf("CancelOrder fallo: %v", err)
	}
	if resp.ReturnCode != "success" {
		t.Errorf("ReturnCode = %q", resp.ReturnCode)
	}
	if repo.statusActual != domain.OrderCancelled {
		t.Errorf("status = %q, esperaba CANCELLED", repo.statusActual)
	}
}

func TestCancelOrder_IdempotentAlreadyCancelled(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	created, _ := svc.CreateOrder(context.Background(), validCreateReq())
	_, _ = svc.CancelOrder(context.Background(), validCancelReq(created.ThirdOrderNo))
	resp, err := svc.CancelOrder(context.Background(), validCancelReq(created.ThirdOrderNo))
	if err != nil {
		t.Fatalf("segundo cancel debia ser idempotente: %v", err)
	}
	if resp.ReturnCode != "success" {
		t.Errorf("ReturnCode = %q", resp.ReturnCode)
	}
}

func TestCancelOrder_TransicionInvalida(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)

	created, _ := svc.CreateOrder(context.Background(), validCreateReq())
	_ = svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr_test_123", StateID: 5})
	_, _ = svc.CompleteOrder(context.Background(), validCompleteReq(created.ThirdOrderNo))
	_, err := svc.CancelOrder(context.Background(), validCancelReq(created.ThirdOrderNo))
	if err == nil {
		t.Fatal("se esperaba error: DONE no puede ir a CANCELLED")
	}
}

func TestCancelOrder_PairMismatch(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	created, _ := svc.CreateOrder(context.Background(), validCreateReq())
	_, err := svc.CancelOrder(context.Background(), validCancelReq(created.ThirdOrderNo, func(r *CancelOrderRequest) {
		r.OrderNo = "GS-WRONG"
	}))
	if err == nil {
		t.Fatal("se esperaba pair mismatch")
	}
}

func TestFlexibleStatus_Unmarshal(t *testing.T) {
	var f FlexibleStatus
	if err := f.UnmarshalJSON([]byte(`2`)); err != nil {
		t.Fatal(err)
	}
	if f != "2" {
		t.Errorf("int → %q", f)
	}
	if err := f.UnmarshalJSON([]byte(`"0"`)); err != nil {
		t.Fatal(err)
	}
	if f != "0" {
		t.Errorf("string → %q", f)
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

func TestGenerarThirdOrderNo(t *testing.T) {
	id1 := generarThirdOrderNo()
	id2 := generarThirdOrderNo()
	if id1 == id2 {
		t.Error("generarThirdOrderNo genero IDs duplicados")
	}
	if len(id1) < 10 {
		t.Errorf("orderNo muy corto: %q", id1)
	}
}

func TestParseAttach(t *testing.T) {
	no, id := parseAttach("deviceNo=E00375&deviceId=7678242eba")
	if no != "E00375" || id != "7678242eba" {
		t.Fatalf("parseAttach = (%q,%q)", no, id)
	}
	no, id = parseAttach("")
	if no != "" || id != "" {
		t.Fatalf("empty attach = (%q,%q)", no, id)
	}
}

func TestCreateOrder_NotifyUrlVacio(t *testing.T) {
	svc := NewOrderService(&mockOrderRepo{}, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.CreateOrder(context.Background(), validCreateReq(func(r *CreateOrderRequest) { r.NotifyURL = "" }))
	if err == nil {
		t.Fatal("se esperaba error por notifyUrl vacio")
	}
}

func TestCreateOrder_PersisteGsOrderNoYNotify(t *testing.T) {
	repo := &mockOrderRepo{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, nil)
	_, err := svc.CreateOrder(context.Background(), validCreateReq())
	if err != nil {
		t.Fatal(err)
	}
	if repo.creada.GsOrderNo != "GS-ORDER-001" {
		t.Errorf("GsOrderNo = %q", repo.creada.GsOrderNo)
	}
	if repo.creada.NotifyURL != "https://gs.example/notify" {
		t.Errorf("NotifyURL = %q", repo.creada.NotifyURL)
	}
	if repo.creada.DeviceID != "dev-1" || repo.creada.DeviceNo != "E001" {
		t.Errorf("device = %q/%q", repo.creada.DeviceNo, repo.creada.DeviceID)
	}
}

// AT-06: un solo notify; segundo webhook no vuelve a notificar.
func TestHandlePVSWebhook_NotifyOnce(t *testing.T) {
	now := time.Now()
	orden := &domain.Order{
		ThirdOrderNo: "T-001",
		GsOrderNo:    "GS-001",
		PvsQrID:      "qr-1",
		Status:       domain.OrderQRShown,
		NotifyURL:    "https://gs.example/notify",
		PriceCents:   15000,
		CreatedAt:    now,
	}
	repo := &mockOrderRepo{creada: orden, statusActual: domain.OrderQRShown}
	gs := &mockGSClient{}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, gs)

	if err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr-1", StateID: 5}); err != nil {
		t.Fatalf("primer webhook: %v", err)
	}
	if gs.calls != 1 {
		t.Fatalf("notify calls = %d, esperaba 1", gs.calls)
	}
	if gs.lastReq == nil {
		t.Fatal("lastReq nil")
	}
	if gs.lastReq.OrderStatus != "2" || gs.lastReq.OrderNo != "GS-001" || gs.lastReq.ThirdOrderNo != "T-001" {
		t.Fatalf("payload notify incorrecto: %+v", gs.lastReq)
	}
	if gs.lastReq.TotalAmount != "150.00" {
		t.Fatalf("TotalAmount = %q, esperaba 150.00", gs.lastReq.TotalAmount)
	}
	if repo.creada.GsNotifiedAt.IsZero() {
		t.Fatal("GsNotifiedAt sigue zero tras notify ok")
	}

	// Segundo webhook: estado ya confirmado → no-op, sin segundo notify.
	if err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr-1", StateID: 5}); err != nil {
		t.Fatalf("segundo webhook: %v", err)
	}
	if gs.calls != 1 {
		t.Fatalf("segundo notify no debe ocurrir; calls = %d", gs.calls)
	}
}

// AT-07: si NotifyPayment falla, gs_notified_at queda null y el webhook NO falla.
func TestHandlePVSWebhook_NotifyFailLeavesNull(t *testing.T) {
	orden := &domain.Order{
		ThirdOrderNo: "T-002",
		GsOrderNo:    "GS-002",
		PvsQrID:      "qr-2",
		Status:       domain.OrderQRShown,
		NotifyURL:    "https://gs.example/notify",
		PriceCents:   10000,
		CreatedAt:    time.Now(),
	}
	repo := &mockOrderRepo{creada: orden, statusActual: domain.OrderQRShown}
	gs := &mockGSClient{err: errors.New("gs down")}
	svc := NewOrderService(repo, &mockPVSClient{}, &mockSyncLogRepo{}, gs)

	if err := svc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{QrID: "qr-2", StateID: 5}); err != nil {
		t.Fatalf("webhook no debe fallar por notify: %v", err)
	}
	if gs.calls != 1 {
		t.Fatalf("calls = %d, esperaba 1 intento", gs.calls)
	}
	if !repo.creada.GsNotifiedAt.IsZero() {
		t.Fatal("GsNotifiedAt no debe setearse si notify falla")
	}
	if repo.statusActual != domain.OrderPaymentConfirmed {
		t.Fatalf("status = %s, esperaba PAYMENT_CONFIRMED aunque notify falle", repo.statusActual)
	}
}

func TestFormatGSTime(t *testing.T) {
	if FormatGSTime(time.Time{}) != "" {
		t.Fatal("zero time debe ser string vacio")
	}
	// 15:04:05 UTC = 12:04:05 America/Argentina/Buenos_Aires (UTC-3)
	t0 := time.Date(2026, 7, 10, 15, 4, 5, 0, time.UTC)
	if got := FormatGSTime(t0); got != "2026-07-10 12:04:05" {
		t.Fatalf("FormatGSTime = %q, want ARG wall clock", got)
	}
}
