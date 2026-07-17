// Package handler contiene los handlers HTTP de la API REST.
// Usa el ServeMux mejorado de Go 1.22+ con pattern matching de metodo y path.
// No depende de frameworks externos: solo net/http estandar.
//
// Superficie GS Open API v2 (Machine Server = GS, Third Party = nosotros):
//
//	POST /order/qr|status|refund|refundStatus|complete|cancel
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/logging"
	"github.com/seba/vps-powermix/internal/service"
)

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

type DBPinger interface {
	PingContext(ctx context.Context) error
}

type Handler struct {
	orderSvc  OrderService
	refundSvc RefundService
	db        DBPinger
}

func New(orderSvc OrderService, refundSvc RefundService, db DBPinger) *Handler {
	return &Handler{orderSvc: orderSvc, refundSvc: refundSvc, db: db}
}

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

	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.Handle("GET /metrics", MetricsHandler())

	return logging.RequestIDMiddleware(metricsMiddleware(recoveryMiddleware(loggingMiddleware(mux))))
}

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

// Por ahora reusa CreateOrderRequest del service (PR-B alineara campos v2).
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req service.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGSErr(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	resp, err := h.orderSvc.CreateOrder(r.Context(), &req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeGSOK(w, resp)
}

type queryStatusBody struct {
	OrderNo      string `json:"orderNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
}

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
		writeError(w, r, err)
		return
	}
	writeGSOK(w, resp)
}

func (h *Handler) CompleteOrder(w http.ResponseWriter, r *http.Request) {
	var req service.CompleteOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGSErr(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	resp, err := h.orderSvc.CompleteOrder(r.Context(), &req)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeGSOK(w, resp)
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	var req service.CancelOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGSErr(w, http.StatusBadRequest, "JSON invalido")
		return
	}

	resp, err := h.orderSvc.CancelOrder(r.Context(), &req)
	if err != nil {
		writeError(w, r, err)
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
		writeError(w, r, err)
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
		writeError(w, r, err)
		return
	}
	writeGSOK(w, resp)
}

// PVSWebhook maneja POST /webhook/pvs — callback oficial PVS.
// Doc: POST {{HOST}}?qr.reference=ref body {status APPROVED|REJECTED, qrId, ...}.
func (h *Handler) PVSWebhook(w http.ResponseWriter, r *http.Request) {
	var req service.PVSWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON invalido")
		return
	}
	// Doc oficial: query qr.reference=MerchantQRref
	req.QueryReference = r.URL.Query().Get("qr.reference")

	if err := h.orderSvc.HandlePVSWebhook(r.Context(), &req); err != nil {
		logging.From(r.Context()).Error("pvs webhook error", "error", err)
		writeError(w, r, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, msg string) {
	// Errores no-GS (webhook/health) mantienen forma simple.
	if status == http.StatusBadRequest || status == http.StatusNotFound ||
		status == http.StatusConflict || status == http.StatusInternalServerError {
	}
	respondJSON(w, status, map[string]string{"error": msg})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
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
		logging.From(r.Context()).Error("error interno", "error", err)
		writeGS(w, http.StatusInternalServerError, 400, "error interno", nil)
	}
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logging.From(r.Context()).Error("panic recuperado", "path", r.URL.Path,
					"method", r.Method, "panic", rec)
				respondError(w, http.StatusInternalServerError, "error interno")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// shouldLogHTTPBody limita el body log a endpoints GS/PVS.
// /healthz y /metrics quedan fuera (ruido + sin valor de debug de negocio).
func shouldLogHTTPBody(path string) bool {
	return strings.HasPrefix(path, "/order/") || path == "/webhook/pvs"
}

// bodyCaptureWriter copia el response body al buffer sin romper el write real.
type bodyCaptureWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (w *bodyCaptureWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *bodyCaptureWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	_, _ = w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logging.From(r.Context())
		path := r.URL.Path

		// healthz / metrics: log liviano, sin body
		if !shouldLogHTTPBody(path) {
			log.Info("http.request",
				"method", r.Method,
				"path", path,
				"remote", r.RemoteAddr,
			)
			next.ServeHTTP(w, r)
			return
		}

		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			log.Warn("http.request body read error",
				"error", err, "method", r.Method, "path", path)
			reqBody = nil
		}
		_ = r.Body.Close()
		// Handler sigue pudiendo leer el body.
		r.Body = io.NopCloser(bytes.NewReader(reqBody))

		reqAttrs := []any{
			"method", r.Method,
			"path", path,
			"remote", r.RemoteAddr,
		}
		if body, ok := logging.FormatBodyForLog(reqBody); ok {
			reqAttrs = append(reqAttrs, "body", body)
		}
		log.Info("http.request", reqAttrs...)

		cw := &bodyCaptureWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(cw, r)

		resAttrs := []any{
			"method", r.Method,
			"path", path,
			"status", cw.status,
		}
		if body, ok := logging.FormatBodyForLog(cw.buf.Bytes()); ok {
			resAttrs = append(resAttrs, "body", body)
		}
		log.Info("http.response", resAttrs...)
	})
}
