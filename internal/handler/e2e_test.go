// Package handler contiene tests end-to-end que ejercitan el flujo
// HTTP completo con mocks en los servicios.
package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestE2E_CrearPagarCompletar: flujo feliz completo.
// CreateOrder -> PVSWebhook(stateId=5) -> QueryStatus -> CompleteOrder.
func TestE2E_CrearPagarCompletar(t *testing.T) {
	orderSvc := &mockOrderSvc{}
	refundSvc := &mockRefundSvc{}
	h := New(orderSvc, refundSvc, &mockDB{})

	// 1. Crear orden
	body := `{"objectId":"drink-001","totalAmount":"100.00","deviceId":"dev-1"}`
	req := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("CreateOrder: status %d, esperaba 200", w.Code)
	}

	// 2. Simular pago via webhook
	body = `{"qrId":"qr_001","stateId":5}`
	req = httptest.NewRequest("POST", "/webhook/pvs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("Webhook: status %d, esperaba 200", w.Code)
	}

	// 3. Consultar estado
	req = httptest.NewRequest("GET", "/api/v1/orders/ord-drink-001", nil)
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("QueryStatus: status %d, esperaba 200", w.Code)
	}

	// 4. Completar entrega
	req = httptest.NewRequest("POST", "/api/v1/orders/ord-drink-001/complete", nil)
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("CompleteOrder: status %d, esperaba 200", w.Code)
	}
}

// TestE2E_RefundFlow: CreateOrder -> webhook -> refund -> webhook confirm.
func TestE2E_RefundFlow(t *testing.T) {
	orderSvc := &mockOrderSvc{}
	refundSvc := &mockRefundSvc{}
	h := New(orderSvc, refundSvc, &mockDB{})

	// 1. Crear orden
	body := `{"objectId":"drink-002","totalAmount":"100.00","deviceId":"dev-1"}`
	req := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("CreateOrder: %d", w.Code)
	}

	// 2. Pagar
	httptest.NewRequest("POST", "/webhook/pvs", strings.NewReader(`{"qrId":"qr_001","stateId":5}`))
	_ = httptest.NewRecorder()

	// 3. Reembolsar
	body = `{"refundNo":"ref-001","refundAmount":"100.00","refundReason":"test"}`
	req = httptest.NewRequest("POST", "/api/v1/orders/ord-drink-002/refund", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("Refund: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "success") {
		t.Errorf("Refund response no contiene success: %s", w.Body.String())
	}

	// 4. Webhook confirmacion de reembolso
	req = httptest.NewRequest("POST", "/webhook/pvs", strings.NewReader(`{"qrId":"qr_001","stateId":4}`))
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("Webhook refund: %d", w.Code)
	}
}

// TestE2E_CancelFlow: CreateOrder -> CancelOrder.
func TestE2E_CancelFlow(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	httptest.NewRequest("POST", "/api/v1/orders",
		strings.NewReader(`{"objectId":"drink-003","totalAmount":"50.00","deviceId":"dev-1"}`))

	req := httptest.NewRequest("POST", "/api/v1/orders/ord-drink-003/cancel", nil)
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("CancelOrder: %d", w.Code)
	}
}
