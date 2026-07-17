package logging

import "sync/atomic"

// Config runtime de body logging (seteada desde main con env).
// Default: enabled=true, max=DefaultMaxBodyLogBytes.
var (
	httpBodyLogging  atomic.Bool
	httpBodyMaxBytes atomic.Int64
)

func init() {
	httpBodyLogging.Store(true)
	httpBodyMaxBytes.Store(int64(DefaultMaxBodyLogBytes))
}

// ConfigureHTTPBodyLogging activa/desactiva el log de bodies y el max de bytes.
// maxBytes <= 0 usa DefaultMaxBodyLogBytes.
func ConfigureHTTPBodyLogging(enabled bool, maxBytes int) {
	httpBodyLogging.Store(enabled)
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodyLogBytes
	}
	httpBodyMaxBytes.Store(int64(maxBytes))
}

// HTTPBodyLoggingEnabled reporta si hay que loguear bodies.
func HTTPBodyLoggingEnabled() bool {
	return httpBodyLogging.Load()
}

// HTTPBodyMaxBytes devuelve el tope actual de bytes para SanitizeBody.
func HTTPBodyMaxBytes() int {
	return int(httpBodyMaxBytes.Load())
}

// FormatBodyForLog sanitiza el body si el flag esta ON.
// Si esta OFF devuelve "" y el caller no deberia agregar el atributo body
// (o puede chequear HTTPBodyLoggingEnabled primero).
func FormatBodyForLog(raw []byte) (body string, ok bool) {
	if !HTTPBodyLoggingEnabled() {
		return "", false
	}
	return SanitizeBody(raw, HTTPBodyMaxBytes()), true
}
