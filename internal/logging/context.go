// Package logging provee propagación de IDs de correlación (request_id, scan_id)
// vía context.Context, generación de identificadores únicos, acceso al logger
// estructurado y middleware HTTP para inyectar el request ID.
package logging

import "context"

// ctxKey es el tipo no exportado para evitar colisiones en context.
type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyScanID
)

// WithRequestID guarda el request_id como string liviano en el contexto.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestIDFrom extrae el request_id del contexto; "" si ausente.
func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(keyRequestID).(string)
	return v
}

// WithScanID guarda el scan_id como string liviano en el contexto.
func WithScanID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyScanID, id)
}

// ScanIDFrom extrae el scan_id del contexto; "" si ausente.
func ScanIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(keyScanID).(string)
	return v
}
