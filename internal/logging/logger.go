package logging

import (
	"context"
	"log/slog"
)

// Si hay request_id: logger = slog.Default().With("request_id", id).
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
