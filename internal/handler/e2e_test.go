// Package handler contiene tests end-to-end que ejercitan el flujo
// HTTP completo con mocks en los servicios.
package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestE2E_CrearPagarCompletar: flujo feliz GS v2.
// POST /order/qr -> webhook PVS -> /order/status -> /order/complete
func TestE2E_CrearPagarCompletar(t *testing.T) {
	orderSvc := &mockOrderSvc{}
	refundSvc := &mockRefundSvc{}
	h := New(orderSvc, refundSvc, &mockDB{})

	// 1. Crear orden (GS Open API v2)
	body := `{"orderNo":"GS-001","subject":"Drink","totalAmount":"100.00","notifyUrl":"https://gs.example/n","objectId":"drink-001","attach":"deviceId=dev-1"}`
	req := httptest.NewRequest("POST", "/order/qr", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("CreateOrder: status %d, esperaba 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":200`) {
		t.Fatalf("CreateOrder sin envelope code 200: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "thirdOrderNo") {
		t.Fatalf("CreateOrder sin thirdOrderNo: %s", w.Body.String())
	}

	// 2. Simular pago via webhook PVS
	body = `{"qrId":"qr_001","stateId":5}`
	req = httptest.NewRequest("POST", "/webhook/pvs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("Webhook: status %d, esperaba 200", w.Code)
	}

	// 3. Consultar estado
	body = `{"orderNo":"GS-001","thirdOrderNo":"ord-drink-001"}`
	req = httptest.NewRequest("POST", "/order/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("QueryStatus: status %d, esperaba 200 body=%s", w.Code, w.Body.String())
	}

	// 4. Completar entrega (success + outStockStatus=2)
	body = `{"orderNo":"GS-001","thirdOrderNo":"ord-drink-001","success":true,"orderStatus":2,"outStockStatus":2,"outStockTime":"2026-07-10 12:00:00"}`
	req = httptest.NewRequest("POST", "/order/complete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("CompleteOrder: status %d, esperaba 200 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "returnCode") {
		t.Fatalf("CompleteOrder sin returnCode: %s", w.Body.String())
	}
}

// TestE2E_CancelFlow: CreateOrder -> CancelOrder (v2 body).
func TestE2E_CancelFlow(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	body := `{"orderNo":"GS-003","subject":"Drink","totalAmount":"100.00","notifyUrl":"https://gs.example/n","objectId":"drink-003","attach":"deviceId=dev-1"}`
	req := httptest.NewRequest("POST", "/order/qr", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("CreateOrder: %d", w.Code)
	}

	body = `{"orderNo":"GS-003","thirdOrderNo":"ord-drink-003","orderStatus":0,"cancelTime":"2026-07-10 12:00:00","remark":"user cancel"}`
	req = httptest.NewRequest("POST", "/order/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("CancelOrder: %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "returnCode") {
		t.Fatalf("CancelOrder sin returnCode: %s", w.Body.String())
	}
}

// TestE2E_CompleteFailThenRefund: pay -> complete fail -> refund -> refundStatus.
func TestE2E_CompleteFailThenRefund(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	body := `{"orderNo":"GS-004","subject":"Drink","totalAmount":"100.00","notifyUrl":"https://gs.example/n","objectId":"drink-004","attach":"deviceId=dev-1"}`
	req := httptest.NewRequest("POST", "/order/qr", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("CreateOrder: %d", w.Code)
	}

	req = httptest.NewRequest("POST", "/webhook/pvs", strings.NewReader(`{"qrId":"qr_001","stateId":5}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("Webhook pay: %d", w.Code)
	}

	// complete success=false → FAILED
	body = `{"orderNo":"GS-004","thirdOrderNo":"ord-drink-004","success":false,"orderStatus":2,"outStockStatus":1}`
	req = httptest.NewRequest("POST", "/order/complete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("Complete fail: %d body=%s", w.Code, w.Body.String())
	}

	// refund v2
	body = `{"orderNo":"GS-004","refundNo":"ref-004","thirdOrderNo":"ord-drink-004","refundAmount":"100.00","refundReason":"out of stock"}`
	req = httptest.NewRequest("POST", "/order/refund", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("Refund: %d body=%s", w.Code, w.Body.String())
	}
	// mock devuelve waiting (async reverse)
	if !strings.Contains(w.Body.String(), "waiting") {
		t.Errorf("Refund response debia contener waiting: %s", w.Body.String())
	}

	// refundStatus
	body = `{"orderNo":"GS-004","thirdOrderNo":"ord-drink-004","refundNo":"ref-004"}`
	req = httptest.NewRequest("POST", "/order/refundStatus", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("RefundStatus: %d body=%s", w.Code, w.Body.String())
	}
}

// TestE2E_RefundFlow: CreateOrder -> refund (waiting) -> webhook reverse.
func TestE2E_RefundFlow(t *testing.T) {
	orderSvc := &mockOrderSvc{}
	refundSvc := &mockRefundSvc{}
	h := New(orderSvc, refundSvc, &mockDB{})

	body := `{"orderNo":"GS-002","subject":"Drink","totalAmount":"100.00","notifyUrl":"https://gs.example/n","objectId":"drink-002","attach":"deviceId=dev-1"}`
	req := httptest.NewRequest("POST", "/order/qr", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("CreateOrder: %d", w.Code)
	}

	body = `{"orderNo":"GS-002","refundNo":"ref-001","thirdOrderNo":"ord-drink-002","refundAmount":"100.00","refundReason":"test"}`
	req = httptest.NewRequest("POST", "/order/refund", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("Refund: %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "waiting") {
		t.Errorf("Refund response no contiene waiting: %s", w.Body.String())
	}

	req = httptest.NewRequest("POST", "/webhook/pvs", strings.NewReader(`{"qrId":"qr_001","stateId":4}`))
	w = httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("Webhook refund: %d", w.Code)
	}
}
