// Package service contiene la lógica de negocio que coordina los
// puertos (repositorios y clientes). No sabe nada de HTTP ni SQL.
package service

import (
	"context"
	"fmt"
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
	qrExpiry    time.Duration // cuánto vive el QR (default 180s)
	dedupWindow time.Duration // ventana de dedup (default 20s)
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
func NewOrderService(repo ports.OrderRepository, pvs ports.PVSClient,
	syncLog ports.SyncLogRepo, opts ...Option) *OrderService {
	s := &OrderService{
		orderRepo:   repo,
		pvsClient:   pvs,
		syncLogRepo: syncLog,
		qrExpiry:    180 * time.Second,
		dedupWindow: 20 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// --- Tipos de request/response ---

// CreateOrderRequest es lo que GS nos envía cuando un cliente elige un producto.
type CreateOrderRequest struct {
	ObjectID    string `json:"objectId"`     // SKU del producto
	Subject     string `json:"subject"`      // Descripción
	TotalAmount string `json:"totalAmount"`  // Monto como string ("150.00")
	DeviceID    string `json:"deviceId"`     // ID del dispositivo GS
	DeviceNo    string `json:"deviceNo"`     // Número de serie
	PayMethod   string `json:"payMethod"`    // Método de pago (ej: "wxpay")
	WayCode     string `json:"wayCode"`      // Código de vía (ej: "qr")
}

// CreateOrderResponse es lo que le devolvemos a GS.
type CreateOrderResponse struct {
	QrURL        string `json:"qrUrl"`        // base64 del QR (traducido de PVS qrImage)
	OrderStatus  int    `json:"orderStatus"`  // 1 = pendiente de pago
	ThirdOrderNo string `json:"thirdOrderNo"` // nuestro orderNo
}

// QueryStatusRequest es el polling de GS: nos pregunta el estado de una orden.
type QueryStatusRequest struct {
	ThirdOrderNo string `json:"thirdOrderNo"` // nuestro orderNo
}

// QueryStatusResponse devuelve el estado traducido al formato 1-6 de GS.
type QueryStatusResponse struct {
	OrderStatus  int    `json:"orderStatus"`  // 1-6 segun DOCX seccion 2.3
	ThirdOrderNo string `json:"thirdOrderNo"`
}

// PVSWebhookRequest es lo que PVS nos envia cuando cambia el estado de un QR.
type PVSWebhookRequest struct {
	QrID    string `json:"qrId"`
	StateID int    `json:"stateId"` // 6=In Process, 5=Approved, 4=Reverse, 3=Rejected
}

// CreateOrder ejecuta el flujo completo de creación de orden:
// 1. Validar request
// 2. Dedup: ¿ya tenemos una orden idéntica en los últimos 20s?
// 3. Crear orden en DB con status=RECEIVED
// 4. Llamar a PVS para generar QR
// 5. Actualizar orden con qrId + qrImage + status=QR_SHOWN
// 6. Devolver respuesta a GS (con traducción qrImage→qrUrl)
func (s *OrderService) CreateOrder(ctx context.Context, req *CreateOrderRequest) (*CreateOrderResponse, error) {
	// 1. Validar
	if req.ObjectID == "" {
		return nil, fmt.Errorf("%w: objectId es obligatorio", domain.ErrInvalidInput)
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

	// 2. Dedup: buscar orden reciente con mismo dispositivo+producto+precio
	existente := s.buscarOrdenReciente(ctx, req.DeviceID, req.ObjectID, montoCentavos)
	if existente != nil && existente.PvsQrImage != "" {
		// Ya tenemos una orden para este pedido → devolver el mismo QR
		return &CreateOrderResponse{
			QrURL:        existente.PvsQrImage,
			OrderStatus:  1,
			ThirdOrderNo: existente.OrderNo,
		}, nil
	}

	// 3. Generar orderNo único
	orderNo := generarOrderNo()

	// 4. Crear orden en DB
	orden := &domain.Order{
		OrderNo:    orderNo,
		DeviceID:   req.DeviceID,
		DeviceNo:   req.DeviceNo,
		ObjectID:   req.ObjectID,
		PriceCents: montoCentavos,
		PayMethod:  req.PayMethod,
		WayCode:    req.WayCode,
		Status:     domain.OrderReceived,
	}
	if err := s.orderRepo.Create(ctx, orden); err != nil {
		return nil, fmt.Errorf("creando orden en DB: %w", err)
	}

	// 5. Llamar a PVS para generar QR
	pvsResp, err := s.pvsClient.GenerateQR(ctx, &ports.PVSQRRequest{
		Amount:     montoCentavos,
		ExternalID: orderNo,
		Reference:  orderNo,
	})
	if err != nil {
		// PVS falló → marcar orden como FAILED y devolver error
		_ = s.orderRepo.UpdateStatus(ctx, orderNo, domain.OrderFailed)
		return nil, fmt.Errorf("generando QR en PVS: %w", err)
	}

	// 6. Actualizar orden con datos del QR
	expira := time.Now().Add(s.qrExpiry)
	err = s.orderRepo.UpdateStatusAndFields(ctx, orderNo, domain.OrderQRShown, map[string]interface{}{
		"pvs_qr_id":       pvsResp.QrID,
		"pvs_qr_image":    pvsResp.QrImage,
		"qr_generated_at": time.Now(),
		"qr_expires_at":   expira,
	})
	if err != nil {
		return nil, fmt.Errorf("actualizando orden con QR: %w", err)
	}

	// 7. Log de auditoría (best-effort, no falla el flujo)
	_ = s.syncLogRepo.Insert(ctx, &ports.SyncLogEntry{
		OrderNo:   orderNo,
		Vendor:    "PVS",
		Direction: "outbound",
		Endpoint:  "/qr/pvs",
		Method:    "POST",
	})

	// 8. Respuesta a GS
	// TRADUCCIÓN CLAVE: PVS devuelve qrImage, GS espera qrUrl.
	// El contenido es el mismo (base64 PNG).
	return &CreateOrderResponse{
		QrURL:        pvsResp.QrImage,
		OrderStatus:  1, // pendiente de pago
		ThirdOrderNo: orderNo,
	}, nil
}

// QueryStatus responde al polling de GS.
// GS nos pregunta "¿en qué estado está esta orden?" y le devolvemos
// nuestro estado interno traducido al formato 1-6 que entiende la máquina.
// No llamamos a PVS: devolvemos lo que sabemos de nuestra DB.
func (s *OrderService) QueryStatus(ctx context.Context, req *QueryStatusRequest) (*QueryStatusResponse, error) {
	if req.ThirdOrderNo == "" {
		return nil, fmt.Errorf("%w: thirdOrderNo es obligatorio", domain.ErrInvalidInput)
	}

	orden, err := s.orderRepo.GetByOrderNo(ctx, req.ThirdOrderNo)
	if err != nil {
		return nil, fmt.Errorf("buscando orden %q: %w", req.ThirdOrderNo, err)
	}

	return &QueryStatusResponse{
		OrderStatus:  int(orden.Status.ToGSStatus()),
		ThirdOrderNo: orden.OrderNo,
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
	updated, err := s.orderRepo.UpdateStatusGuardedAndFields(ctx, orden.OrderNo,
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
		OrderNo:   orden.OrderNo,
		Vendor:    "PVS",
		Direction: "inbound",
		Endpoint:  "/webhook/pvs",
		Method:    "POST",
	})

	return nil
}

// CompleteOrder: GS notifica que el producto fue entregado (outStockStatus=2).
// Transicion: PAYMENT_CONFIRMED → DONE.
// Setea gs_completed_at para reconciliacion.
func (s *OrderService) CompleteOrder(ctx context.Context, thirdOrderNo string) error {
	if thirdOrderNo == "" {
		return fmt.Errorf("%w: thirdOrderNo es obligatorio", domain.ErrInvalidInput)
	}

	orden, err := s.orderRepo.GetByOrderNo(ctx, thirdOrderNo)
	if err != nil {
		return fmt.Errorf("buscando orden %q: %w", thirdOrderNo, err)
	}

	if !orden.Status.CanTransitionTo(domain.OrderDone) {
		return fmt.Errorf("%w: orden %s en estado %q no se puede completar",
			domain.ErrInvalidTransition, thirdOrderNo, orden.Status)
	}

	return s.orderRepo.UpdateStatusAndFields(ctx, thirdOrderNo, domain.OrderDone,
		map[string]interface{}{"gs_completed_at": time.Now()})
}

// CancelOrder: GS cancela la orden.
// Transicion: QR_SHOWN o PAYMENT_CONFIRMED → CANCELLED.
// Setea gs_cancelled_at.
func (s *OrderService) CancelOrder(ctx context.Context, thirdOrderNo string) error {
	if thirdOrderNo == "" {
		return fmt.Errorf("%w: thirdOrderNo es obligatorio", domain.ErrInvalidInput)
	}

	orden, err := s.orderRepo.GetByOrderNo(ctx, thirdOrderNo)
	if err != nil {
		return fmt.Errorf("buscando orden %q: %w", thirdOrderNo, err)
	}

	if !orden.Status.CanTransitionTo(domain.OrderCancelled) {
		return fmt.Errorf("%w: orden %s en estado %q no se puede cancelar",
			domain.ErrInvalidTransition, thirdOrderNo, orden.Status)
	}

	return s.orderRepo.UpdateStatusAndFields(ctx, thirdOrderNo, domain.OrderCancelled,
		map[string]interface{}{"gs_cancelled_at": time.Now()})
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

// generarOrderNo genera un identificador único para la orden.
// Usa UUID v4. En el futuro se puede migrar a UUID v7 (time-sortable).
func generarOrderNo() string {
	return "ORD-" + uuid.New().String()
}
