// Package timeutil centraliza la zona horaria de la app (Argentina).
package timeutil

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const DefaultLocation = "America/Argentina/Buenos_Aires"

var (
	loadOnce sync.Once
	loc      *time.Location
	loadErr  error
)

// Location devuelve America/Argentina/Buenos_Aires (carga lazy, panic si falta tzdata).
func Location() *time.Location {
	loadOnce.Do(func() {
		loc, loadErr = time.LoadLocation(DefaultLocation)
	})
	if loadErr != nil {
		panic(fmt.Sprintf("cargando timezone %s: %v", DefaultLocation, loadErr))
	}
	return loc
}

// Now es time.Now() en la zona de la app (mismo instante, reloj de pared ARG).
func Now() time.Time {
	return time.Now().In(Location())
}

// Format formaliza t en reloj ARG: yyyy-MM-dd HH:mm:ss (sin offset).
func Format(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(Location()).Format("2006-01-02 15:04:05")
}

// WithDSNTimezone agrega timezone=America/Argentina/Buenos_Aires al DSN
// (lib/pq aplica el parámetro a cada conexión del pool).
func WithDSNTimezone(dsn string) string {
	if dsn == "" || strings.Contains(dsn, "timezone=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "timezone=" + DefaultLocation
}
