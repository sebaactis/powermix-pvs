package domain

import "errors"

// Errores tipados del dominio. Los consumers (service, handler) usan
// errors.Is() para decidir codigos HTTP o flujos alternativos.
var (
	ErrOrderNotFound       = errors.New("orden no encontrada")
	ErrOrderNotCancellable = errors.New("la orden no se puede cancelar en su estado actual")
	ErrOrderNotRefundable  = errors.New("la orden no se puede reembolsar en su estado actual")
	ErrRefundNotFound      = errors.New("reembolso no encontrado")
	ErrDuplicateOrder      = errors.New("orden duplicada detectada por ventana de dedup")
	ErrInvalidAmount       = errors.New("monto invalido (debe ser mayor a cero)")
	ErrInvalidTransition   = errors.New("transicion de estado invalida")
	ErrPVSServiceError     = errors.New("error en el servicio de PVS")
	ErrGSServiceError      = errors.New("error en el servicio de GS")
	ErrIdempotencyViolation = errors.New("violacion de idempotencia")
)
