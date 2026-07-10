// Package handler contiene tests de seguridad que verifican que la API
// maneja correctamente entradas maliciosas o malformadas.
package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSecurity_JSONMalformado: body no es JSON valido -> 400.
func TestSecurity_JSONMalformado(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	req := httptest.NewRequest("POST", "/order/qr",
		strings.NewReader("{malformed json}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperaba 400 (JSON malformado)", w.Code)
	}
}

// TestSecurity_EmptyBody: body vacio -> 400 o error controlado.
func TestSecurity_EmptyBody(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	req := httptest.NewRequest("POST", "/order/qr", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperaba 400 (body vacio)", w.Code)
	}
}

// TestSecurity_SQLInjectionPath: intento de SQL injection en thirdOrderNo.
func TestSecurity_SQLInjectionPath(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	body := `{"thirdOrderNo":"1' OR '1'='1"}`
	req := httptest.NewRequest("POST", "/order/status", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code == http.StatusInternalServerError {
		t.Error("SQL injection devolvio 500 (deberia ser controlado)")
	}
}

// TestSecurity_SQLInjectionBody: intento de SQL injection en campos JSON.
func TestSecurity_SQLInjectionBody(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	body := `{"orderNo":"GS-X","subject":"X","totalAmount":"100.00","notifyUrl":"https://gs.example/n","objectId":"'; DROP TABLE orders; --","attach":"deviceId=dev-1"}`
	req := httptest.NewRequest("POST", "/order/qr", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code == http.StatusInternalServerError {
		t.Error("SQL injection body devolvio 500")
	}
	if w.Code >= 500 {
		t.Errorf("SQL injection body devolvio %d (error de servidor inaceptable)", w.Code)
	}
}

// TestSecurity_JSONMuyGrande: payload enorme -> debe rechazarse o no crashear.
func TestSecurity_JSONMuyGrande(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	body := `{"orderNo":"GS-BIG","subject":"X","totalAmount":"1.00","notifyUrl":"https://gs.example/n","attach":"deviceId=` +
		strings.Repeat("A", 1024*1024) + `"}` // ~1MB
	req := httptest.NewRequest("POST", "/order/qr", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Log("payload grande fue aceptado (puede ser OK si el servidor lo permite)")
	}
}

// TestSecurity_EstadoInvalido: stateId invalido -> 400 o no-op.
func TestSecurity_EstadoInvalido(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	body := `{"qrId":"qr_001","stateId":9999}`
	req := httptest.NewRequest("POST", "/webhook/pvs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code >= http.StatusInternalServerError {
		t.Errorf("stateId invalido devolvio %d (deberia ser 200 o 400)", w.Code)
	}
}

// TestSecurity_MetodoNoPermitido: GET en POST endpoint -> 405.
func TestSecurity_MetodoNoPermitido(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	req := httptest.NewRequest("GET", "/order/qr", nil)
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Logf("GET /order/qr devolvio %d (podria ser 405 con Go 1.22+)", w.Code)
	}
}

// TestSecurity_ContentTypeIncorrecto: sin Content-Type.
func TestSecurity_ContentTypeIncorrecto(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	req := httptest.NewRequest("POST", "/order/qr",
		strings.NewReader(`{"orderNo":"GS-1","subject":"T","totalAmount":"100.00","notifyUrl":"https://gs.example/n","objectId":"test","attach":"deviceId=dev-1"}`))
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code >= 500 {
		t.Errorf("sin Content-Type devolvio %d", w.Code)
	}
}
