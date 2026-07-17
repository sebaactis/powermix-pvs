// Package config gestiona la configuracion del servicio mediante
// variables de entorno (12-factor app). Load() valida campos requeridos
// al arrancar, asi el programa falla rapido si falta algo critico.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config es la unica fuente de verdad para toda la configuracion
// del servicio. Cargala con Load(); el valor cero no es valido.
type Config struct {
	HTTPAddr string // direccion donde escucha el servidor HTTP

	DatabaseURL string // DSN de PostgreSQL

	GSBaseURL      string        // URL base de la API de GS
	GSKey          string        // key del header de autenticacion
	GSSecret       string        // secret compartido para firmar key-md5
	GSReplayWindow time.Duration // ventana de tolerancia para replay attacks

	PVSBaseURL       string        // URL base de la API de PVS
	PVSClientID      string        // client_id para OAuth2
	PVSClientSecret  string        // client_secret para OAuth2
	PVSCallbackURL   string        // URL donde PVS nos envia el webhook
	PVSNotifyTimeout time.Duration // timeout para llamadas a PVS

	QRExpiry time.Duration // tiempo de validez del QR desde su generacion

	ReconcilerInterval  time.Duration // cada cuanto ejecuta el reconciler
	ReconcilerBatchSize int           // maximo de ordenes por lote
	ReconcilerEnabled   bool          // true = arranca el worker al iniciar

	// Keepalive evita que plataformas con spin-down por inactividad
	// (ej. Render free tier) duerman el servicio. Pega un GET a KeepaliveURL
	// cada KeepaliveInterval. Queda deshabilitado si KeepaliveURL es vacio.
	KeepaliveURL      string        // URL publica a la que se le hace self-ping
	KeepaliveInterval time.Duration // cada cuanto ejecuta el keepalive

	GSEnabled bool // true = procesa pedidos entrantes de GS

	LogLevel  string // nivel de log: debug, info, warn, error
	LogFormat string // formato: text o json

	// LogHTTPBodies controla si se loguean bodies HTTP (GS/PVS in/out).
	// Default true: util para debug de integracion. Apagar con LOG_HTTP_BODIES=false.
	LogHTTPBodies       bool
	LogHTTPBodyMaxBytes int // tope del body sanitizado en logs (0 = default del paquete logging)

	SyncLogRetentionDays   int           // dias que se conservan los logs de sync
	LockAcquisitionTimeout time.Duration // timeout maximo para adquirir un lock de fila
}

// Load lee las variables de entorno, asigna defaults, valida los campos
// requeridos y devuelve un Config listo para usar.
func Load() (*Config, error) {
	getEnv := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}

	getEnvInt := func(key string, fallback int) int {
		if v := os.Getenv(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
		return fallback
	}

	getEnvDurSec := func(key string, fallbackSec int) time.Duration {
		return time.Duration(getEnvInt(key, fallbackSec)) * time.Second
	}

	getEnvBool := func(key string, fallback bool) bool {
		switch strings.ToLower(os.Getenv(key)) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		default:
			return fallback
		}
	}

	cfg := &Config{
		// HTTP
		HTTPAddr: getEnv("HTTP_ADDR", ":8080"),

		DatabaseURL: getEnv("DATABASE_URL", ""),

		// GS
		GSBaseURL:      getEnv("GS_BASE_URL", ""),
		GSKey:          getEnv("GS_KEY", ""),
		GSSecret:       getEnv("GS_SECRET", ""),
		GSReplayWindow: getEnvDurSec("GS_REPLAY_WINDOW_SEC", 300),

		// PVS
		PVSBaseURL:       getEnv("PVS_BASE_URL", "https://api01.pvssa.com.ar"),
		PVSClientID:      getEnv("PVS_CLIENT_ID", ""),
		PVSClientSecret:  getEnv("PVS_CLIENT_SECRET", ""),
		PVSCallbackURL:   getEnv("PVS_CALLBACK_URL", ""),
		PVSNotifyTimeout: getEnvDurSec("PVS_NOTIFY_TIMEOUT_SEC", 10),

		QRExpiry: getEnvDurSec("QR_EXPIRY_SEC", 180),

		ReconcilerInterval:  getEnvDurSec("RECONCILER_INTERVAL_SEC", 60),
		ReconcilerBatchSize: getEnvInt("RECONCILER_BATCH_SIZE", 200),
		ReconcilerEnabled:   getEnvBool("RECONCILER_ENABLED", false),

		// Keepalive: por defecto arma la URL desde RENDER_EXTERNAL_URL
		// (Render la inyecta automaticamente, zero-config). Si KEEPALIVE_URL
		// esta seteada, tiene prioridad (override manual / fuera de Render).
		KeepaliveURL:      resolverKeepaliveURL(getEnv("KEEPALIVE_URL", ""), os.Getenv("RENDER_EXTERNAL_URL")),
		KeepaliveInterval: getEnvDurSec("KEEPALIVE_INTERVAL_SEC", 30),

		GSEnabled: getEnvBool("GS_PVS_ENABLED", false),

		LogLevel:  getEnv("LOG_LEVEL", "info"),
		LogFormat: getEnv("LOG_FORMAT", "json"),

		// Body logging: default ON. LOG_HTTP_BODY_MAX_BYTES 0 → default 8KiB en logging pkg.
		LogHTTPBodies:       getEnvBool("LOG_HTTP_BODIES", true),
		LogHTTPBodyMaxBytes: getEnvInt("LOG_HTTP_BODY_MAX_BYTES", 0),

		SyncLogRetentionDays:   getEnvInt("SYNC_LOG_RETENTION_DAYS", 30),
		LockAcquisitionTimeout: getEnvDurSec("LOCK_ACQUISITION_TIMEOUT_SEC", 5),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// resolverKeepaliveURL decide la URL publica a la que el keepalive le
// pega. Prioridad:
//   1. keepaliveURL (env KEEPALIVE_URL) si esta seteada — override manual.
//   2. renderExternalURL (env RENDER_EXTERNAL_URL, inyectada por Render) + "/healthz".
//   3. cadena vacia (keepalive deshabilitado).
//
// Normaliza el trailing slash de la base para no duplicarlo al agregar
// "/healthz".
func resolverKeepaliveURL(keepaliveURL, renderExternalURL string) string {
	if keepaliveURL != "" {
		return keepaliveURL
	}
	if renderExternalURL == "" {
		return ""
	}
	return strings.TrimRight(renderExternalURL, "/") + "/healthz"
}

func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("error cargando configuracion: %v", err))
	}
	return cfg
}

// validate revisa que todos los campos REQUERIDOS tengan valor.
// No valida logica de negocio (eso va en cada paquete).
func (c *Config) validate() error {
	var missing []string

	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if c.GSBaseURL == "" {
		missing = append(missing, "GS_BASE_URL")
	}
	if c.GSKey == "" {
		missing = append(missing, "GS_KEY")
	}
	if c.GSSecret == "" {
		missing = append(missing, "GS_SECRET")
	}
	if c.PVSClientID == "" {
		missing = append(missing, "PVS_CLIENT_ID")
	}
	if c.PVSClientSecret == "" {
		missing = append(missing, "PVS_CLIENT_SECRET")
	}
	if c.PVSCallbackURL == "" {
		missing = append(missing, "PVS_CALLBACK_URL")
	}

	if len(missing) > 0 {
		return fmt.Errorf("variables de entorno requeridas faltantes: %s",
			strings.Join(missing, ", "))
	}
	return nil
}

// ValidateLogLevel chequea que LogLevel sea uno de los valores aceptados.
// Se puede llamar despues de Load() en main() para fail-fast adicional.
func ValidateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("LOG_LEVEL invalido: %q (opciones: debug, info, warn, error)", level)
	}
}

func ValidateLogFormat(format string) error {
	switch format {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("LOG_FORMAT invalido: %q (opciones: text, json)", format)
	}
}

// ValidateReglasNegocio valida reglas de negocio en la configuracion.
// Ejemplo: ventanas de tiempo muy chicas pueden causar problemas.
func (c *Config) ValidateReglasNegocio() error {
	var errs []string

	if c.GSReplayWindow < 30*time.Second {
		errs = append(errs,
			"GS_REPLAY_WINDOW_SEC no puede ser menor a 30 segundos")
	}
	if c.QRExpiry < 30*time.Second {
		errs = append(errs,
			"QR_EXPIRY_SEC no puede ser menor a 30 segundos")
	}
	if c.ReconcilerBatchSize < 1 || c.ReconcilerBatchSize > 1000 {
		errs = append(errs,
			"RECONCILER_BATCH_SIZE debe estar entre 1 y 1000")
	}
	if c.SyncLogRetentionDays < 1 {
		errs = append(errs,
			"SYNC_LOG_RETENTION_DAYS no puede ser menor a 1")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
