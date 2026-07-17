// Package keepalive implementa un bucle de self-ping periodico hacia una
// URL publica. Su objetivo es evitar que plataformas con spin-down por
// inactividad (ej. Render free tier) duerman el servicio por falta de
// trafico entrante.
//
// Importante: el ping debe apuntar a la URL PUBLICA del servicio (la que
// atraviesa el router de la plataforma), no a localhost. Un ping a
// 127.0.0.1 no atraviesa el router y no sirve para evitar el spin-down.
package keepalive

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Options configura el bucle de keepalive.
type Options struct {
	URL      string        // URL publica a la que pegarle. Vacio = no-op.
	Interval time.Duration // frecuencia del ping. <= 0 = no-op.
	Client   *http.Client  // opcional; si es nil usa uno con timeout de 10s.
	Logger   *slog.Logger  // opcional; si es nil usa slog.Default().
}

// Run bloquea hasta que ctx se cancela, haciendole un GET a opts.URL cada
// opts.Interval. Nunca panic: los errores de red o status no-2xx se
// loggean como warn y el bucle continua.
//
// Si opts.URL es vacio o opts.Interval <= 0, retorna de inmediato (no-op),
// lo que permite llamarla incondicionalmente desde main sin chequeos previos.
func Run(ctx context.Context, opts Options) {
	if opts.URL == "" || opts.Interval <= 0 {
		return
	}

	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	log.Info("keepalive iniciado", "url", opts.URL, "interval", opts.Interval)

	for {
		select {
		case <-ctx.Done():
			log.Info("keepalive detenido")
			return
		case <-ticker.C:
			ping(ctx, client, opts.URL, log)
		}
	}
}

// ping ejecuta un unico GET a url usando client y loggea el resultado.
// Usa el contexto recibido para cancelarse junto al bucle.
func ping(ctx context.Context, client *http.Client, url string, log *slog.Logger) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Warn("keepalive: no se pudo construir el request", "url", url, "error", err)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Warn("keepalive: error de ping", "url", url, "error", err)
		return
	}
	// Se drena el body antes de cerrarlo para que la conexion vuelva al
	// pool del cliente HTTP y se reutilice en el siguiente ping.
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Debug("keepalive ok", "url", url, "status", resp.StatusCode)
		return
	}
	log.Warn("keepalive: status inesperado",
		"url", url, "status", fmt.Sprintf("%d %s", resp.StatusCode, resp.Status))
}
