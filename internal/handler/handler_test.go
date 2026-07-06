package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seba/vps-powermix/internal/service"
)

// --- Mocks ---

type mockOrderSvc struct {
	createResp      *service.CreateOrderResponse
	createErr       error
	queryResp       *service.QueryStatusResponse
	queryErr        error
	completeErr     error
	cancelErr       error
	webhookErr      error
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
		OrderStatus:  1,
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
	return &service.QueryStatusResponse{OrderStatus: 1, ThirdOrderNo: req.ThirdOrderNo}, nil
}

func (m *mockOrderSvc) CompleteOrder(_ context.Context, _ string) error {
	return m.completeErr
}

func (m *mockOrderSvc) CancelOrder(_ context.Context, _ string) error {
	return m.cancelErr
}

func (m *mockOrderSvc) HandlePVSWebhook(_ context.Context, _ *service.PVSWebhookRequest) error {
	return m.webhookErr
}

type mockRefundSvc struct {
	refundResp *service.RefundResponse
	refundErr  error
}

func (m *mockRefundSvc) Refund(_ context.Context, _ *service.RefundRequest) (*service.RefundResponse, error) {
	if m.refundErr != nil {
		return nil, m.refundErr
	}
	if m.refundResp != nil {
		return m.refundResp, nil
	}
	return &service.RefundResponse{RefundNo: "ref-001", ThirdOrderNo: "ord-001", RefundStatus: "success"}, nil
}

type mockDB struct {
	pingErr error
}

func (m *mockDB) PingContext(_ context.Context) error {
	return m.pingErr
}

// --- Tests de ruteo y respuesta ---

func TestCreateOrder_Handler(t *testing.T) {
	h := &Handler{
		orderSvc: &mockOrderSvc{},
		db:       &mockDB{},
	}

	body := `{"objectId":"drink-test","totalAmount":"100.00","deviceId":"dev-1"}`
	req := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "base64_qr") {
		t.Errorf("body no contiene qrUrl: %s", w.Body.String())
	}
}

func TestQueryStatus_Handler(t *testing.T) {
	h := &Handler{
		orderSvc: &mockOrderSvc{
			queryResp: &service.QueryStatusResponse{OrderStatus: 1, ThirdOrderNo: "ord-test"},
		},
		db: &mockDB{},
	}

	req := httptest.NewRequest("GET", "/api/v1/orders/ord-test", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ord-test") {
		t.Errorf("body no contiene orderNo: %s", w.Body.String())
	}
}

func TestCompleteOrder_Handler(t *testing.T) {
	h := &Handler{
		orderSvc: &mockOrderSvc{},
		db:       &mockDB{},
	}
	req := httptest.NewRequest("POST", "/api/v1/orders/ord-test/complete", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
}

func TestCancelOrder_Handler(t *testing.T) {
	h := &Handler{
		orderSvc: &mockOrderSvc{},
		db:       &mockDB{},
	}
	req := httptest.NewRequest("POST", "/api/v1/orders/ord-test/cancel", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperaba 200", w.Code)
	}
}

func TestRefund_Handler(t *testing.T) {
	h := &Handler{
		refundSvc: &mockRefundSvc{},
		db:        &mockDB{},
	}

	body := `{"refundNo":"ref-001","refundAmount":"100.00","refundReason":"test"}`
	req := httptest.NewRequest("POST", "/api/v1/orders/ord-test/refund", strings.NewReader(body))
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

func TestPVSWebhook_Handler(t *testing.T) {
	h := &Handler{
		orderSvc: &mockOrderSvc{},
		db:       &mockDB{},
	}

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
	h := &Handler{
		db: &mockDB{},
	}
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
	h := &Handler{
		db: &mockDB{pingErr: assertAnError{}},
	}
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, esperaba 503", w.Code)
	}
}

func TestCreateOrder_JSONInvalido(t *testing.T) {
	h := &Handler{
		orderSvc: &mockOrderSvc{},
		db:       &mockDB{},
	}

	// body no es JSON valido
	req := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader("{malformed"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperaba 400", w.Code)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	// Handler que paniquea
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

// --- Helpers de test ---

// assertAnError implementa error para tests donde necesitamos un error no nil.
type assertAnError struct{}

func (assertAnError) Error() string { return "error simulado" }

// Compile-time check: el handler compila con los mocks
var _ OrderService = (*mockOrderSvc)(nil)
var _ RefundService = (*mockRefundSvc)(nil)
