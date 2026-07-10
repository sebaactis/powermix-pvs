// Package handler contiene los handlers HTTP de la API REST.
// Usa el ServeMux mejorado de Go 1.22+ con pattern matching de metodo y path.
// No depende de frameworks externos: solo net/http estandar.
//
// Superficie GS Open API v2 (Machine Server = GS, Third Party = nosotros):
//
//	POST /order/qr|status|refund|refundStatus|complete|cancel
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

// OrderService es lo que el handler necesita de OrderService.
type OrderService interface {
	CreateOrder(ctx context.Context, req *service.CreateOrderRequest) (*service.CreateOrderResponse, error)
	QueryStatus(ctx context.Context, req *service.QueryStatusRequest) (*service.QueryStatusResponse, error)
	CompleteOrder(ctx context.Context, req *service.CompleteOrderRequest) (*service.CompleteOrderResponse, error)
	CancelOrder(ctx context.Context, req *service.CancelOrderRequest) (*service.CancelOrderResponse, error)
	HandlePVSWebhook(ctx context.Context, req *service.PVSWebhookRequest) error
}

// RefundService es lo que el handler necesita de RefundService.
type RefundService interface {
	Refund(ctx context.Context, req *service.RefundRequest) (*service.RefundResponse, error)
	RefundStatus(ctx context.Context, req *service.RefundStatusRequest) (*service.RefundStatusResponse, error)
}

// DBPinger es la unica dependencia de DB que necesita el handler.
type DBPinger interface {
	PingContext(ctx context.Context) error
}

// Handler agrupa todos los endpoints HTTP.
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

	// GS Open API v2 (dashboard URLs)
	mux.HandleFunc("POST /order/qr", h.CreateOrder)
	mux.HandleFunc("POST /order/status", h.QueryStatus)
	mux.HandleFunc("POST /order/refund", h.Refund)
	mux.HandleFunc("POST /order/refundStatus", h.RefundStatus)
	mux.HandleFunc("POST /order/complete", h.CompleteOrder)
	mux.HandleFunc("POST /order/cancel", h.CancelOrder)

	// PVS webhook (sin cambios de contrato)
	mux.HandleFunc("POST /webhook/pvs", h.PVSWebhook)

	// Ops
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.Handle("GET /metrics", MetricsHandler())

	return metricsMiddleware(recoveryMiddleware(loggingMiddleware(mux)))
}

// --- Envelope GS v2 ---

// gsEnvelope es el cuerpo estandar hacia/desde GS: {code,msg,data}.
type gsEnvelope struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func writeGS(w http.ResponseWriter, httpStatus, code int, msg string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(gsEnvelope{Code: code, Msg: msg, Data: data})
}

func writeGSOK(w http.ResponseWriter, data interface{}) {
	writeGS(w, http.StatusOK, 200, "success", data)
}

func writeGSErr(w http.ResponseWriter, httpStatus int, msg string) {
	writeGS(w, httpStatus, 400, msg, nil)
}

// --- GS endpoints ---

// CreateOrder maneja POST /order/qr.
// Por ahora reusa CreateOrderRequest del service (PR-B alineara campos v2).
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req service.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGSErr(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	resp, err := h.orderSvc.CreateOrder(r.Context(), &req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeGSOK(w, resp)
}

// queryStatusBody es el body de POST /order/status.
type queryStatusBody struct {
	OrderNo      string `json:"orderNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
}

// QueryStatus maneja POST /order/status.
func (h *Handler) QueryStatus(w http.ResponseWriter, r *http.Request) {
	var body queryStatusBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeGSErr(w, http.StatusBadRequest, "JSON invalido")
		return
	}
	if body.OrderNo == "" {
		writeGSErr(w, http.StatusBadRequest, "orderNo es obligatorio")
		return
	}
	if body.ThirdOrderNo == "" {
		writeGSErr(w, http.StatusBadRequest, "thirdOrderNo es obligatorio")
		return
	}

	resp, err := h.orderSvc.QueryStatus(r.Context(), &service.QueryStatusRequest{
		OrderNo:      body.OrderNo,
		ThirdOrderNo: body.ThirdOrderNo,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeGSOK(w, resp)
}

// CompleteOrder maneja POST /order/complete.
func (h *Handler) CompleteOrder(w http.ResponseWriter, r *http.Request) {
	var req service.CompleteOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGSErr(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	resp, err := h.orderSvc.CompleteOrder(r.Context(), &req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeGSOK(w, resp)
}

// CancelOrder maneja POST /order/cancel.
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	var req service.CancelOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGSErr(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	resp, err := h.orderSvc.CancelOrder(r.Context(), &req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeGSOK(w, resp)
}

// Refund maneja POST /order/refund (thin: body con thirdOrderNo + refund fields).
func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	var req service.RefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGSErr(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	resp, err := h.refundSvc.Refund(r.Context(), &req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeGSOK(w, resp)
}

// RefundStatus maneja POST /order/refundStatus.
func (h *Handler) RefundStatus(w http.ResponseWriter, r *http.Request) {
	var req service.RefundStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGSErr(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	resp, err := h.refundSvc.RefundStatus(r.Context(), &req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeGSOK(w, resp)
}

// --- PVS webhook ---

// PVSWebhook maneja POST /webhook/pvs (contrato PVS sin cambios).
func (h *Handler) PVSWebhook(w http.ResponseWriter, r *http.Request) {
	var req service.PVSWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	if err := h.orderSvc.HandlePVSWebhook(r.Context(), &req); err != nil {
		slog.Error("pvs webhook error", "error", err)
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

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, msg string) {
	// Errores no-GS (webhook/health) mantienen forma simple.
	// Errores de endpoints GS usan writeGSErr/writeError.
	if status == http.StatusBadRequest || status == http.StatusNotFound ||
		status == http.StatusConflict || status == http.StatusInternalServerError {
		// Si el caller es GS path, writeError ya usa envelope.
	}
	respondJSON(w, status, map[string]string{"error": msg})
}

// writeError mapea errores del dominio a envelope GS (code 400 + HTTP acorde).
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrOrderNotFound),
		errors.Is(err, domain.ErrRefundNotFound):
		writeGSErr(w, http.StatusNotFound, err.Error())

	case errors.Is(err, domain.ErrOrderNotRefundable),
		errors.Is(err, domain.ErrOrderNotCancellable),
		errors.Is(err, domain.ErrInvalidTransition):
		writeGSErr(w, http.StatusConflict, err.Error())

	case errors.Is(err, domain.ErrInvalidInput),
		errors.Is(err, domain.ErrInvalidAmount):
		writeGSErr(w, http.StatusBadRequest, err.Error())

	default:
		slog.Error("error interno", "error", err)
		writeGS(w, http.StatusInternalServerError, 400, "error interno", nil)
	}
}

// --- Middleware ---

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

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("request", "method", r.Method, "path", r.URL.Path,
			"remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
