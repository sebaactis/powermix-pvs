// Package handler contiene los handlers HTTP de la API REST.
// Usa el ServeMux mejorado de Go 1.22+ con pattern matching de metodo y path.
// No depende de frameworks externos: solo net/http estandar.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/service"
)

// --- Interfaces minimas para testabilidad ---
// Definimos solo los metodos que el handler necesita, asi los tests
// pueden usar mocks sin necesidad de los servicios reales.

// OrderService es lo que el handler necesita de OrderService.
type OrderService interface {
	CreateOrder(ctx context.Context, req *service.CreateOrderRequest) (*service.CreateOrderResponse, error)
	QueryStatus(ctx context.Context, req *service.QueryStatusRequest) (*service.QueryStatusResponse, error)
	CompleteOrder(ctx context.Context, thirdOrderNo string) error
	CancelOrder(ctx context.Context, thirdOrderNo string) error
	HandlePVSWebhook(ctx context.Context, req *service.PVSWebhookRequest) error
}

// RefundService es lo que el handler necesita de RefundService.
type RefundService interface {
	Refund(ctx context.Context, req *service.RefundRequest) (*service.RefundResponse, error)
}

// DBPinger es la unica dependencia de DB que necesita el handler.
// *sqlx.DB la satisface implicitamente; tambien los mocks en tests.
type DBPinger interface {
	PingContext(ctx context.Context) error
}

// Handler agrupa todos los endpoints HTTP.
// Depende de interfaces, no de implementaciones concretas.
type Handler struct {
	orderSvc  OrderService
	refundSvc RefundService
	db        DBPinger
}

// New crea un Handler listo para usar.
func New(orderSvc OrderService, refundSvc RefundService, db DBPinger) *Handler {
	return &Handler{orderSvc: orderSvc, refundSvc: refundSvc, db: db}
}

// Routes construye el mux con todas las rutas y middlewares.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	// GS endpoints
	mux.HandleFunc("POST /api/v1/orders", h.CreateOrder)
	mux.HandleFunc("GET /api/v1/orders/{orderNo}", h.QueryStatus)
	mux.HandleFunc("POST /api/v1/orders/{orderNo}/complete", h.CompleteOrder)
	mux.HandleFunc("POST /api/v1/orders/{orderNo}/cancel", h.CancelOrder)
	mux.HandleFunc("POST /api/v1/orders/{orderNo}/refund", h.Refund)

	// PVS webhook
	mux.HandleFunc("POST /webhook/pvs", h.PVSWebhook)

	// Health check
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.Handle("GET /metrics", MetricsHandler())

	return metricsMiddleware(recoveryMiddleware(loggingMiddleware(mux)))
}

// --- GS endpoints ---

// CreateOrder maneja POST /api/v1/orders (GS crea un pedido).
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req service.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	resp, err := h.orderSvc.CreateOrder(r.Context(), &req)
	if err != nil {
		writeError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// QueryStatus maneja GET /api/v1/orders/{orderNo} (GS consulta estado).
func (h *Handler) QueryStatus(w http.ResponseWriter, r *http.Request) {
	orderNo := r.PathValue("orderNo")
	if orderNo == "" {
		respondError(w, http.StatusBadRequest, "orderNo es obligatorio")
		return
	}

	resp, err := h.orderSvc.QueryStatus(r.Context(), &service.QueryStatusRequest{
		ThirdOrderNo: orderNo,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// CompleteOrder maneja POST /api/v1/orders/{orderNo}/complete (GS entrega producto).
func (h *Handler) CompleteOrder(w http.ResponseWriter, r *http.Request) {
	orderNo := r.PathValue("orderNo")
	if orderNo == "" {
		respondError(w, http.StatusBadRequest, "orderNo es obligatorio")
		return
	}

	if err := h.orderSvc.CompleteOrder(r.Context(), orderNo); err != nil {
		writeError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// CancelOrder maneja POST /api/v1/orders/{orderNo}/cancel (GS cancela orden).
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	orderNo := r.PathValue("orderNo")
	if orderNo == "" {
		respondError(w, http.StatusBadRequest, "orderNo es obligatorio")
		return
	}

	if err := h.orderSvc.CancelOrder(r.Context(), orderNo); err != nil {
		writeError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Refund maneja POST /api/v1/orders/{orderNo}/refund (GS pide reembolso).
// Toma el orderNo del path y el resto de los datos del body.
func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	var req service.RefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON invalido")
		return
	}
	// Path param sobreescribe ThirdOrderNo si vino en el body
	req.ThirdOrderNo = r.PathValue("orderNo")

	resp, err := h.refundSvc.Refund(r.Context(), &req)
	if err != nil {
		writeError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// --- PVS webhook ---

// PVSWebhook maneja POST /webhook/pvs (PVS notifica cambio de estado).
// Devuelve 200 siempre que el request sea valido (incluso no-op por idempotencia).
// En caso de error interno devuelve 500 para que PVS reintente.
func (h *Handler) PVSWebhook(w http.ResponseWriter, r *http.Request) {
	var req service.PVSWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	if err := h.orderSvc.HandlePVSWebhook(r.Context(), &req); err != nil {
		slog.Error("pvs webhook error", "error", err)
		// Errores comunes: qrId vacio (400), orden no encontrada (404),
		// stateId invalido (400), DB error (500).
		// Para simplificar, los que no son de validacion van como 500 (reintentar).
		writeError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Health check ---

// Healthz maneja GET /healthz. Verifica que la DB responda.
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	if err := h.db.PingContext(r.Context()); err != nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Helpers ---

// respondJSON escribe una respuesta JSON con el status code indicado.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// respondError escribe un error simple como JSON.
func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// writeError mapea errores del dominio a codigos HTTP.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrOrderNotFound),
		errors.Is(err, domain.ErrRefundNotFound):
		respondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, domain.ErrOrderNotRefundable),
		errors.Is(err, domain.ErrOrderNotCancellable),
		errors.Is(err, domain.ErrInvalidTransition):
		respondError(w, http.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrInvalidInput),
		errors.Is(err, domain.ErrInvalidAmount):
		respondError(w, http.StatusBadRequest, err.Error())

	default:
		slog.Error("error interno", "error", err)
		respondError(w, http.StatusInternalServerError, "error interno")
	}
}

// --- Middleware ---

// recoveryMiddleware atrapa panics y responde con 500.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recuperado", "path", r.URL.Path,
					"method", r.Method, "panic", rec)
				respondError(w, http.StatusInternalServerError, "error interno")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logea cada request entrante.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("request", "method", r.Method, "path", r.URL.Path,
			"remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
