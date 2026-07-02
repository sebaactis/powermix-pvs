package pvs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/seba/vps-powermix/internal/ports"
)

// Helper: crea un cliente con el token ya precargado para que los
// tests no necesiten mockear el endpoint OAuth.
func clientePrepago(mockURL string, opts ...Opcion) *Cliente {
	c := New(mockURL, "test-client", "test-secret", opts...)
	c.tokenCache.mu.Lock()
	c.tokenCache.token = "token-precargado-test"
	c.tokenCache.expiresAt = time.Now().Add(1 * time.Hour)
	c.tokenCache.mu.Unlock()
	return c
}

func TestGenerateQR_OK(t *testing.T) {
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("metodo = %s, esperaba POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/qr/pvs") {
			t.Errorf("path = %s, esperaba .../qr/pvs", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token-precargado-test" {
			t.Errorf("Authorization = %q, esperaba Bearer token-precargado-test",
				r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"qrId":    "qr_abc123",
			"qrImage": "base64_fake_image",
		})
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(1000, 1000))
	resp, err := client.GenerateQR(context.Background(), &ports.PVSQRRequest{
		Amount:    15000,
		ExternalID: "order-001",
		Reference:  "ref-001",
	})
	if err != nil {
		t.Fatalf("GenerateQR fallo: %v", err)
	}
	if resp.QrID != "qr_abc123" {
		t.Errorf("QrID = %q, esperaba qr_abc123", resp.QrID)
	}
	if resp.QrImage != "base64_fake_image" {
		t.Errorf("QrImage = %q, esperaba base64_fake_image", resp.QrImage)
	}
}

func TestGenerateQR_Error4xx(t *testing.T) {
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"code":"400","message":"monto invalido"}`)
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(1000, 1000))
	_, err := client.GenerateQR(context.Background(), &ports.PVSQRRequest{
		Amount: 0, ExternalID: "order-bad", Reference: "ref-bad",
	})
	if err == nil {
		t.Fatal("se esperaba error, pero fue nil")
	}
}

func TestQueryStatus_OK_StateId5(t *testing.T) {
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("metodo = %s, esperaba GET", r.Method)
		}
		if !strings.Contains(r.URL.Path, "transactions/qrpvs/") {
			t.Errorf("path = %s, esperaba .../transactions/qrpvs/{qrId}", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token-precargado-test" {
			t.Errorf("Authorization malo: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"stateId": 5,
			"status":  "APPROVED",
		})
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(1000, 1000))
	resp, err := client.QueryStatus(context.Background(), "qr_abc123")
	if err != nil {
		t.Fatalf("QueryStatus fallo: %v", err)
	}
	if resp.StateID != 5 {
		t.Errorf("StateID = %d, esperaba 5 (APPROVED)", resp.StateID)
	}
}

func TestQueryStatus_StateId6_InProcess(t *testing.T) {
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"stateId": 6,
			"status":  "IN_PROCESS",
		})
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(1000, 1000))
	resp, err := client.QueryStatus(context.Background(), "qr_abc123")
	if err != nil {
		t.Fatalf("QueryStatus fallo: %v", err)
	}
	if resp.StateID != 6 {
		t.Errorf("StateID = %d, esperaba 6 (IN_PROCESS)", resp.StateID)
	}
}

func TestReverse_OK(t *testing.T) {
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("metodo = %s, esperaba POST", r.Method)
		}
		if !strings.Contains(r.URL.Path, "reverse") {
			t.Errorf("path = %s, esperaba .../reverse", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(1000, 1000))
	resp, err := client.Reverse(context.Background(), "qr_abc123")
	if err != nil {
		t.Fatalf("Reverse fallo: %v", err)
	}
	if !resp.Success {
		t.Error("Reverse deberia haber sido exitoso")
	}
}

func TestRetryOn401(t *testing.T) {
	intentosQR := 0
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Manejar OAuth (token refresh despues del 401)
		if strings.Contains(r.URL.Path, "oauth2") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "token_nuevo_tras_401",
				"expires_in":   3600,
			})
			return
		}
		// QR endpoint
		intentosQR++
		if intentosQR == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"qrId":    "qr_retry_ok",
			"qrImage": "base64_retry",
		})
	}))
	defer mockPVS.Close()

	// Token precargado, pero mock responde 401 en primer intento
	client := clientePrepago(mockPVS.URL, ConRateLimit(1000, 1000))

	resp, err := client.GenerateQR(context.Background(), &ports.PVSQRRequest{
		Amount: 1000, ExternalID: "order-retry", Reference: "ref-retry",
	})
	if err != nil {
		t.Fatalf("GenerateQR con retry fallo: %v", err)
	}
	if resp.QrID != "qr_retry_ok" {
		t.Errorf("QrID = %q, esperaba qr_retry_ok", resp.QrID)
	}
	if intentosQR != 2 {
		t.Errorf("intentos QR = %d, esperaba 2 (1 fallo 401 + 1 retry)", intentosQR)
	}
}

func TestRateLimitNoBloquea(t *testing.T) {
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"qrId": "qr_ok", "qrImage": "base64",
		})
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(5000, 5000))
	for i := 0; i < 10; i++ {
		_, err := client.GenerateQR(context.Background(), &ports.PVSQRRequest{
			Amount: 1000,
			ExternalID: fmt.Sprintf("order-%d", i),
			Reference:  fmt.Sprintf("ref-%d", i),
		})
		if err != nil {
			t.Fatalf("request %d fallo: %v", i, err)
		}
	}
}

// TestTokenCache_GetYInvalidate testea el OAuth2 cache directamente.
func TestTokenCache_GetYInvalidate(t *testing.T) {
	intentosOAuth := 0
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		intentosOAuth++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": fmt.Sprintf("token_nuevo_%d", intentosOAuth),
			"expires_in":   3600,
		})
	}))
	defer mockPVS.Close()

	cache := NewTokenCache(mockPVS.URL, "test-client", "test-secret")

	// Primer Get: debe pedir token
	tok1, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("primer Get fallo: %v", err)
	}
	if tok1 == "" {
		t.Error("primer token vacio")
	}
	if intentosOAuth != 1 {
		t.Errorf("intentos OAuth = %d, esperaba 1", intentosOAuth)
	}

	// Segundo Get: debe REUSAR el cache
	tok2, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("segundo Get fallo: %v", err)
	}
	if tok2 != tok1 {
		t.Error("segundo token deberia ser el mismo que el primero (cache)")
	}
	if intentosOAuth != 1 {
		t.Errorf("intentos OAuth = %d, esperaba 1 (cache hit)", intentosOAuth)
	}

	// Invalidate + Get: debe pedir token NUEVO
	cache.Invalidate(context.Background())
	tok3, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("tercer Get fallo: %v", err)
	}
	if tok3 == tok1 {
		t.Error("tercer token NO deberia ser el mismo que el primero (invalido)")
	}
	if intentosOAuth != 2 {
		t.Errorf("intentos OAuth = %d, esperaba 2 (invalido + refresh)", intentosOAuth)
	}
}
