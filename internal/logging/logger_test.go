package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// captureLogger crea un slog.Logger que escribe a un buffer y lo retorna
// junto con el buffer para inspección.
func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), &buf
}

// replaceDefaultLogger instala un logger temporal y restaura el original al final.
func replaceDefaultLogger(l *slog.Logger) func() {
	orig := slog.Default()
	slog.SetDefault(l)
	return func() { slog.SetDefault(orig) }
}

func TestFrom_WithRequestID(t *testing.T) {
	l, buf := captureLogger()
	restore := replaceDefaultLogger(l)
	defer restore()

	ctx := WithRequestID(context.Background(), "req_abc")
	logger := From(ctx)
	logger.Info("mensaje de prueba")

	out := buf.String()
	if !strings.Contains(out, "request_id=req_abc") {
		t.Fatalf("output no contiene request_id=req_abc: %s", out)
	}
}

func TestFrom_WithScanID(t *testing.T) {
	l, buf := captureLogger()
	restore := replaceDefaultLogger(l)
	defer restore()

	ctx := WithScanID(context.Background(), "scan_xyz")
	logger := From(ctx)
	logger.Info("mensaje de prueba")

	out := buf.String()
	if !strings.Contains(out, "scan_id=scan_xyz") {
		t.Fatalf("output no contiene scan_id=scan_xyz: %s", out)
	}
}

func TestFrom_WithBothIDs(t *testing.T) {
	l, buf := captureLogger()
	restore := replaceDefaultLogger(l)
	defer restore()

	ctx := WithRequestID(context.Background(), "req_123")
	ctx = WithScanID(ctx, "scan_456")
	logger := From(ctx)
	logger.Info("mensaje con ambos IDs")

	out := buf.String()
	if !strings.Contains(out, "request_id=req_123") {
		t.Fatalf("falta request_id=req_123 en output: %s", out)
	}
	if !strings.Contains(out, "scan_id=scan_456") {
		t.Fatalf("falta scan_id=scan_456 en output: %s", out)
	}
}

func TestFrom_EmptyContext(t *testing.T) {
	l, buf := captureLogger()
	restore := replaceDefaultLogger(l)
	defer restore()

	ctx := context.Background()
	logger := From(ctx)
	logger.Info("mensaje sin IDs")

	out := buf.String()
	if strings.Contains(out, "request_id=") {
		t.Fatalf("output NO debería tener request_id: %s", out)
	}
	if strings.Contains(out, "scan_id=") {
		t.Fatalf("output NO debería tener scan_id: %s", out)
	}
}

func TestFrom_ReturnsSlogLogger(t *testing.T) {
	logger := From(context.Background())
	if logger == nil {
		t.Fatal("From(ctx) devolvió nil")
	}
	// Verifica que el tipo concreto es *slog.Logger (no un wrapper custom)
	if _, ok := any(logger).(*slog.Logger); !ok {
		t.Fatalf("From(ctx) no devolvió *slog.Logger, devolvió %T", logger)
	}
}
