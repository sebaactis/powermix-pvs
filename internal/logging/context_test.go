package logging

import (
	"context"
	"testing"
)

func TestWithRequestID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	id := "req_test123"
	ctx = WithRequestID(ctx, id)

	got := RequestIDFrom(ctx)
	if got != id {
		t.Fatalf("RequestIDFrom = %q, want %q", got, id)
	}
}

func TestRequestIDFrom_EmptyContext(t *testing.T) {
	ctx := context.Background()
	got := RequestIDFrom(ctx)
	if got != "" {
		t.Fatalf("RequestIDFrom = %q, want empty string", got)
	}
}

func TestWithScanID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	id := "scan_test456"
	ctx = WithScanID(ctx, id)

	got := ScanIDFrom(ctx)
	if got != id {
		t.Fatalf("ScanIDFrom = %q, want %q", got, id)
	}
}

func TestScanIDFrom_EmptyContext(t *testing.T) {
	ctx := context.Background()
	got := ScanIDFrom(ctx)
	if got != "" {
		t.Fatalf("ScanIDFrom = %q, want empty string", got)
	}
}

func TestContextKeys_Independent(t *testing.T) {
	ctx := context.Background()
	ctx = WithRequestID(ctx, "req_xyz")
	ctx = WithScanID(ctx, "scan_abc")

	if RequestIDFrom(ctx) != "req_xyz" {
		t.Fatal("request_id sobreescrito por scan_id")
	}
	if ScanIDFrom(ctx) != "scan_abc" {
		t.Fatal("scan_id sobreescrito por request_id")
	}
}
