package domain

import (
	"errors"
)

// Errores tipados del dominio. Los consumers (service, handler) usan
// errors.Is() para decidir codigos HTTP o flujos alternativos.
var (
	ErrOrderNotFound        = errors.New("orden no encontrada")
	ErrOrderNotCancellable  = errors.New("la orden no se puede cancelar en su estado actual")
	ErrOrderNotRefundable   = errors.New("la orden no se puede reembolsar en su estado actual")
	ErrRefundNotFound       = errors.New("reembolso no encontrado")
	ErrDuplicateOrder       = errors.New("orden duplicada detectada por ventana de dedup")
	ErrInvalidAmount        = errors.New("monto invalido (debe ser mayor a cero)")
	ErrInvalidTransition    = errors.New("transicion de estado invalida")
	ErrPVSServiceError      = errors.New("error en el servicio de PVS")
	ErrGSServiceError       = errors.New("error en el servicio de GS")
	ErrIdempotencyViolation = errors.New("violacion de idempotencia")
	ErrInvalidInput         = errors.New("entrada invalida")
)

// PVSBusinessError representa un 4xx devuelto por PVS que es culpa del cliente
// (monto invalido, validacion, etc). El handler lo propaga al cliente GS con su
// mensaje legible. Los 5xx y errores de red NO son de este tipo: quedan como
// error interno para no exponer detalles de infraestructura.
type PVSBusinessError struct {
	StatusCode int    // codigo HTTP que devolvio PVS (4xx)
	Code       string // code del envelope PVS (E_007)
	Message    string // message del envelope PVS ("Monto invalido")
}

func (e *PVSBusinessError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}
