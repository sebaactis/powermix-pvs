package logging

import (
	"strings"
	"testing"
)

func TestConfigureHTTPBodyLogging_DefaultOn(t *testing.T) {
	// Restaura defaults al final para no ensuciar otros tests del paquete.
	defer ConfigureHTTPBodyLogging(true, DefaultMaxBodyLogBytes)

	ConfigureHTTPBodyLogging(true, 0)
	if !HTTPBodyLoggingEnabled() {
		t.Fatal("expected enabled")
	}
	body, ok := FormatBodyForLog([]byte(`{"a":1}`))
	if !ok || !strings.Contains(body, `"a":1`) {
		t.Fatalf("FormatBodyForLog on: ok=%v body=%q", ok, body)
	}
}

func TestConfigureHTTPBodyLogging_Off(t *testing.T) {
	defer ConfigureHTTPBodyLogging(true, DefaultMaxBodyLogBytes)

	ConfigureHTTPBodyLogging(false, 1024)
	if HTTPBodyLoggingEnabled() {
		t.Fatal("expected disabled")
	}
	body, ok := FormatBodyForLog([]byte(`{"password":"x"}`))
	if ok || body != "" {
		t.Fatalf("when off want ok=false empty body; got ok=%v body=%q", ok, body)
	}
}

func TestConfigureHTTPBodyLogging_MaxBytes(t *testing.T) {
	defer ConfigureHTTPBodyLogging(true, DefaultMaxBodyLogBytes)

	ConfigureHTTPBodyLogging(true, 20)
	if HTTPBodyMaxBytes() != 20 {
		t.Fatalf("max=%d want 20", HTTPBodyMaxBytes())
	}
	// payload grande → truncado al max configurado
	raw := []byte(`{"note":"` + strings.Repeat("z", 100) + `"}`)
	body, ok := FormatBodyForLog(raw)
	if !ok {
		t.Fatal("expected ok")
	}
	if len(body) > 20 {
		t.Fatalf("len=%d > 20: %q", len(body), body)
	}
}
