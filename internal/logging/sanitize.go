package logging

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DefaultMaxBodyLogBytes es el tamaño máximo del body ya sanitizado en logs.
const DefaultMaxBodyLogBytes = 8 * 1024

// maxLoggedStringRunes: cualquier string JSON más largo se reemplaza por un marker.
// Cubre base64 / payloads grandes aunque el key no sea conocido (qrUrl, etc.).
const maxLoggedStringRunes = 256

var sensitiveKeys = map[string]struct{}{
	"token":         {},
	"secret":        {},
	"password":      {},
	"client_secret": {},
	"authorization": {},
	"api_key":       {},
	"access_token":  {},
	"refresh_token": {},
}

var largeFieldKeys = map[string]struct{}{
	"qrurl":   {},
	"qrimage": {},
	"qrraw":   {},
	"qr":      {},
	"image":   {},
}

// SanitizeBody prepara un body HTTP para log:
//   - redacta keys sensibles en JSON (case-insensitive)
//   - achica campos tipo QR / strings largos
//   - trunca el resultado final a maxBytes (0 o negativo = default)
//
// Si raw no es JSON válido, se trunca como texto. Nunca paniquea.
func SanitizeBody(raw []byte, maxBytes int) string {
	if len(raw) == 0 {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodyLogBytes
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return truncateString(string(raw), maxBytes)
	}

	out, err := json.Marshal(sanitizeValue(v, ""))
	if err != nil {
		return truncateString(string(raw), maxBytes)
	}
	return truncateString(string(out), maxBytes)
}

func sanitizeValue(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, child := range x {
			lk := strings.ToLower(k)
			if _, ok := sensitiveKeys[lk]; ok {
				out[k] = "***"
				continue
			}
			if _, ok := largeFieldKeys[lk]; ok {
				if s, ok := child.(string); ok {
					out[k] = truncatedMarker(s)
					continue
				}
			}
			out[k] = sanitizeValue(child, k)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, child := range x {
			out[i] = sanitizeValue(child, key)
		}
		return out
	case string:
		if utf8.RuneCountInString(x) > maxLoggedStringRunes {
			return truncatedMarker(x)
		}
		return x
	default:
		return v
	}
}

func truncatedMarker(s string) string {
	return "[truncated " + strconv.Itoa(len(s)) + " bytes]"
}

func truncateString(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	if maxBytes <= 3 {
		// Corta en límite de rune para no partir UTF-8.
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		return s[:cut]
	}
	cut := maxBytes - 3
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}
