package logging

import (
	"strings"
	"testing"
)

func TestNewRequestID_Format(t *testing.T) {
	id := NewRequestID()
	if !strings.HasPrefix(id, "req_") {
		t.Fatalf("NewRequestID = %q, quiere prefijo req_", id)
	}
	body := strings.TrimPrefix(id, "req_")
	if len(body) != 26 {
		t.Fatalf("cuerpo ULID tiene %d chars, quiere 26 (got %q)", len(body), body)
	}
}

func TestNewRequestID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewRequestID()
		if ids[id] {
			t.Fatalf("NewRequestID duplicado en iteración %d: %s", i, id)
		}
		ids[id] = true
	}
}

func TestNewScanID_Format(t *testing.T) {
	id := NewScanID()
	if !strings.HasPrefix(id, "scan_") {
		t.Fatalf("NewScanID = %q, quiere prefijo scan_", id)
	}
	body := strings.TrimPrefix(id, "scan_")
	// uuid v4 sin guiones = 32 chars; con guiones = 36
	if len(body) != 36 {
		t.Fatalf("UUID v4 tiene %d chars, quiere 36 (got %q)", len(body), body)
	}
}

func TestNewScanID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := NewScanID()
		if ids[id] {
			t.Fatalf("NewScanID duplicado en iteración %d: %s", i, id)
		}
		ids[id] = true
	}
}

func TestNewRequestID_CrockfordAlphabet(t *testing.T) {
	const valid = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	id := NewRequestID()
	body := strings.TrimPrefix(id, "req_")
	for _, c := range body {
		if !strings.ContainsRune(valid, c) {
			t.Fatalf("carácter %q fuera del alfabeto Crockford en %q", c, body)
		}
	}
}
