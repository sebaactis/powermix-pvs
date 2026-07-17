package pvs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/seba/vps-powermix/internal/domain"
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
		// Envelope real PVS: { code, message, ok, data }
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    "200",
			"message": "OK",
			"ok":      true,
			"data": map[string]string{
				"qrId":    "qr_abc123",
				"qrImage": "base64_fake_image",
			},
		})
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(1000, 1000))
	resp, err := client.GenerateQR(context.Background(), &ports.PVSQRRequest{
		Amount:     15000,
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

// Ejemplo live a veces manda data.qr en vez de data.qrImage.
func TestGenerateQR_CampoQrFallback(t *testing.T) {
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "OK", "message": "Operacion exitosa.", "ok": true,
			"data": map[string]string{
				"qrId": "qr_via_qr_field",
				"qr":   "base64_via_qr",
			},
		})
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(1000, 1000))
	resp, err := client.GenerateQR(context.Background(), &ports.PVSQRRequest{
		Amount: 1000, ExternalID: "e1", Reference: "r1",
	})
	if err != nil {
		t.Fatalf("GenerateQR fallo: %v", err)
	}
	if resp.QrID != "qr_via_qr_field" {
		t.Errorf("QrID = %q", resp.QrID)
	}
	if resp.QrImage != "base64_via_qr" {
		t.Errorf("QrImage = %q, esperaba base64_via_qr (fallback de data.qr)", resp.QrImage)
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

func TestMapearError_4xx_PVSBusinessError(t *testing.T) {
	c := clientePrepago("http://unused")
	err := c.mapearError(http.StatusBadRequest,
		[]byte(`{"code":"E_007","message":"Monto invalido","ok":false}`))

	var be *domain.PVSBusinessError
	if !errors.As(err, &be) {
		t.Fatalf("esperaba *domain.PVSBusinessError, got %T: %v", err, err)
	}
	if be.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, esperaba 400", be.StatusCode)
	}
	if be.Code != "E_007" || be.Message != "Monto invalido" {
		t.Errorf("Code/Message = %q/%q, esperaba E_007/Monto invalido", be.Code, be.Message)
	}
	if got := err.Error(); got != "E_007: Monto invalido" {
		t.Errorf("Error() = %q", got)
	}
}

func TestMapearError_5xx_Interno(t *testing.T) {
	c := clientePrepago("http://unused")
	err := c.mapearError(http.StatusInternalServerError,
		[]byte(`{"code":"X","message":"boom","ok":false}`))

	var be *domain.PVSBusinessError
	if errors.As(err, &be) {
		t.Fatalf("5xx NO debe ser PVSBusinessError, got %T", err)
	}
	var hse *httpStatusError
	if !errors.As(err, &hse) || hse.CodigoHTTP() != 500 {
		t.Fatalf("esperaba *httpStatusError 500, got %T: %v", err, err)
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
		// Envelope real PVS: stateId vive en data
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "200", "message": "OK", "ok": true,
			"data": map[string]interface{}{
				"stateId": 5,
				"status":  "APPROVED",
			},
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
			"code": "200", "message": "OK", "ok": true,
			"data": map[string]interface{}{
				"stateId": 6,
				"status":  "IN_PROCESS",
			},
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
		// Envelope real PVS: txeId vive en data
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "200", "message": "OK", "ok": true,
			"data": map[string]string{
				"txeId": "txe_rev_001",
			},
		})
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
	if resp.TxEID != "txe_rev_001" {
		t.Errorf("TxEID = %q, esperaba txe_rev_001", resp.TxEID)
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
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "200", "message": "OK", "ok": true,
			"data": map[string]string{
				"qrId":    "qr_retry_ok",
				"qrImage": "base64_retry",
			},
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
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "200", "message": "OK", "ok": true,
			"data": map[string]string{
				"qrId": "qr_ok", "qrImage": "base64",
			},
		})
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(5000, 5000))
	for i := 0; i < 10; i++ {
		_, err := client.GenerateQR(context.Background(), &ports.PVSQRRequest{
			Amount:     1000,
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

// TestTokenCache_OAuthBasicAuth: doc PVS pide Basic(clientID:secret)
// + form solo grant_type. No client_id/secret en body.
func TestTokenCache_OAuthBasicAuth(t *testing.T) {
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, esperaba POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			t.Errorf("path = %s, esperaba .../oauth2/token", r.URL.Path)
		}

		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Error("falta Authorization Basic")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if user != "test-client" || pass != "test-secret" {
			t.Errorf("BasicAuth = %q:%q, esperaba test-client:test-secret", user, pass)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("grant_type = %q, esperaba client_credentials", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "" || r.Form.Get("client_secret") != "" {
			t.Errorf("body no debe traer client_id/client_secret: %v", r.Form)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token-via-basic",
			"expires_in":   3600,
		})
	}))
	defer mockPVS.Close()

	cache := NewTokenCache(mockPVS.URL, "test-client", "test-secret")
	tok, err := cache.Get(context.Background())
	if err != nil {
		t.Fatalf("Get fallo: %v", err)
	}
	if tok != "token-via-basic" {
		t.Errorf("token = %q, esperaba token-via-basic", tok)
	}
}

func TestDecodePVSData_OK(t *testing.T) {
	body := []byte(`{
		"code":"200","message":"OK","ok":true,
		"data":{"qrId":"qr_1","qrImage":"img"}
	}`)
	type qrData struct {
		QrID    string `json:"qrId"`
		QrImage string `json:"qrImage"`
	}
	dest, err := decodePVSData[qrData](body)
	if err != nil {
		t.Fatalf("decodePVSData: %v", err)
	}
	if dest.QrID != "qr_1" || dest.QrImage != "img" {
		t.Errorf("dest = %+v", dest)
	}
}

func TestDecodePVSData_SinData(t *testing.T) {
	body := []byte(`{"code":"500","message":"fail","ok":false}`)
	_, err := decodePVSData[struct{}](body)
	if err == nil {
		t.Fatal("esperaba error sin data")
	}
}

func captureSlog(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prev := slog.Default()
	slog.SetDefault(logger)
	return &buf, func() { slog.SetDefault(prev) }
}

func TestGenerateQR_LogsSanitizedBodies(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	// qrImage largo para forzar marker de truncate en el log
	longQR := strings.Repeat("A", 300)
	mockPVS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "200", "message": "OK", "ok": true,
			"data": map[string]string{
				"qrId":    "qr_log_1",
				"qrImage": longQR,
			},
		})
	}))
	defer mockPVS.Close()

	client := clientePrepago(mockPVS.URL, ConRateLimit(1000, 1000))
	_, err := client.GenerateQR(context.Background(), &ports.PVSQRRequest{
		Amount:     10000,
		ExternalID: "order-log-001",
		Reference:  "ref-log-001",
	})
	if err != nil {
		t.Fatalf("GenerateQR: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "msg=pvs.http.request") {
		t.Fatalf("missing pvs.http.request: %s", out)
	}
	if !strings.Contains(out, "msg=pvs.http.response") {
		t.Fatalf("missing pvs.http.response: %s", out)
	}
	if !strings.Contains(out, "order-log-001") {
		t.Fatalf("request externalId not in log: %s", out)
	}
	if strings.Contains(out, longQR) {
		t.Fatalf("raw qrImage leaked into logs: %s", out)
	}
	if !strings.Contains(out, "[truncated") {
		t.Fatalf("expected truncated marker for qrImage: %s", out)
	}
}

func TestFetchToken_RedactsAccessTokenInLogs(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	const secretToken = "super-secret-access-token-xyz"
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": secretToken,
			"expires_in":   3600,
		})
	}))
	defer mock.Close()

	cache := NewTokenCache(mock.URL, "cid", "csecret")
	tok, err := cache.fetchToken(context.Background())
	if err != nil {
		t.Fatalf("fetchToken: %v", err)
	}
	if tok != secretToken {
		t.Fatalf("token en memoria = %q", tok)
	}

	out := buf.String()
	if !strings.Contains(out, "msg=pvs.http.request") {
		t.Fatalf("missing oauth request log: %s", out)
	}
	if !strings.Contains(out, "msg=pvs.http.response") {
		t.Fatalf("missing oauth response log: %s", out)
	}
	if strings.Contains(out, secretToken) {
		t.Fatalf("access_token leaked in logs: %s", out)
	}
	if strings.Contains(out, "csecret") {
		t.Fatalf("client_secret leaked in logs: %s", out)
	}
	// token sigue en memoria para uso real
	if cache.token != secretToken {
		t.Fatalf("cache token broken")
	}
}
