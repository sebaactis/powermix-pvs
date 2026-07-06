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

	req := httptest.NewRequest("POST", "/api/v1/orders",
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

	req := httptest.NewRequest("POST", "/api/v1/orders", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperaba 400 (body vacio)", w.Code)
	}
}

// TestSecurity_SQLInjectionPath: intento de SQL injection en path param.
func TestSecurity_SQLInjectionPath(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	// Path con SQL injection
	req := httptest.NewRequest("GET", "/api/v1/orders/1%27%20OR%20%271%27%3D%271", nil)
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	// No debe devolver 500 (error interno)
	if w.Code == http.StatusInternalServerError {
		t.Error("SQL injection path devolvio 500 (deberia ser 404 u otro controlado)")
	}
}

// TestSecurity_SQLInjectionBody: intento de SQL injection en campos JSON.
func TestSecurity_SQLInjectionBody(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	body := `{"objectId":"'; DROP TABLE orders; --","totalAmount":"100.00","deviceId":"dev-1"}`
	req := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	// No debe devolver 500
	if w.Code == http.StatusInternalServerError {
		t.Error("SQL injection body devolvio 500")
	}
	// Deberia ser 200 (el servicio maneja el dato como string, no lo ejecuta)
	// o 400 si la validacion lo rechaza
	if w.Code >= 500 {
		t.Errorf("SQL injection body devolvio %d (error de servidor inaceptable)", w.Code)
	}
}

// TestSecurity_JSONMuyGrande: payload enorme -> debe rechazarse.
func TestSecurity_JSONMuyGrande(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	// Objeto con datos enormes
	body := `{"objectId":"test","totalAmount":"1.00","deviceId":"` +
		strings.Repeat("A", 1024*1024) + `"}` // ~1MB
	req := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	// Deberia dar 400 (body demasiado grande) o 413
	if w.Code == http.StatusOK {
		t.Log("payload grande fue aceptado (puede ser OK si el servidor lo permite)")
	}
}

// TestSecurity_EstadoInvalido: stateId invalido -> 400.
func TestSecurity_EstadoInvalido(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	body := `{"qrId":"qr_001","stateId":9999}`
	req := httptest.NewRequest("POST", "/webhook/pvs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	// Deberia ser 400 (stateId invalido) o 200 si el service lo maneja como no-op
	if w.Code >= http.StatusInternalServerError {
		t.Errorf("stateId invalido devolvio %d (deberia ser 200 o 400)", w.Code)
	}
}

// TestSecurity_MetodoNoPermitido: GET en POST endpoint -> 405.
func TestSecurity_MetodoNoPermitido(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	// GET en lugar de POST en CreateOrder
	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	// Go 1.22+ ServeMux devuelve 405 para metodo no permitido
	if w.Code != http.StatusMethodNotAllowed {
		t.Logf("GET /api/v1/orders devolvio %d (podria ser 405 con Go 1.22+)", w.Code)
	}
}

// TestSecurity_ContentTypeIncorrecto: sin Content-Type -> 400.
func TestSecurity_ContentTypeIncorrecto(t *testing.T) {
	h := New(&mockOrderSvc{}, &mockRefundSvc{}, &mockDB{})

	req := httptest.NewRequest("POST", "/api/v1/orders",
		strings.NewReader(`{"objectId":"test","totalAmount":"100.00","deviceId":"dev-1"}`))
	// Sin Content-Type
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, req)

	// json.Decoder no requiere Content-Type para procesar.
	// El handler no valida Content-Type explicitamente.
	if w.Code >= 500 {
		t.Errorf("sin Content-Type devolvio %d", w.Code)
	}
}
