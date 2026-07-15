// Package service contiene la lógica de negocio que coordina los
// puertos (repositorios y clientes). No sabe nada de HTTP ni SQL.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/logging"
	"github.com/seba/vps-powermix/internal/ports"
	"github.com/seba/vps-powermix/internal/timeutil"
)

// OrderService coordina la creación y ciclo de vida de una orden.
// Depende de interfaces (ports), no de implementaciones concretas.
type OrderService struct {
	orderRepo   ports.OrderRepository
	pvsClient   ports.PVSClient
	syncLogRepo ports.SyncLogRepo
	gsClient    ports.GSClient // notify outbound a GS (puede ser nil en tests)
	qrExpiry    time.Duration  // cuánto vive el QR (default 180s)
	dedupWindow time.Duration  // ventana de dedup (default 20s)
}

type Option func(*OrderService)

func ConQRExpiry(d time.Duration) Option {
	return func(s *OrderService) { s.qrExpiry = d }
}

func ConDedupWindow(d time.Duration) Option {
	return func(s *OrderService) { s.dedupWindow = d }
}

// NewOrderService crea un OrderService listo para usar.
// gs puede ser nil si no se necesita notify outbound (tests unitarios).
func NewOrderService(repo ports.OrderRepository, pvs ports.PVSClient,
	syncLog ports.SyncLogRepo, gs ports.GSClient, opts ...Option) *OrderService {
	s := &OrderService{
		orderRepo:   repo,
		pvsClient:   pvs,
		syncLogRepo: syncLog,
		gsClient:    gs,
		qrExpiry:    180 * time.Second,
		dedupWindow: 20 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// FormatGSTime formatea timestamps al estilo GS en reloj ARG: yyyy-MM-dd HH:mm:ss.
func FormatGSTime(t time.Time) string {
	return timeutil.Format(t)
}

type CreateOrderRequest struct {
	OrderNo     string `json:"orderNo"`     // serial de GS (requerido)
	ObjectID    string `json:"objectId"`    // SKU (opcional en v2)
	Subject     string `json:"subject"`     // nombre del producto (requerido)
	Attach      string `json:"attach"`      // deviceNo=...&deviceId=...
	TotalAmount string `json:"totalAmount"` // monto string ("150.00")
	NotifyURL   string `json:"notifyUrl"`   // callback de pago hacia GS
}

type CreateOrderResponse struct {
	QrURL        string `json:"qrUrl"`        // base64 del QR (PVS qrImage → qrUrl)
	OrderStatus  string `json:"orderStatus"`  // "1" = pendiente de pago
	ThirdOrderNo string `json:"thirdOrderNo"` // nuestro id
}

type QueryStatusRequest struct {
	OrderNo      string `json:"orderNo"`      // serial GS
	ThirdOrderNo string `json:"thirdOrderNo"` // nuestro id
}

type QueryStatusResponse struct {
	OrderNo      string `json:"orderNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
	OrderStatus  string `json:"orderStatus"` // "1"…"6"
}

type FlexibleStatus string

func (f *FlexibleStatus) UnmarshalJSON(b []byte) error {
	b = bytesTrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexibleStatus(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = FlexibleStatus(n.String())
	return nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

type CompleteOrderRequest struct {
	OrderNo        string         `json:"orderNo"`
	ThirdOrderNo   string         `json:"thirdOrderNo"`
	Success        bool           `json:"success"`
	OrderStatus    FlexibleStatus `json:"orderStatus"`
	OutStockStatus int            `json:"outStockStatus"`
	OutStockTime   string         `json:"outStockTime"`
}

type CompleteOrderResponse struct {
	OrderNo      string `json:"orderNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
	ReturnCode   string `json:"returnCode"`
	ReturnMsg    string `json:"returnMsg"`
}

type CancelOrderRequest struct {
	OrderNo      string         `json:"orderNo"`
	ThirdOrderNo string         `json:"thirdOrderNo"`
	OrderStatus  FlexibleStatus `json:"orderStatus"` // 0 = cancel
	Remark       string         `json:"remark"`
	CancelTime   string         `json:"cancelTime"`
}

type CancelOrderResponse struct {
	OrderNo      string `json:"orderNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
	ReturnCode   string `json:"returnCode"`
	ReturnMsg    string `json:"returnMsg"`
}

// PVSWebhookRequest es el callback oficial PVS (colección QR Integration).
// POST {{HOST}}?qr.reference=ref — body con status APPROVED|REJECTED.
type PVSWebhookRequest struct {
	Reference  string           `json:"reference"`
	Amount     float64          `json:"amount"`
	QrID       string           `json:"qrId"`
	TxEID      string           `json:"txeId"`
	Status     string           `json:"status"` // APPROVED | REJECTED
	NotifiedAt string           `json:"notified_at"`
	Payer      *PVSWebhookPayer `json:"payer,omitempty"`

	// Query oficial ?qr.reference=... (lo llena el handler, no el JSON).
	QueryReference string `json:"-"`

	// Solo tests/mock viejos hasta alinear mockpvs. NO es contrato callback real.
	StateID int `json:"stateId,omitempty"`
}

// PVSWebhookPayer es el bloque payer del callback oficial.
type PVSWebhookPayer struct {
	Name     string `json:"name"`
	IDType   string `json:"idType"`
	IDNumber string `json:"idNumber"`
}

// CreateOrder ejecuta POST /order/qr (GS Open API v2):
// 1. Validar orderNo, subject, totalAmount, notifyUrl
// 2. Idempotencia por gs_order_no si ya hay QR_SHOWN
// 3. Crear orden (third_order_no nuestro + gs_order_no + notify_url)
// 5. QR_SHOWN + respuesta {qrUrl, orderStatus:"1", thirdOrderNo}
func (s *OrderService) CreateOrder(ctx context.Context, req *CreateOrderRequest) (*CreateOrderResponse, error) {
	if req.OrderNo == "" {
		return nil, fmt.Errorf("%w: orderNo es obligatorio", domain.ErrInvalidInput)
	}
	if req.Subject == "" {
		return nil, fmt.Errorf("%w: subject es obligatorio", domain.ErrInvalidInput)
	}
	if req.NotifyURL == "" {
		return nil, fmt.Errorf("%w: notifyUrl es obligatorio", domain.ErrInvalidInput)
	}
	if req.TotalAmount == "" {
		return nil, fmt.Errorf("%w: totalAmount es obligatorio", domain.ErrInvalidInput)
	}
	montoCentavos, err := parseMonto(req.TotalAmount)
	if err != nil {
		return nil, fmt.Errorf("%w: monto invalido %q: %w", domain.ErrInvalidInput, req.TotalAmount, err)
	}
	if montoCentavos <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	deviceNo, deviceID := parseAttach(req.Attach)

	// Idempotencia: mismo orderNo de GS ya mostrado → mismo QR
	if existente, err := s.orderRepo.GetByGsOrderNo(ctx, req.OrderNo); err == nil && existente != nil {
		if existente.PvsQrImage != "" {
			return &CreateOrderResponse{
				QrURL:        existente.PvsQrImage,
				OrderStatus:  "1",
				ThirdOrderNo: existente.ThirdOrderNo,
			}, nil
		}
	}

	thirdOrderNo := generarThirdOrderNo()
	orden := &domain.Order{
		ThirdOrderNo: thirdOrderNo,
		GsOrderNo:    req.OrderNo,
		DeviceID:     deviceID,
		DeviceNo:     deviceNo,
		ObjectID:     req.ObjectID,
		PriceCents:   montoCentavos,
		NotifyURL:    req.NotifyURL,
		Status:       domain.OrderReceived,
		RequestID:    logging.RequestIDFrom(ctx),
	}
	if err := s.orderRepo.Create(ctx, orden); err != nil {
		return nil, fmt.Errorf("creando orden en DB: %w", err)
	}

	pvsResp, err := s.pvsClient.GenerateQR(ctx, &ports.PVSQRRequest{
		Amount:     montoCentavos,
		ExternalID: thirdOrderNo,
		Reference:  thirdOrderNo,
	})
	if err != nil {
		_ = s.orderRepo.UpdateStatus(ctx, thirdOrderNo, domain.OrderFailed)
		return nil, fmt.Errorf("generando QR en PVS: %w", err)
	}

	expira := time.Now().Add(s.qrExpiry)
	err = s.orderRepo.UpdateStatusAndFields(ctx, thirdOrderNo, domain.OrderQRShown, map[string]interface{}{
		"pvs_qr_id":       pvsResp.QrID,
		"pvs_qr_image":    pvsResp.QrImage,
		"qr_generated_at": time.Now(),
		"qr_expires_at":   expira,
	})
	if err != nil {
		return nil, fmt.Errorf("actualizando orden con QR: %w", err)
	}

	logging.From(ctx).Info("pvs.qr.created",
		"order_id", thirdOrderNo,
		"third_order_no", thirdOrderNo,
		"pvs_qr_id", pvsResp.QrID,
	)

	_ = s.syncLogRepo.Insert(ctx, &ports.SyncLogEntry{
		ThirdOrderNo: thirdOrderNo,
		Vendor:       "PVS",
		Direction:    "outbound",
		Endpoint:     "/qr/pvs",
		Method:       "POST",
	})

	// PVS qrImage → GS qrUrl (mismo base64)
	return &CreateOrderResponse{
		QrURL:        pvsResp.QrImage,
		OrderStatus:  "1",
		ThirdOrderNo: thirdOrderNo,
	}, nil
}

// QueryStatus responde POST /order/status (GS Open API v2).
// Requiere orderNo (GS) + thirdOrderNo (nuestro) y valida que el par coincida.
// No llama a PVS: devuelve el estado de nuestra DB mapeado a "1"…"6".
func (s *OrderService) QueryStatus(ctx context.Context, req *QueryStatusRequest) (*QueryStatusResponse, error) {
	if req.OrderNo == "" {
		return nil, fmt.Errorf("%w: orderNo es obligatorio", domain.ErrInvalidInput)
	}
	if req.ThirdOrderNo == "" {
		return nil, fmt.Errorf("%w: thirdOrderNo es obligatorio", domain.ErrInvalidInput)
	}

	orden, err := s.orderRepo.GetByThirdOrderNo(ctx, req.ThirdOrderNo)
	if err != nil {
		return nil, fmt.Errorf("buscando orden %q: %w", req.ThirdOrderNo, err)
	}
	if orden.GsOrderNo != req.OrderNo {
		return nil, fmt.Errorf("%w: orderNo no coincide con la orden", domain.ErrInvalidInput)
	}

	return &QueryStatusResponse{
		OrderNo:      orden.GsOrderNo,
		ThirdOrderNo: orden.ThirdOrderNo,
		OrderStatus:  strconv.Itoa(int(orden.Status.ToGSStatus())),
	}, nil
}

// HandlePVSWebhook procesa la notificación de pago de PVS.
// PVS nos avisa cuando el estado del QR cambia (aprobado, rechazado, etc).
//
// La idempotencia la da el state machine: si el estado ya fue aplicado,
// CanTransitionTo rechaza la transición y es un no-op silencioso.
// (Puede reenviar el mismo webhook varias veces.)
func (s *OrderService) HandlePVSWebhook(ctx context.Context, req *PVSWebhookRequest) error {
	// reference body o query ?qr.reference= (doc oficial).
	ref := req.Reference
	if ref == "" {
		ref = req.QueryReference
	}

	// 1. Buscar orden: qrId preferido; si no, reference (= thirdOrderNo al crear QR).
	var orden *domain.Order
	var err error
	switch {
	case req.QrID != "":
		orden, err = s.orderRepo.GetByPVSQrID(ctx, req.QrID)
		if err != nil {
			return fmt.Errorf("buscando orden por qrId %q: %w", req.QrID, err)
		}
	case ref != "":
		orden, err = s.orderRepo.GetByThirdOrderNo(ctx, ref)
		if err != nil {
			return fmt.Errorf("buscando orden por reference %q: %w", ref, err)
		}
	default:
		return fmt.Errorf("%w: qrId o reference obligatorio", domain.ErrInvalidInput)
	}

	// 2. Preferir status texto (callback real). Fallback stateId (tests/mock).
	pvsStatus, err := resolvePVSWebhookStatus(req)
	if err != nil {
		return err
	}
	nuevoEstado := pvsStatusToOrderStatus(pvsStatus, orden.Status)
	if nuevoEstado == "" {
		// IN_PROCESS: el pago sigue pendiente, nada que hacer.
		return nil
	}

	if !orden.Status.CanTransitionTo(nuevoEstado) {
		// Estado ya aplicado o transicion invalida. No-op silencioso.
		return nil
	}

	// 4. Actualizar con guarded update — el WHERE status = $estado_actual
	// protege contra races: si el reconciler cambio el estado entre el
	// paso 1 y este, el UPDATE afecta 0 filas y updated=false (no-op).
	fields := map[string]interface{}{}
	if nuevoEstado == domain.OrderPaymentConfirmed {
		fields["payment_confirmed_at"] = time.Now()
	}
	updated, err := s.orderRepo.UpdateStatusGuardedAndFields(ctx, orden.ThirdOrderNo,
		orden.Status, nuevoEstado, fields)
	if err != nil {
		return fmt.Errorf("actualizando orden a %s: %w", nuevoEstado, err)
	}
	if !updated {
		// Otra transaccion cambio el estado primero. No-op.
		return nil
	}

	// 5. Audit log (best-effort, no falla el flujo)
	_ = s.syncLogRepo.Insert(ctx, &ports.SyncLogEntry{
		ThirdOrderNo: orden.ThirdOrderNo,
		Vendor:       "PVS",
		Direction:    "inbound",
		Endpoint:     "/webhook/pvs",
		Method:       "POST",
	})

	// 6. Milestone trace: payment.confirmed con trazabilidad asincrona
	if nuevoEstado == domain.OrderPaymentConfirmed {
		logging.From(ctx).Info("payment.confirmed",
			"order_id", orden.ThirdOrderNo,
			"original_request_id", orden.RequestID,
		)
	}

	// 7. Notify outbound a GS (solo pago confirmado; best-effort)
	if nuevoEstado == domain.OrderPaymentConfirmed {
		orden.Status = domain.OrderPaymentConfirmed
		if orden.PaymentConfirmedAt.IsZero() {
			orden.PaymentConfirmedAt = time.Now()
		}
		s.NotifyPaymentIfNeeded(ctx, orden)
	}

	return nil
}

// NotifyPaymentIfNeeded avisa a GS el pago exitoso (orderStatus "2").
// Best-effort: errores se loguean y dejan gs_notified_at en null para retry.
// El reconciler reusa este metodo para reintentos de notify fallidos.
func (s *OrderService) NotifyPaymentIfNeeded(ctx context.Context, orden *domain.Order) {
	if s.gsClient == nil || orden == nil {
		return
	}
	if orden.NotifyURL == "" || !orden.GsNotifiedAt.IsZero() {
		return
	}
	if orden.Status != domain.OrderPaymentConfirmed {
		return
	}

	payTime := orden.PaymentConfirmedAt
	if payTime.IsZero() {
		payTime = time.Now()
	}
	orderTime := orden.CreatedAt
	if orderTime.IsZero() {
		orderTime = payTime
	}

	resp, err := s.gsClient.NotifyPayment(ctx, &ports.GSNotifyPaymentRequest{
		NotifyURL:    orden.NotifyURL,
		OrderNo:      orden.GsOrderNo,
		ThirdOrderNo: orden.ThirdOrderNo,
		OrderStatus:  "2",
		OrderTime:    FormatGSTime(orderTime),
		PayTime:      FormatGSTime(payTime),
		TotalAmount:  fmt.Sprintf("%d.%02d", orden.PriceCents/100, orden.PriceCents%100),
	})
	if err != nil {
		logging.From(ctx).Error("notify a GS fallo", "thirdOrderNo", orden.ThirdOrderNo, "error", err)
		logging.From(ctx).Info("gs.notify.failed",
			"order_id", orden.ThirdOrderNo,
			"reason", "network_error",
		)
		return
	}
	if resp != nil && resp.ReturnCode != "" && resp.ReturnCode != "success" {
		logging.From(ctx).Error("notify a GS returnCode no exitoso",
			"thirdOrderNo", orden.ThirdOrderNo, "returnCode", resp.ReturnCode)
		logging.From(ctx).Info("gs.notify.failed",
			"order_id", orden.ThirdOrderNo,
			"reason", "return_code_not_success",
			"return_code", resp.ReturnCode,
		)
		return
	}

	now := time.Now()
	if err := s.orderRepo.UpdateStatusAndFields(ctx, orden.ThirdOrderNo, orden.Status,
		map[string]interface{}{"gs_notified_at": now}); err != nil {
		logging.From(ctx).Error("marcando gs_notified_at", "thirdOrderNo", orden.ThirdOrderNo, "error", err)
		return
	}
	orden.GsNotifiedAt = now
	logging.From(ctx).Info("gs.notify.ok",
		"order_id", orden.ThirdOrderNo,
		"third_order_no", orden.ThirdOrderNo,
		"gs_order_no", orden.GsOrderNo,
	)
}

// CompleteOrder: GS notifica resultado de preparacion/entrega (POST /order/complete).
// success+outStockStatus=2 → DONE; success=false → FAILED (refundable si hubo pago).
// returnCode refleja si procesamos el notify, no el exito de la bebida.
func (s *OrderService) CompleteOrder(ctx context.Context, req *CompleteOrderRequest) (*CompleteOrderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request nulo", domain.ErrInvalidInput)
	}
	if req.OrderNo == "" {
		return nil, fmt.Errorf("%w: orderNo es obligatorio", domain.ErrInvalidInput)
	}
	if req.ThirdOrderNo == "" {
		return nil, fmt.Errorf("%w: thirdOrderNo es obligatorio", domain.ErrInvalidInput)
	}

	orden, err := s.orderRepo.GetByThirdOrderNo(ctx, req.ThirdOrderNo)
	if err != nil {
		return nil, fmt.Errorf("buscando orden %q: %w", req.ThirdOrderNo, err)
	}
	if orden.GsOrderNo != req.OrderNo {
		return nil, fmt.Errorf("%w: pair mismatch orderNo %q vs gs_order_no %q",
			domain.ErrInvalidInput, req.OrderNo, orden.GsOrderNo)
	}

	var (
		nuevo  domain.OrderStatus
		fields map[string]interface{}
		retMsg = "success"
	)
	switch {
	case req.Success && req.OutStockStatus == 2:
		nuevo = domain.OrderDone
		fields = map[string]interface{}{"gs_completed_at": time.Now()}
	case !req.Success:
		nuevo = domain.OrderFailed
		fields = map[string]interface{}{
			"failure_reason": "gs_complete_success=false",
		}
		retMsg = "gs_complete_success=false"
	default:
		return nil, fmt.Errorf("%w: success=true requiere outStockStatus=2, got %d",
			domain.ErrInvalidInput, req.OutStockStatus)
	}

	if !orden.Status.CanTransitionTo(nuevo) {
		return nil, fmt.Errorf("%w: orden %s en estado %q no puede ir a %q",
			domain.ErrInvalidTransition, req.ThirdOrderNo, orden.Status, nuevo)
	}

	if err := s.orderRepo.UpdateStatusAndFields(ctx, req.ThirdOrderNo, nuevo, fields); err != nil {
		return nil, err
	}

	return &CompleteOrderResponse{
		OrderNo:      orden.GsOrderNo,
		ThirdOrderNo: orden.ThirdOrderNo,
		ReturnCode:   "success",
		ReturnMsg:    retMsg,
	}, nil
}

// CancelOrder: GS cancela la orden (POST /order/cancel).
// Idempotente si ya esta CANCELLED.
func (s *OrderService) CancelOrder(ctx context.Context, req *CancelOrderRequest) (*CancelOrderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request nulo", domain.ErrInvalidInput)
	}
	if req.OrderNo == "" {
		return nil, fmt.Errorf("%w: orderNo es obligatorio", domain.ErrInvalidInput)
	}
	if req.ThirdOrderNo == "" {
		return nil, fmt.Errorf("%w: thirdOrderNo es obligatorio", domain.ErrInvalidInput)
	}
	if req.CancelTime == "" {
		return nil, fmt.Errorf("%w: cancelTime es obligatorio", domain.ErrInvalidInput)
	}

	orden, err := s.orderRepo.GetByThirdOrderNo(ctx, req.ThirdOrderNo)
	if err != nil {
		return nil, fmt.Errorf("buscando orden %q: %w", req.ThirdOrderNo, err)
	}
	if orden.GsOrderNo != req.OrderNo {
		return nil, fmt.Errorf("%w: pair mismatch orderNo %q vs gs_order_no %q",
			domain.ErrInvalidInput, req.OrderNo, orden.GsOrderNo)
	}

	// Idempotente: ya cancelada.
	if orden.Status == domain.OrderCancelled {
		return &CancelOrderResponse{
			OrderNo:      orden.GsOrderNo,
			ThirdOrderNo: orden.ThirdOrderNo,
			ReturnCode:   "success",
			ReturnMsg:    req.Remark,
		}, nil
	}

	if !orden.Status.CanTransitionTo(domain.OrderCancelled) {
		return nil, fmt.Errorf("%w: orden %s en estado %q no se puede cancelar",
			domain.ErrInvalidTransition, req.ThirdOrderNo, orden.Status)
	}

	fields := map[string]interface{}{"gs_cancelled_at": time.Now()}
	if req.Remark != "" {
		fields["failure_reason"] = req.Remark
	}
	if err := s.orderRepo.UpdateStatusAndFields(ctx, req.ThirdOrderNo, domain.OrderCancelled, fields); err != nil {
		return nil, err
	}

	return &CancelOrderResponse{
		OrderNo:      orden.GsOrderNo,
		ThirdOrderNo: orden.ThirdOrderNo,
		ReturnCode:   "success",
		ReturnMsg:    req.Remark,
	}, nil
}

// resolvePVSWebhookStatus: status texto gana si viene; si no, stateId (compat).
func resolvePVSWebhookStatus(req *PVSWebhookRequest) (domain.PVSStatus, error) {
	if req.Status != "" {
		return domain.PVSStatusFromCallback(req.Status)
	}
	if req.StateID != 0 {
		return domain.PVSStatusFromStateID(req.StateID)
	}
	return "", fmt.Errorf("%w: status o stateId obligatorio", domain.ErrInvalidInput)
}

// pvsStatusToOrderStatus elige el estado interno objetivo para un PVSStatus.
// Toma el estado actual para decidir el destino de REVERSED:
// Si la orden ya esta en REFUND_PENDING (pedimos reembolso), la confirmacion
// de PVS completa el reembolso → REFUNDED.
// Si no, es una reversa espontanea → REFUND_PENDING.
func pvsStatusToOrderStatus(pvs domain.PVSStatus, actual domain.OrderStatus) domain.OrderStatus {
	switch pvs {
	case domain.PVSApproved:
		return domain.OrderPaymentConfirmed
	case domain.PVSReversed:
		if actual == domain.OrderRefundPending {
			return domain.OrderRefunded // confirmacion de reembolso
		}
		return domain.OrderRefundPending // reversa espontanea
	case domain.PVSRejected:
		return domain.OrderFailed
	default:
		return "" // IN_PROCESS / EXPIRED: sin cambio
	}
}

// buscarOrdenReciente busca una orden con mismo dispositivo+producto+precio
// creada en los últimos dedupWindow segundos. Devuelve nil si no encuentra.
// el QR que ya generamos en vez de crear una orden nueva.
func (s *OrderService) buscarOrdenReciente(ctx context.Context, deviceID, objectID string, montoCentavos int64) *domain.Order {
	since := time.Now().Add(-s.dedupWindow)
	orden, err := s.orderRepo.FindRecentDup(ctx, deviceID, objectID, montoCentavos, since)
	if err != nil {
		// best-effort: si falla la busqueda, no bloqueamos la creacion.
		return nil
	}
	return orden
}

func parseAttach(attach string) (deviceNo, deviceID string) {
	attach = strings.TrimSpace(attach)
	if attach == "" {
		return "", ""
	}
	// url.ParseQuery espera query sin "?"; toleramos un "?" inicial.
	q := strings.TrimPrefix(attach, "?")
	vals, err := url.ParseQuery(q)
	if err != nil {
		return "", ""
	}
	return vals.Get("deviceNo"), vals.Get("deviceId")
}

// parseMonto convierte un string como "150.00" a centavos int64 (15000).
// Acepta formatos: "150", "150.00", "150.5", "150,50" (coma decimal).
func parseMonto(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".") // tolerar coma decimal

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("no se pudo parsear %q: %w", s, err)
	}

	centavos := int64(f * 100)
	return centavos, nil
}

// generarThirdOrderNo genera un identificador único para la orden.
// Usa UUID v4. En el futuro se puede migrar a UUID v7 (time-sortable).
func generarThirdOrderNo() string {
	return "ORD-" + uuid.New().String()
}
