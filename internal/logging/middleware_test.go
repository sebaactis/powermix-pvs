package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddleware_GeneratesNewID(t *testing.T) {
	var capturedID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedID == "" {
		t.Fatal("middleware no inyectó request_id en el contexto del handler")
	}

	// El response header debe tener el mismo ID
	respID := rec.Header().Get("X-Request-ID")
	if respID != capturedID {
		t.Fatalf("X-Request-ID response = %q, contexto = %q", respID, capturedID)
	}
}

func TestRequestIDMiddleware_UsesIncomingHeader(t *testing.T) {
	incomingID := "req_from_client_999"
	var capturedID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", incomingID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedID != incomingID {
		t.Fatalf("middleware ignoró header entrante: got %q, want %q", capturedID, incomingID)
	}

	respID := rec.Header().Get("X-Request-ID")
	if respID != incomingID {
		t.Fatalf("X-Request-ID response = %q, want %q", respID, incomingID)
	}
}

func TestRequestIDMiddleware_PassesToNext(t *testing.T) {
	called := false
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest("POST", "/orders", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("middleware no llamó al handler siguiente")
	}
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusTeapot)
	}
}

func TestRequestIDMiddleware_EmptyHeader_GeneratesID(t *testing.T) {
	var capturedID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFrom(r.Context())
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "") // header vacío
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedID == "" {
		t.Fatal("middleware no generó ID nuevo cuando el header estaba vacío")
	}
}

func TestRequestIDMiddleware_IDAvailableViaFrom(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// Verifica que From(ctx) funciona después del middleware
		logger := From(ctx)
		if logger == nil {
			t.Fatal("From(ctx) devolvió nil dentro del handler")
		}
		// Verifica que el request_id está disponible para lectura directa
		id := RequestIDFrom(ctx)
		if id == "" {
			t.Fatal("RequestIDFrom devolvió vacío dentro del handler")
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}
