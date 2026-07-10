package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seba/vps-powermix/internal/service"
)

// --- Mocks ---

type mockOrderSvc struct {
	createResp  *service.CreateOrderResponse
	createErr   error
	queryResp   *service.QueryStatusResponse
	queryErr    error
	completeErr error
	cancelErr   error
	webhookErr  error
}

func (m *mockOrderSvc) CreateOrder(_ context.Context, req *service.CreateOrderRequest) (*service.CreateOrderResponse, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.createResp != nil {
		return m.createResp, nil
	}
	return &service.CreateOrderResponse{
		QrURL:        "base64_qr",
		OrderStatus:  "1",
		ThirdOrderNo: "ord-" + req.ObjectID,
	}, nil
}

func (m *mockOrderSvc) QueryStatus(_ context.Context, req *service.QueryStatusRequest) (*service.QueryStatusResponse, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	if m.queryResp != nil {
		return m.queryResp, nil
	}
	return &service.QueryStatusResponse{OrderStatus: "1", ThirdOrderNo: req.ThirdOrderNo}, nil
}

func (m *mockOrderSvc) CompleteOrder(_ context.Context, req *service.CompleteOrderRequest) (*service.CompleteOrderResponse, error) {
	if m.completeErr != nil {
		return nil, m.completeErr
	}
	return &service.CompleteOrderResponse{
		OrderNo: req.OrderNo, ThirdOrderNo: req.ThirdOrderNo,
		ReturnCode: "success", ReturnMsg: "success",
	}, nil
}

func (m *mockOrderSvc) CancelOrder(_ context.Context, req *service.CancelOrderRequest) (*service.CancelOrderResponse, error) {
	if m.cancelErr != nil {
		return nil, m.cancelErr
	}
	return &service.CancelOrderResponse{
		OrderNo: req.OrderNo, ThirdOrderNo: req.ThirdOrderNo,
		ReturnCode: "success", ReturnMsg: req.Remark,
	}, nil
}

func (m *mockOrderSvc) HandlePVSWebhook(_ context.Context, _ *service.PVSWebhookRequest) error {
	return m.webhookErr
}

type mockRefundSvc struct {
	refundResp       *service.RefundResponse
	refundErr        error
	refundStatusResp *service.RefundStatusResponse
	refundStatusErr  error
}

func (m *mockRefundSvc) Refund(_ context.Context, _ *service.RefundRequest) (*service.RefundResponse, error) {
	if m.refundErr != nil {
		return nil, m.refundErr
	}
	if m.refundResp != nil {
		return m.refundResp, nil
	}
	return &service.RefundResponse{
		RefundNo: "ref-001", OrderNo: "GS-1", ThirdOrderNo: "ord-001",
		RefundStatus: "waiting", RefundMsg: "waiting",
	}, nil
}

func (m *mockRefundSvc) RefundStatus(_ context.Context, _ *service.RefundStatusRequest) (*service.RefundStatusResponse, error) {
	if m.refundStatusErr != nil {
		return nil, m.refundStatusErr
	}
	if m.refundStatusResp != nil {
		return m.refundStatusResp, nil
	}
	return &service.RefundStatusResponse{
		RefundNo: "ref-001", OrderNo: "GS-1", ThirdOrderNo: "ord-001",
		RefundStatus: "pending", RefundMsg: "pending",
	}, nil
}

type mockDB struct {
	pingErr error
}

func (m *mockDB) PingContext(_ context.Context) error {
	return m.pingErr
}

func decodeEnvelope(t *testing.T, body string) gsEnvelope {
	t.Helper()
	var env gsEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("envelope invalido: %v body=%s", err, body)
	}
	return env
}

// --- Tests de ruteo y respuesta ---

func TestCreateOrder_Handler(t *testing.T) {
	h := &Handler{orderSvc: &mockOrderSvc{}, db: &mockDB{}}

	body := `{"orderNo":"GS-1","subject":"Test","totalAmount":"100.00","notifyUrl":"https://gs.example/n","objectId":"drink-test","attach":"deviceId=dev-1"}`
	req := httptest.NewRequest("POST", "/order/qr", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Code != 200 {
		t.Errorf("code = %d, esperaba 200", env.Code)
	}
	raw, _ := json.Marshal(env.Data)
	if !strings.Contains(string(raw), "base64_qr") {
		t.Errorf("data no contiene qrUrl: %s", w.Body.String())
	}
}

func TestQueryStatus_Handler(t *testing.T) {
	h := &Handler{
		orderSvc: &mockOrderSvc{
			queryResp: &service.QueryStatusResponse{OrderStatus: "1", ThirdOrderNo: "ord-test"},
		},
		db: &mockDB{},
	}

	body := `{"orderNo":"GS-1","thirdOrderNo":"ord-test"}`
	req := httptest.NewRequest("POST", "/order/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Code != 200 {
		t.Errorf("code = %d", env.Code)
	}
	if !strings.Contains(w.Body.String(), "ord-test") {
		t.Errorf("body no contiene thirdOrderNo: %s", w.Body.String())
	}
}

func TestCompleteOrder_Handler(t *testing.T) {
	h := &Handler{orderSvc: &mockOrderSvc{}, db: &mockDB{}}
	body := `{"orderNo":"GS-1","thirdOrderNo":"ord-test","success":true,"orderStatus":2,"outStockStatus":2}`
	req := httptest.NewRequest("POST", "/order/complete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Code != 200 {
		t.Errorf("code = %d", env.Code)
	}
	if !strings.Contains(w.Body.String(), "returnCode") {
		t.Errorf("body sin returnCode: %s", w.Body.String())
	}
}

func TestCancelOrder_Handler(t *testing.T) {
	h := &Handler{orderSvc: &mockOrderSvc{}, db: &mockDB{}}
	body := `{"orderNo":"GS-1","thirdOrderNo":"ord-test","orderStatus":0,"cancelTime":"2026-07-10 12:00:00","remark":"user"}`
	req := httptest.NewRequest("POST", "/order/cancel", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
}

func TestRefund_Handler(t *testing.T) {
	h := &Handler{refundSvc: &mockRefundSvc{}, db: &mockDB{}}

	body := `{"refundNo":"ref-001","thirdOrderNo":"ord-test","refundAmount":"100.00","refundReason":"test"}`
	req := httptest.NewRequest("POST", "/order/refund", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "success") {
		t.Errorf("body no contiene success: %s", w.Body.String())
	}
}

func TestRefundStatus_Handler(t *testing.T) {
	h := &Handler{refundSvc: &mockRefundSvc{}, db: &mockDB{}}
	body := `{"orderNo":"GS-1","thirdOrderNo":"ord-001","refundNo":"ref-001"}`
	req := httptest.NewRequest("POST", "/order/refundStatus", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "pending") {
		t.Errorf("body no contiene pending: %s", w.Body.String())
	}
}

func TestLegacyAPI_NotFound(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})
	req := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("legacy path status = %d, esperaba 404", w.Code)
	}
}

func TestPVSWebhook_Handler(t *testing.T) {
	h := &Handler{orderSvc: &mockOrderSvc{}, db: &mockDB{}}

	body := `{"qrId":"qr_test","stateId":5}`
	req := httptest.NewRequest("POST", "/webhook/pvs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
}

func TestHealthz_Healthy(t *testing.T) {
	h := &Handler{db: &mockDB{}}
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("body no contiene ok: %s", w.Body.String())
	}
}

func TestHealthz_Unhealthy(t *testing.T) {
	h := &Handler{db: &mockDB{pingErr: assertAnError{}}}
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, esperaba 503", w.Code)
	}
}

func TestCreateOrder_JSONInvalido(t *testing.T) {
	h := &Handler{orderSvc: &mockOrderSvc{}, db: &mockDB{}}

	req := httptest.NewRequest("POST", "/order/qr", strings.NewReader("{malformed"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperaba 400", w.Code)
	}
	env := decodeEnvelope(t, w.Body.String())
	if env.Code != 400 {
		t.Errorf("code = %d, esperaba 400", env.Code)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})
	wrapped := recoveryMiddleware(panicHandler)

	req := httptest.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, esperaba 500", w.Code)
	}
}

type assertAnError struct{}

func (assertAnError) Error() string { return "error simulado" }

var _ OrderService = (*mockOrderSvc)(nil)
var _ RefundService = (*mockRefundSvc)(nil)
