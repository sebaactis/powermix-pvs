package logging

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestSanitizeBody_Empty(t *testing.T) {
	if got := SanitizeBody(nil, 0); got != "" {
		t.Fatalf("nil: got %q, want empty", got)
	}
	if got := SanitizeBody([]byte{}, 0); got != "" {
		t.Fatalf("empty: got %q, want empty", got)
	}
}

func TestSanitizeBody_RedactsSensitiveKeys(t *testing.T) {
	raw := []byte(`{"user":"seba","password":"super-secret","token":"abc123"}`)
	got := SanitizeBody(raw, 0)

	if strings.Contains(got, "super-secret") || strings.Contains(got, "abc123") {
		t.Fatalf("secret leaked: %s", got)
	}
	if !strings.Contains(got, `"password":"***"`) {
		t.Fatalf("password not redacted: %s", got)
	}
	if !strings.Contains(got, `"token":"***"`) {
		t.Fatalf("token not redacted: %s", got)
	}
	if !strings.Contains(got, `"user":"seba"`) {
		t.Fatalf("safe field lost: %s", got)
	}
}

func TestSanitizeBody_RedactsNestedSensitiveKeys(t *testing.T) {
	raw := []byte(`{"data":{"access_token":"tok-nested","ok":true}}`)
	got := SanitizeBody(raw, 0)

	if strings.Contains(got, "tok-nested") {
		t.Fatalf("nested token leaked: %s", got)
	}
	if !strings.Contains(got, `"access_token":"***"`) {
		t.Fatalf("nested access_token not redacted: %s", got)
	}
}

func TestSanitizeBody_TruncatesQRFields(t *testing.T) {
	// qrUrl corto también se marca (campo conocido como "grande").
	raw := []byte(`{"qrUrl":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB","orderNo":"GS-1"}`)
	got := SanitizeBody(raw, 0)

	if strings.Contains(got, "iVBORw0KGgo") {
		t.Fatalf("qrUrl raw still present: %s", got)
	}
	if !strings.Contains(got, `[truncated`) {
		t.Fatalf("expected truncated marker: %s", got)
	}
	if !strings.Contains(got, `"orderNo":"GS-1"`) {
		t.Fatalf("safe field lost: %s", got)
	}
}

func TestSanitizeBody_TruncatesLongGenericString(t *testing.T) {
	long := strings.Repeat("a", maxLoggedStringRunes+10)
	raw, err := json.Marshal(map[string]string{"note": long})
	if err != nil {
		t.Fatal(err)
	}
	got := SanitizeBody(raw, 0)

	if strings.Contains(got, long) {
		t.Fatalf("long string not truncated: %s", got)
	}
	if !strings.Contains(got, `[truncated`) {
		t.Fatalf("expected truncated marker: %s", got)
	}
}

func TestSanitizeBody_NonJSONTruncates(t *testing.T) {
	raw := []byte("not-json-but-useful-payload")
	got := SanitizeBody(raw, 10)
	if len(got) > 10 {
		t.Fatalf("len=%d > max 10: %q", len(got), got)
	}
	if !strings.HasSuffix(got, "...") && len(raw) > 10 {
		// con max 10 debe truncar con ...
		t.Fatalf("expected truncation suffix: %q", got)
	}
}

func TestSanitizeBody_RespectsMaxBytes(t *testing.T) {
	// JSON grande: muchas keys para forzar truncate final.
	m := map[string]string{}
	for i := 0; i < 50; i++ {
		m["k"+strconv.Itoa(i)] = strings.Repeat("x", 40)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	const max = 64
	got := SanitizeBody(raw, max)
	if len(got) > max {
		t.Fatalf("len=%d > max %d: %q", len(got), max, got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ... suffix: %q", got)
	}
}

func TestSanitizeBody_JSONOutputWhenInputJSON(t *testing.T) {
	raw := []byte(`{"orderNo":"GS-1","totalAmount":"100.00"}`)
	got := SanitizeBody(raw, 0)
	var v map[string]any
	if err := json.Unmarshal([]byte(got), &v); err != nil {
		t.Fatalf("output not valid JSON: %v (%s)", err, got)
	}
	if v["orderNo"] != "GS-1" {
		t.Fatalf("orderNo = %v", v["orderNo"])
	}
}

func TestSanitizeBody_SensitiveKeyCaseInsensitive(t *testing.T) {
	raw := []byte(`{"Password":"x","CLIENT_SECRET":"y"}`)
	got := SanitizeBody(raw, 0)
	if strings.Contains(got, `"x"`) || strings.Contains(got, `"y"`) {
		t.Fatalf("case-insensitive redact failed: %s", got)
	}
}
