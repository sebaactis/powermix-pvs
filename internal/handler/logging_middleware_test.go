package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seba/vps-powermix/internal/logging"
)

func captureSlog(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prev := slog.Default()
	slog.SetDefault(logger)
	return &buf, func() { slog.SetDefault(prev) }
}

func TestLoggingMiddleware_OrderPath_LogsReqAndResBody(t *testing.T) {
	logging.ConfigureHTTPBodyLogging(true, 0)
	defer logging.ConfigureHTTPBodyLogging(true, 0)

	buf, restore := captureSlog(t)
	defer restore()

	const reqJSON = `{"orderNo":"GS-1","totalAmount":"100.00"}`
	var seenBody string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("handler read body: %v", err)
		}
		seenBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":200,"msg":"success"}`))
	})

	h := logging.RequestIDMiddleware(loggingMiddleware(inner))
	req := httptest.NewRequest(http.MethodPost, "/order/qr", strings.NewReader(reqJSON))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if seenBody != reqJSON {
		t.Fatalf("handler body = %q, want %q", seenBody, reqJSON)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"code":200`) {
		t.Fatalf("client body broken: %s", rr.Body.String())
	}

	out := buf.String()
	if !strings.Contains(out, "msg=http.request") {
		t.Fatalf("missing http.request log: %s", out)
	}
	if !strings.Contains(out, "msg=http.response") {
		t.Fatalf("missing http.response log: %s", out)
	}
	if !strings.Contains(out, "GS-1") {
		t.Fatalf("request body not in log: %s", out)
	}
	if !strings.Contains(out, "status=200") {
		t.Fatalf("response status not in log: %s", out)
	}
	if !strings.Contains(out, "success") {
		t.Fatalf("response body not in log: %s", out)
	}
}

func TestLoggingMiddleware_RedactsSecretsInBody(t *testing.T) {
	logging.ConfigureHTTPBodyLogging(true, 0)
	defer logging.ConfigureHTTPBodyLogging(true, 0)

	buf, restore := captureSlog(t)
	defer restore()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	h := loggingMiddleware(inner)
	req := httptest.NewRequest(http.MethodPost, "/order/status",
		strings.NewReader(`{"password":"super-secret","orderNo":"GS-2"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	out := buf.String()
	if strings.Contains(out, "super-secret") {
		t.Fatalf("secret leaked in logs: %s", out)
	}
	if !strings.Contains(out, "***") {
		t.Fatalf("expected redaction marker: %s", out)
	}
}

func TestLoggingMiddleware_FlagOff_NoBodyField(t *testing.T) {
	logging.ConfigureHTTPBodyLogging(false, 0)
	defer logging.ConfigureHTTPBodyLogging(true, 0)

	buf, restore := captureSlog(t)
	defer restore()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h := loggingMiddleware(inner)
	req := httptest.NewRequest(http.MethodPost, "/order/qr",
		strings.NewReader(`{"orderNo":"GS-OFF"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	out := buf.String()
	if !strings.Contains(out, "msg=http.request") {
		t.Fatalf("missing http.request: %s", out)
	}
	if strings.Contains(out, "GS-OFF") || strings.Contains(out, "body=") {
		t.Fatalf("body must not be logged when flag off: %s", out)
	}
}

func TestLoggingMiddleware_Healthz_NoBodyField(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	h := loggingMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	out := buf.String()
	if !strings.Contains(out, "msg=http.request") {
		t.Fatalf("missing http.request: %s", out)
	}
	// TextHandler imprime body= solo si el atributo existe.
	if strings.Contains(out, "body=") {
		t.Fatalf("healthz must not log body attr: %s", out)
	}
	if strings.Contains(out, "msg=http.response") {
		t.Fatalf("healthz must not log http.response body path: %s", out)
	}
}

func TestLoggingMiddleware_WebhookPVS_LogsBody(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	h := loggingMiddleware(inner)
	body := `{"qrId":"qr_1","stateId":5}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/pvs", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	out := buf.String()
	if !strings.Contains(out, "qr_1") {
		t.Fatalf("webhook body missing from log: %s", out)
	}
	if !strings.Contains(out, "msg=http.response") {
		t.Fatalf("missing http.response: %s", out)
	}
}

func TestShouldLogHTTPBody(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/order/qr", true},
		{"/order/status", true},
		{"/webhook/pvs", true},
		{"/healthz", false},
		{"/metrics", false},
		{"/", false},
	}
	for _, tc := range cases {
		if got := shouldLogHTTPBody(tc.path); got != tc.want {
			t.Errorf("shouldLogHTTPBody(%q)=%v want %v", tc.path, got, tc.want)
		}
	}
}

func TestLoggingMiddleware_ResponseStillValidJSON(t *testing.T) {
	_, restore := captureSlog(t)
	defer restore()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGSOK(w, map[string]string{"thirdOrderNo": "ord-1"})
	})
	h := loggingMiddleware(inner)
	req := httptest.NewRequest(http.MethodPost, "/order/qr", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var env gsEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("response not JSON: %v body=%s", err, rr.Body.String())
	}
	if env.Code != 200 {
		t.Fatalf("code=%d", env.Code)
	}
}
