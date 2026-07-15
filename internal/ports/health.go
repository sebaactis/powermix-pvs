package ports

import (
	"context"
	"time"
)

// HealthChecker define los chequeos de salud del servicio.
type HealthChecker interface {
	// PingDB verifica que la conexion a la base de datos responde.
	PingDB(ctx context.Context) error

	// CheckClockDrift mide la diferencia entre nuestro reloj y NTP.
	// Si la deriva supera los 5 minutos, todas las firmas key-md5
	// de GS van a fallar por el replay window.
	CheckClockDrift(ctx context.Context) (drift time.Duration, err error)
}
