package logging

import (
	"context"
	"log/slog"
)

// From extrae el logger del contexto.
// Si hay request_id: logger = slog.Default().With("request_id", id).
// Si hay scan_id: logger = logger.With("scan_id", id).
// Si ninguno: devuelve slog.Default().
func From(ctx context.Context) *slog.Logger {
	logger := slog.Default()

	if reqID := RequestIDFrom(ctx); reqID != "" {
		logger = logger.With("request_id", reqID)
	}
	if scanID := ScanIDFrom(ctx); scanID != "" {
		logger = logger.With("scan_id", scanID)
	}

	return logger
}
