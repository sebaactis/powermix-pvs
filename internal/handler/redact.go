package handler

import (
	"context"
	"log/slog"
)

// RedactingHandler es un wrapper de slog.Handler que redacta atributos
// sensibles antes de escribirlos al log.
// Previene fugas de tokens, secrets, y datos personales en los logs.
type RedactingHandler struct {
	inner     slog.Handler
	sensitive map[string]bool
}

// NewRedactingHandler crea un handler que redacta los atributos con
// las claves indicadas (ej: "token", "secret", "password").
func NewRedactingHandler(inner slog.Handler, sensitiveKeys ...string) *RedactingHandler {
	s := make(map[string]bool, len(sensitiveKeys))
	for _, k := range sensitiveKeys {
		s[k] = true
	}
	return &RedactingHandler{inner: inner, sensitive: s}
}

// Enabled delega en el handler interno.
func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle redacta atributos sensibles y luego delega en el handler interno.
func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	var attrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		if h.sensitive[a.Key] {
			a.Value = slog.StringValue("***REDACTED***")
		}
		attrs = append(attrs, a)
		return true
	})

	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	for _, a := range attrs {
		newRecord.AddAttrs(a)
	}

	return h.inner.Handle(ctx, newRecord)
}

// WithAttrs delega en el handler interno.
func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &RedactingHandler{
		inner:     h.inner.WithAttrs(attrs),
		sensitive: h.sensitive,
	}
}

// WithGroup delega en el handler interno.
func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{
		inner:     h.inner.WithGroup(name),
		sensitive: h.sensitive,
	}
}
