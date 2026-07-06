// Command server es el punto de entrada del servicio vps-powermix.
// Wirea config -> DB -> repos -> clients -> services -> handler -> server.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	gsclient "github.com/seba/vps-powermix/internal/client/gs"
	pvsclient "github.com/seba/vps-powermix/internal/client/pvs"
	"github.com/seba/vps-powermix/internal/config"
	"github.com/seba/vps-powermix/internal/handler"
	"github.com/seba/vps-powermix/internal/reconciler"
	"github.com/seba/vps-powermix/internal/service"
	"github.com/seba/vps-powermix/internal/store"
)

func main() {
	// 1. Config
	cfg := config.MustLoad()

	// 2. Logger con redaccion de datos sensibles
	logHandler := handler.NewRedactingHandler(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: parseLogLevel(cfg.LogLevel),
		}),
		"token", "secret", "password", "api_key",
	)
	slog.SetDefault(slog.New(logHandler))
	slog.Info("iniciando servidor", "addr", cfg.HTTPAddr, "gs_enabled", cfg.GSEnabled)

	// 3. Base de datos
	db, err := sqlx.Connect("postgres", cfg.DatabaseURL)
	if err != nil {
		slog.Error("conectando a base de datos", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	slog.Info("conexion a base de datos establecida")

	// 4. Repositorios
	orderRepo := store.NewPostgresOrderRepository(db)
	refundRepo := store.NewPostgresRefundRepository(db)
	syncLogRepo := store.NewPostgresSyncLogRepo(db)
	idempStore := store.NewPostgresIdempotencyStore(db)
	reconcilerStore := store.NewPostgresReconcilerStore(db)

	_ = idempStore // reservado para uso futuro

	// 5. Clientes externos
	pvsClient := pvsclient.New(cfg.PVSBaseURL, cfg.PVSClientID, cfg.PVSClientSecret,
		pvsclient.ConRateLimit(50, 50))
	_ = gsclient.New(cfg.GSBaseURL, cfg.GSKey, cfg.GSSecret) // reservado para PR5

	// 6. Servicios
	orderSvc := service.NewOrderService(orderRepo, pvsClient, syncLogRepo,
		service.ConQRExpiry(cfg.QRExpiry))
	refundSvc := service.NewRefundService(orderRepo, pvsClient, refundRepo, syncLogRepo)

	// 7. HTTP handler
	h := handler.New(orderSvc, refundSvc, db)
	mux := h.Routes()

	// 8. Reconciler (opcional, ver RECONCILER_ENABLED)
	if cfg.ReconcilerEnabled {
		rec := reconciler.New(reconcilerStore, orderRepo, pvsClient,
			cfg.ReconcilerInterval)
		go rec.Run(context.Background())
		slog.Info("reconciler iniciado en background",
			"interval", cfg.ReconcilerInterval)
	}

	// 9. Servidor HTTP
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 10. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("servidor HTTP escuchando", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("error en servidor HTTP", "error", err)
			os.Exit(1)
		}
	}()

	// Esperar senial de terminacion
	sig := <-quit
	slog.Info("senal recibida, apagando servidor", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("error al apagar servidor", "error", err)
	}
	slog.Info("servidor apagado correctamente")
}

// parseLogLevel convierte string a slog.Level.
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
