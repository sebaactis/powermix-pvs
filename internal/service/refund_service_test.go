package service

import (
	"context"
	"testing"

	"github.com/seba/vps-powermix/internal/domain"
)

// --- Mocks para RefundService ---

type mockRefundRepo struct {
	byRefundNo map[string]*domain.Refund
	created    *domain.Refund
	status     domain.RefundStatus
}

func (m *mockRefundRepo) Create(_ context.Context, r *domain.Refund) error {
	m.created = r
	if m.byRefundNo == nil {
		m.byRefundNo = make(map[string]*domain.Refund)
	}
	m.byRefundNo[r.RefundNo] = r
	m.status = r.Status
	return nil
}

func (m *mockRefundRepo) GetByRefundNo(_ context.Context, refundNo string) (*domain.Refund, error) {
	if m.byRefundNo != nil {
		if r, ok := m.byRefundNo[refundNo]; ok {
			r.Status = m.status
			return r, nil
		}
	}
	return nil, domain.ErrRefundNotFound
}

func (m *mockRefundRepo) UpdateStatus(_ context.Context, _ string, status domain.RefundStatus) error {
	m.status = status
	if m.created != nil {
		m.created.Status = status
	}
	return nil
}

// --- Tests ---

// TestRefund_HappyPath: orden PAYMENT_CONFIRMED, PVS.Reverse ok.
func TestRefund_HappyPath(t *testing.T) {
	orderRepo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	refundRepo := &mockRefundRepo{}
	syncLog := &mockSyncLogRepo{}

	orderSvc := NewOrderService(orderRepo, pvs, syncLog)
	refundSvc := NewRefundService(orderRepo, pvs, refundRepo, syncLog)

	// Crear orden y pagarla
	created, err := orderSvc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := orderSvc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID: "qr_test_123", StateID: 5,
	}); err != nil {
		t.Fatal(err)
	}

	// Solicitar reembolso
	resp, err := refundSvc.Refund(context.Background(), &RefundRequest{
		RefundNo:     "refund-001",
		ThirdOrderNo: created.ThirdOrderNo,
		Amount:       "100.00",
		Reason:       "Cliente se arrepintio",
	})
	if err != nil {
		t.Fatalf("Refund fallo: %v", err)
	}
	if resp.RefundStatus != "success" {
		t.Errorf("RefundStatus = %q, esperaba success", resp.RefundStatus)
	}

	// Orden debe estar en REFUND_PENDING
	if orderRepo.statusActual != domain.OrderRefundPending {
		t.Errorf("order status = %q, esperaba REFUND_PENDING", orderRepo.statusActual)
	}
	// Reembolso debe estar en SUCCESS
	if refundRepo.status != domain.RefundSuccess {
		t.Errorf("refund status = %q, esperaba SUCCESS", refundRepo.status)
	}
	if refundRepo.created == nil || refundRepo.created.OrderNo != created.ThirdOrderNo {
		t.Error("no se creo el reembolso correctamente")
	}
}

// TestRefund_DoneReembolsable: orden DONE tambien se puede reembolsar.
func TestRefund_DoneReembolsable(t *testing.T) {
	orderRepo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	refundRepo := &mockRefundRepo{}
	syncLog := &mockSyncLogRepo{}

	orderSvc := NewOrderService(orderRepo, pvs, syncLog)
	refundSvc := NewRefundService(orderRepo, pvs, refundRepo, syncLog)

	created, _ := orderSvc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	_ = orderSvc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID: "qr_test_123", StateID: 5,
	})
	_ = orderSvc.CompleteOrder(context.Background(), created.ThirdOrderNo)

	// Orden esta en DONE, debe poder reembolsarse
	_, err := refundSvc.Refund(context.Background(), &RefundRequest{
		RefundNo:     "refund-002",
		ThirdOrderNo: created.ThirdOrderNo,
		Amount:       "100.00",
	})
	if err != nil {
		t.Fatalf("Refund sobre DONE fallo: %v", err)
	}
	if orderRepo.statusActual != domain.OrderRefundPending {
		t.Errorf("order status = %q, esperaba REFUND_PENDING", orderRepo.statusActual)
	}
}

// TestRefund_PVSErrorDeRed: PVS.Reverse falla con error de red.
func TestRefund_PVSErrorDeRed(t *testing.T) {
	orderRepo := &mockOrderRepo{}
	pvs := &mockPVSClient{reverseErr: true}
	refundRepo := &mockRefundRepo{}
	syncLog := &mockSyncLogRepo{}

	orderSvc := NewOrderService(orderRepo, pvs, syncLog)
	refundSvc := NewRefundService(orderRepo, pvs, refundRepo, syncLog)

	created, _ := orderSvc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	_ = orderSvc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID: "qr_test_123", StateID: 5,
	})

	_, err := refundSvc.Refund(context.Background(), &RefundRequest{
		RefundNo: "refund-003", ThirdOrderNo: created.ThirdOrderNo, Amount: "100.00",
	})
	if err == nil {
		t.Fatal("se esperaba error de PVS Reverse")
	}
	if orderRepo.statusActual != domain.OrderRefundFailed {
		t.Errorf("order status = %q, esperaba REFUND_FAILED", orderRepo.statusActual)
	}
	if refundRepo.status != domain.RefundFailed {
		t.Errorf("refund status = %q, esperaba FAILED", refundRepo.status)
	}
}

// TestRefund_PVSRechaza: PVS.Reverse devuelve Success=false.
func TestRefund_PVSRechaza(t *testing.T) {
	orderRepo := &mockOrderRepo{}
	pvs := &mockPVSClient{reverseFail: true}
	refundRepo := &mockRefundRepo{}
	syncLog := &mockSyncLogRepo{}

	orderSvc := NewOrderService(orderRepo, pvs, syncLog)
	refundSvc := NewRefundService(orderRepo, pvs, refundRepo, syncLog)

	created, _ := orderSvc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	_ = orderSvc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID: "qr_test_123", StateID: 5,
	})

	resp, err := refundSvc.Refund(context.Background(), &RefundRequest{
		RefundNo: "refund-004", ThirdOrderNo: created.ThirdOrderNo, Amount: "100.00",
	})
	if err != nil {
		t.Fatalf("PVS rechazo no deberia ser error de sistema: %v", err)
	}
	if resp.RefundStatus != "fail" {
		t.Errorf("RefundStatus = %q, esperaba fail", resp.RefundStatus)
	}
	if orderRepo.statusActual != domain.OrderRefundFailed {
		t.Errorf("order status = %q, esperaba REFUND_FAILED", orderRepo.statusActual)
	}
	if refundRepo.status != domain.RefundFailed {
		t.Errorf("refund status = %q, esperaba FAILED", refundRepo.status)
	}
}

// TestRefund_Idempotencia: mismo refundNo dos veces devuelve el mismo resultado.
func TestRefund_Idempotencia(t *testing.T) {
	orderRepo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	refundRepo := &mockRefundRepo{}
	syncLog := &mockSyncLogRepo{}

	orderSvc := NewOrderService(orderRepo, pvs, syncLog)
	refundSvc := NewRefundService(orderRepo, pvs, refundRepo, syncLog)

	created, _ := orderSvc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	_ = orderSvc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID: "qr_test_123", StateID: 5,
	})

	// Primer llamado
	resp1, err := refundSvc.Refund(context.Background(), &RefundRequest{
		RefundNo: "refund-005", ThirdOrderNo: created.ThirdOrderNo, Amount: "100.00",
	})
	if err != nil {
		t.Fatal(err)
	}
	llamadasPvs := pvs.llamadas

	// Segundo llamado con mismo refundNo
	resp2, err := refundSvc.Refund(context.Background(), &RefundRequest{
		RefundNo: "refund-005", ThirdOrderNo: created.ThirdOrderNo, Amount: "100.00",
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp1.RefundStatus != resp2.RefundStatus {
		t.Errorf("status difiere: %q vs %q", resp1.RefundStatus, resp2.RefundStatus)
	}
	if pvs.llamadas != llamadasPvs {
		t.Error("PVS.Reverse fue llamado en el segundo Refund (no deberia)")
	}
}

// TestRefund_OrdenNoReembolsable: orden en QR_SHOWN no se puede reembolsar.
func TestRefund_OrdenNoReembolsable(t *testing.T) {
	orderRepo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	refundRepo := &mockRefundRepo{}
	syncLog := &mockSyncLogRepo{}

	orderSvc := NewOrderService(orderRepo, pvs, syncLog)
	refundSvc := NewRefundService(orderRepo, pvs, refundRepo, syncLog)

	created, _ := orderSvc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	// Orden en QR_SHOWN, no se puede reembolsar

	_, err := refundSvc.Refund(context.Background(), &RefundRequest{
		RefundNo: "refund-006", ThirdOrderNo: created.ThirdOrderNo, Amount: "100.00",
	})
	if err != domain.ErrOrderNotRefundable {
		t.Errorf("error = %v, esperaba ErrOrderNotRefundable", err)
	}
}

// TestRefund_ValidacionRefundNoVacio: sin refundNo, debe fallar.
func TestRefund_ValidacionRefundNoVacio(t *testing.T) {
	svc := NewRefundService(&mockOrderRepo{}, &mockPVSClient{}, &mockRefundRepo{}, &mockSyncLogRepo{})
	_, err := svc.Refund(context.Background(), &RefundRequest{
		ThirdOrderNo: "ord-123", Amount: "100.00",
	})
	if err == nil {
		t.Fatal("se esperaba error por refundNo vacio")
	}
}

// TestRefund_ValidacionThirdOrderNoVacio: sin thirdOrderNo, debe fallar.
func TestRefund_ValidacionThirdOrderNoVacio(t *testing.T) {
	svc := NewRefundService(&mockOrderRepo{}, &mockPVSClient{}, &mockRefundRepo{}, &mockSyncLogRepo{})
	_, err := svc.Refund(context.Background(), &RefundRequest{
		RefundNo: "refund-007", Amount: "100.00",
	})
	if err == nil {
		t.Fatal("se esperaba error por thirdOrderNo vacio")
	}
}

// TestRefund_OrdenInexistente: orden que no existe, debe fallar.
func TestRefund_OrdenInexistente(t *testing.T) {
	svc := NewRefundService(&mockOrderRepo{}, &mockPVSClient{}, &mockRefundRepo{}, &mockSyncLogRepo{})
	_, err := svc.Refund(context.Background(), &RefundRequest{
		RefundNo: "refund-008", ThirdOrderNo: "no-existe", Amount: "100.00",
	})
	if err == nil {
		t.Fatal("se esperaba error por orden inexistente")
	}
}

// TestRefund_WebhookConfirmacion: PVS webhook con stateId=4 confirma el
// reembolso y mueve REFUND_PENDING -> REFUNDED.
func TestRefund_WebhookConfirmacion(t *testing.T) {
	orderRepo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	refundRepo := &mockRefundRepo{}
	syncLog := &mockSyncLogRepo{}

	orderSvc := NewOrderService(orderRepo, pvs, syncLog)

	created, _ := orderSvc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	_ = orderSvc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID: "qr_test_123", StateID: 5,
	})

	// Hacer Refund -> REFUND_PENDING
	refundSvc := NewRefundService(orderRepo, pvs, refundRepo, syncLog)
	_, _ = refundSvc.Refund(context.Background(), &RefundRequest{
		RefundNo: "refund-009", ThirdOrderNo: created.ThirdOrderNo, Amount: "100.00",
	})

	if orderRepo.statusActual != domain.OrderRefundPending {
		t.Fatalf("orden debia estar REFUND_PENDING antes del webhook, estaba %q",
			orderRepo.statusActual)
	}

	// Llega el webhook de PVS confirmando el reverse (stateId=4)
	// REFUND_PENDING -> REFUNDED
	err := orderSvc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID: "qr_test_123", StateID: 4,
	})
	if err != nil {
		t.Fatalf("webhook confirmacion fallo: %v", err)
	}

	if orderRepo.statusActual != domain.OrderRefunded {
		t.Errorf("order status = %q, esperaba REFUNDED (confirmacion webhook)",
			orderRepo.statusActual)
	}
}

// TestRefund_WebhookReverseEspontaneo: stateId=4 sin refund pendiente
// mueve PAYMENT_CONFIRMED -> REFUND_PENDING (reversa espontanea de PVS).
func TestRefund_WebhookReverseEspontaneo(t *testing.T) {
	orderRepo := &mockOrderRepo{}
	pvs := &mockPVSClient{}
	syncLog := &mockSyncLogRepo{}

	orderSvc := NewOrderService(orderRepo, pvs, syncLog)

	// No nos interesa el orderNo aca, solo el flujo
	_, _ = orderSvc.CreateOrder(context.Background(), &CreateOrderRequest{
		ObjectID: "drink-test", TotalAmount: "100.00", DeviceID: "dev-1",
	})
	_ = orderSvc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID: "qr_test_123", StateID: 5,
	})

	// Llega stateId=4 sin que pidamos reembolso
	// PAYMENT_CONFIRMED -> REFUND_PENDING
	err := orderSvc.HandlePVSWebhook(context.Background(), &PVSWebhookRequest{
		QrID: "qr_test_123", StateID: 4,
	})
	if err != nil {
		t.Fatalf("webhook reversa espontanea fallo: %v", err)
	}
	if orderRepo.statusActual != domain.OrderRefundPending {
		t.Errorf("order status = %q, esperaba REFUND_PENDING (reversa espontanea)",
			orderRepo.statusActual)
	}
}
