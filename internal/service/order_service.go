// Package service contiene la lógica de negocio que coordina los
// puertos (repositorios y clientes). No sabe nada de HTTP ni SQL.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/ports"
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

// Option permite configurar el servicio (functional options).
type Option func(*OrderService)

// ConQRExpiry cambia el tiempo de validez del QR.
func ConQRExpiry(d time.Duration) Option {
	return func(s *OrderService) { s.qrExpiry = d }
}

// ConDedupWindow cambia la ventana de deduplicación.
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

// FormatGSTime formatea timestamps al estilo GS: yyyy-MM-dd HH:mm:ss UTC.
func FormatGSTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

// --- Tipos de request/response (GS Open API v2) ---

// CreateOrderRequest es el body de POST /order/qr.
type CreateOrderRequest struct {
	OrderNo     string `json:"orderNo"`     // serial de GS (requerido)
	ObjectID    string `json:"objectId"`    // SKU (opcional en v2)
	Subject     string `json:"subject"`     // nombre del producto (requerido)
	Attach      string `json:"attach"`      // deviceNo=...&deviceId=...
	TotalAmount string `json:"totalAmount"` // monto string ("150.00")
	NotifyURL   string `json:"notifyUrl"`   // callback de pago hacia GS
}

// CreateOrderResponse es data de POST /order/qr.
type CreateOrderResponse struct {
	QrURL        string `json:"qrUrl"`        // base64 del QR (PVS qrImage → qrUrl)
	OrderStatus  string `json:"orderStatus"`  // "1" = pendiente de pago
	ThirdOrderNo string `json:"thirdOrderNo"` // nuestro id
}

// QueryStatusRequest es el body de POST /order/status.
type QueryStatusRequest struct {
	OrderNo      string `json:"orderNo"`      // serial GS
	ThirdOrderNo string `json:"thirdOrderNo"` // nuestro id
}

// QueryStatusResponse es data de POST /order/status.
type QueryStatusResponse struct {
	OrderNo      string `json:"orderNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
	OrderStatus  string `json:"orderStatus"` // "1"…"6"
}

// FlexibleStatus acepta orderStatus como int o string en JSON.
type FlexibleStatus string

// UnmarshalJSON tolera "2" o 2.
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

// CompleteOrderRequest es el body de POST /order/complete.
type CompleteOrderRequest struct {
	OrderNo        string         `json:"orderNo"`
	ThirdOrderNo   string         `json:"thirdOrderNo"`
	Success        bool           `json:"success"`
	OrderStatus    FlexibleStatus `json:"orderStatus"`
	OutStockStatus int            `json:"outStockStatus"`
	OutStockTime   string         `json:"outStockTime"`
}

// CompleteOrderResponse es data de POST /order/complete.
type CompleteOrderResponse struct {
	OrderNo      string `json:"orderNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
	ReturnCode   string `json:"returnCode"`
	ReturnMsg    string `json:"returnMsg"`
}

// CancelOrderRequest es el body de POST /order/cancel.
type CancelOrderRequest struct {
	OrderNo      string         `json:"orderNo"`
	ThirdOrderNo string         `json:"thirdOrderNo"`
	OrderStatus  FlexibleStatus `json:"orderStatus"` // 0 = cancel
	Remark       string         `json:"remark"`
	CancelTime   string         `json:"cancelTime"`
}

// CancelOrderResponse es data de POST /order/cancel.
type CancelOrderResponse struct {
	OrderNo      string `json:"orderNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
	ReturnCode   string `json:"returnCode"`
	ReturnMsg    string `json:"returnMsg"`
}

// PVSWebhookRequest es lo que PVS nos envia cuando cambia el estado de un QR.
type PVSWebhookRequest struct {
	QrID    string `json:"qrId"`
	StateID int    `json:"stateId"` // 6=In Process, 5=Approved, 4=Reverse, 3=Rejected
}

// CreateOrder ejecuta POST /order/qr (GS Open API v2):
// 1. Validar orderNo, subject, totalAmount, notifyUrl
// 2. Idempotencia por gs_order_no si ya hay QR_SHOWN
// 3. Crear orden (third_order_no nuestro + gs_order_no + notify_url)
// 4. PVS GenerateQR
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
	if req.QrID == "" {
		return fmt.Errorf("%w: qrId es obligatorio", domain.ErrInvalidInput)
	}

	// 1. Buscar orden por el qrId de PVS
	orden, err := s.orderRepo.GetByPVSQrID(ctx, req.QrID)
	if err != nil {
		return fmt.Errorf("buscando orden por qrId %q: %w", req.QrID, err)
	}

	// 2. Mapear stateId -> estado interno objetivo
	pvsStatus, err := domain.PVSStatusFromStateID(req.StateID)
	if err != nil {
		return fmt.Errorf("mapeando stateId %d: %w", req.StateID, err)
	}
	nuevoEstado := pvsStatusToOrderStatus(pvsStatus, orden.Status)
	if nuevoEstado == "" {
		// stateId=6 (IN_PROCESS): el pago sigue pendiente, nada que hacer.
		return nil
	}

	// 3. Validar transicion a nivel de logica (CanTransitionTo)
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

	// 6. Notify outbound a GS (solo pago confirmado; best-effort)
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
	// Solo pago confirmado (status "2" en el contrato GS).
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
		slog.Error("notify a GS fallo", "thirdOrderNo", orden.ThirdOrderNo, "error", err)
		return
	}
	if resp != nil && resp.ReturnCode != "" && resp.ReturnCode != "success" {
		slog.Error("notify a GS returnCode no exitoso",
			"thirdOrderNo", orden.ThirdOrderNo, "returnCode", resp.ReturnCode)
		return
	}

	now := time.Now()
	if err := s.orderRepo.UpdateStatusAndFields(ctx, orden.ThirdOrderNo, orden.Status,
		map[string]interface{}{"gs_notified_at": now}); err != nil {
		slog.Error("marcando gs_notified_at", "thirdOrderNo", orden.ThirdOrderNo, "error", err)
		return
	}
	orden.GsNotifiedAt = now
	slog.Info("notify a GS ok", "thirdOrderNo", orden.ThirdOrderNo, "gsOrderNo", orden.GsOrderNo)
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
// Es la deduplicacion: si GS reenvia el mismo pedido rapido, devolvemos
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

// parseAttach interpreta attach de GS: "deviceNo=E00375&deviceId=7678...".
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
