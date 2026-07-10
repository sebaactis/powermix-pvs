package service

import (
	"context"
	"fmt"
	"time"

	"github.com/seba/vps-powermix/internal/domain"
	"github.com/seba/vps-powermix/internal/ports"
)

// RefundService maneja los reembolsos solicitados por GS.
// Depende de orderRepo (para actualizar estado de la orden), pvsClient
// (para ejecutar el reverse), refundRepo (para crear/seguir el reembolso),
// y syncLog (para auditoria).
type RefundService struct {
	orderRepo  ports.OrderRepository
	pvsClient  ports.PVSClient
	refundRepo ports.RefundRepository
	syncLog    ports.SyncLogRepo
}

// NewRefundService crea un RefundService listo para usar.
func NewRefundService(orderRepo ports.OrderRepository, pvs ports.PVSClient,
	refundRepo ports.RefundRepository, syncLog ports.SyncLogRepo) *RefundService {
	return &RefundService{
		orderRepo:  orderRepo,
		pvsClient:  pvs,
		refundRepo: refundRepo,
		syncLog:    syncLog,
	}
}

// RefundRequest es lo que GS nos envía para pedir un reembolso (GS Open API v2).
type RefundRequest struct {
	RefundNo     string `json:"refundNo"`
	OrderNo      string `json:"orderNo"` // serial de GS (debe matchear orden.GsOrderNo)
	ThirdOrderNo string `json:"thirdOrderNo"`
	Amount       string `json:"refundAmount"`
	Reason       string `json:"refundReason"`
}

// RefundResponse es lo que le devolvemos a GS (GS Open API v2).
type RefundResponse struct {
	RefundNo      string `json:"refundNo"`
	OrderNo       string `json:"orderNo"`
	ThirdOrderNo  string `json:"thirdOrderNo"`
	ThirdRefundNo string `json:"thirdRefundNo"`
	RefundStatus  string `json:"refundStatus"` // "waiting" | "fail"
	RefundMsg     string `json:"refundMsg"`
	RefundTime    string `json:"refundTime"`
	TotalAmount   string `json:"totalAmount"`
	RefundAmount  string `json:"refundAmount"`
}

// Refund procesa una solicitud de reembolso de GS (GS Open API v2).
//
// Flujo:
//  1. Validar request (refundNo, orderNo, thirdOrderNo, refundAmount)
//  2. Buscar orden por thirdOrderNo
//  3. Pair verify: orden.GsOrderNo == req.OrderNo
//  4. Idempotencia: si ya existe este refundNo, devolver resultado mapeado
//  5. Validar que la orden sea refundable (CanTransitionTo REFUND_PENDING)
//  6. Guard: payment_confirmed_at debe estar seteado (hubo pago real)
//  7. Crear registro de reembolso PENDING
//  8. Llamar a PVS.Reverse(qrId)
//     - Error    -> orden REFUND_FAILED, refund FAILED + error propagado
//     - Rechazo  -> orden REFUND_FAILED, refund FAILED, status "fail"
//     - Success  -> orden REFUND_PENDING, refund SUCCESS, status "waiting"
//
// La confirmacion final del reembolso llega via webhook de PVS (stateId=4)
// que mueve REFUND_PENDING -> REFUNDED. Recien ahi el refundStatus es "success".
func (s *RefundService) Refund(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	// 1. Validar
	if req.RefundNo == "" {
		return nil, fmt.Errorf("%w: refundNo es obligatorio", domain.ErrInvalidInput)
	}
	if req.OrderNo == "" {
		return nil, fmt.Errorf("%w: orderNo es obligatorio", domain.ErrInvalidInput)
	}
	if req.ThirdOrderNo == "" {
		return nil, fmt.Errorf("%w: thirdOrderNo es obligatorio", domain.ErrInvalidInput)
	}
	montoCentavos, err := parseMonto(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("%w: monto invalido %q: %w", domain.ErrInvalidInput, req.Amount, err)
	}
	if montoCentavos <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	// 2. Buscar orden
	orden, err := s.orderRepo.GetByThirdOrderNo(ctx, req.ThirdOrderNo)
	if err != nil {
		return nil, fmt.Errorf("buscando orden %q: %w", req.ThirdOrderNo, err)
	}

	// 3. Pair verify: el orderNo de GS debe coincidir con nuestro registro
	if orden.GsOrderNo != req.OrderNo {
		return nil, fmt.Errorf("%w: pair mismatch orderNo %q vs gs_order_no %q",
			domain.ErrInvalidInput, req.OrderNo, orden.GsOrderNo)
	}

	// 4. Idempotencia: si ya existe este refundNo, devolver resultado mapeado
	existente, err := s.refundRepo.GetByRefundNo(ctx, req.RefundNo)
	if err != nil && err != domain.ErrRefundNotFound {
		return nil, fmt.Errorf("buscando reembolso existente: %w", err)
	}
	if existente != nil {
		return s.buildRefundResponse(orden, existente), nil
	}

	// 5. Validar que la orden sea reembolsable
	if !orden.Status.CanTransitionTo(domain.OrderRefundPending) {
		return nil, domain.ErrOrderNotRefundable
	}

	// 6. Guard: debe haber un pago confirmado registrado
	if orden.PaymentConfirmedAt.IsZero() {
		return nil, domain.ErrOrderNotRefundable
	}

	// 7. Crear registro de reembolso PENDING
	refund := &domain.Refund{
		RefundNo:     req.RefundNo,
		ThirdOrderNo: orden.ThirdOrderNo,
		PriceCents:   montoCentavos,
		Motivo:       req.Reason,
		Status:       domain.RefundPending,
		GSRefundNo:   req.RefundNo,
	}
	if err := s.refundRepo.Create(ctx, refund); err != nil {
		return nil, fmt.Errorf("creando reembolso: %w", err)
	}

	// 8. Llamar a PVS para revertir el pago
	pvsResp, err := s.pvsClient.Reverse(ctx, orden.PvsQrID)
	if err != nil {
		// Error de red/infra: marcar como fallido y propagar
		_ = s.refundRepo.UpdateStatus(ctx, req.RefundNo, domain.RefundFailed)
		_ = s.orderRepo.UpdateStatus(ctx, orden.ThirdOrderNo, domain.OrderRefundFailed)
		return nil, fmt.Errorf("reverse en PVS: %w", err)
	}
	if !pvsResp.Success {
		// PVS rechazo el reverse (negocio): devolver fail, no es error del sistema
		_ = s.refundRepo.UpdateStatus(ctx, req.RefundNo, domain.RefundFailed)
		_ = s.orderRepo.UpdateStatus(ctx, orden.ThirdOrderNo, domain.OrderRefundFailed)
		refund.Status = domain.RefundFailed
		return s.buildRefundResponse(orden, refund), nil
	}

	// 9. PVS acepto el reverse: orden REFUND_PENDING, refund SUCCESS, status "waiting"
	_ = s.orderRepo.UpdateStatus(ctx, orden.ThirdOrderNo, domain.OrderRefundPending)
	_ = s.refundRepo.UpdateStatus(ctx, req.RefundNo, domain.RefundSuccess)
	refund.Status = domain.RefundSuccess

	// 10. Audit log (best-effort)
	_ = s.syncLog.Insert(ctx, &ports.SyncLogEntry{
		ThirdOrderNo: orden.ThirdOrderNo,
		Vendor:       "PVS",
		Direction:    "outbound",
		Endpoint:     "/reverse/pvs",
		Method:       "POST",
	})

	return s.buildRefundResponse(orden, refund), nil
}

// buildRefundResponse arma la respuesta GS v2 a partir de la orden y el refund.
// Mapea el estado del refund + orden al refundStatus del contrato GS:
//   - orden REFUNDED -> "success" (PVS confirmo el reverse via webhook)
//   - refund SUCCESS -> "waiting" (PVS acepto el reverse, pendiente confirmacion)
//   - refund FAILED  -> "fail"
func (s *RefundService) buildRefundResponse(orden *domain.Order, refund *domain.Refund) *RefundResponse {
	status, msg := mapRefundStatusForRefund(orden, refund)
	return &RefundResponse{
		RefundNo:      refund.RefundNo,
		OrderNo:       orden.GsOrderNo,
		ThirdOrderNo:  orden.ThirdOrderNo,
		ThirdRefundNo: refund.RefundNo,
		RefundStatus:  status,
		RefundMsg:     msg,
		RefundTime:    FormatGSTime(time.Now()),
		TotalAmount:   fmt.Sprintf("%d.%02d", orden.PriceCents/100, orden.PriceCents%100),
		RefundAmount:  fmt.Sprintf("%d.%02d", refund.PriceCents/100, refund.PriceCents%100),
	}
}

// mapRefundStatusForRefund: vocabulario del endpoint /order/refund (waiting|success|fail).
func mapRefundStatusForRefund(orden *domain.Order, refund *domain.Refund) (status, msg string) {
	switch {
	case orden.Status == domain.OrderRefunded:
		return "success", "success"
	case refund.Status == domain.RefundSuccess:
		return "waiting", "waiting"
	case refund.Status == domain.RefundFailed,
		orden.Status == domain.OrderRefundFailed:
		return "fail", "fail"
	default:
		return "fail", "fail"
	}
}

// mapRefundStatusForQuery: vocabulario del endpoint /order/refundStatus (pending|success|fail).
func mapRefundStatusForQuery(orden *domain.Order, refund *domain.Refund) (status, msg string) {
	switch {
	case orden.Status == domain.OrderRefunded:
		return "success", "success"
	case refund.Status == domain.RefundSuccess,
		refund.Status == domain.RefundPending,
		orden.Status == domain.OrderRefundPending:
		return "pending", "pending"
	case refund.Status == domain.RefundFailed,
		orden.Status == domain.OrderRefundFailed:
		return "fail", "fail"
	default:
		return "fail", "fail"
	}
}

// --- RefundStatus (POST /order/refundStatus) ---

// RefundStatusRequest es el body de POST /order/refundStatus.
type RefundStatusRequest struct {
	RefundNo     string `json:"refundNo"`     // optional
	OrderNo      string `json:"orderNo"`      // required
	ThirdOrderNo string `json:"thirdOrderNo"` // optional
}

// RefundStatusResponse es la respuesta de POST /order/refundStatus.
type RefundStatusResponse struct {
	RefundNo      string `json:"refundNo"`
	OrderNo       string `json:"orderNo"`
	ThirdOrderNo  string `json:"thirdOrderNo"`
	ThirdRefundNo string `json:"thirdRefundNo"`
	RefundStatus  string `json:"refundStatus"` // pending|success|fail
	RefundMsg     string `json:"refundMsg"`
	RefundTime    string `json:"refundTime"`
	TotalAmount   string `json:"totalAmount"`
	RefundAmount  string `json:"refundAmount"`
}

// RefundStatus consulta el estado de un reembolso (GS Open API v2).
func (s *RefundService) RefundStatus(ctx context.Context, req *RefundStatusRequest) (*RefundStatusResponse, error) {
	if req.OrderNo == "" {
		return nil, fmt.Errorf("%w: orderNo es obligatorio", domain.ErrInvalidInput)
	}

	var (
		refund *domain.Refund
		err    error
	)

	switch {
	case req.RefundNo != "":
		refund, err = s.refundRepo.GetByRefundNo(ctx, req.RefundNo)
	case req.ThirdOrderNo != "":
		refund, err = s.refundRepo.GetLatestByThirdOrderNo(ctx, req.ThirdOrderNo)
	default:
		// Solo orderNo: resolver orden por GsOrderNo y tomar el refund mas reciente.
		ordenGS, loadErr := s.orderRepo.GetByGsOrderNo(ctx, req.OrderNo)
		if loadErr != nil {
			return nil, fmt.Errorf("buscando orden por orderNo %q: %w", req.OrderNo, loadErr)
		}
		refund, err = s.refundRepo.GetLatestByThirdOrderNo(ctx, ordenGS.ThirdOrderNo)
	}
	if err != nil {
		return nil, err
	}

	orden, err := s.orderRepo.GetByThirdOrderNo(ctx, refund.ThirdOrderNo)
	if err != nil {
		return nil, fmt.Errorf("buscando orden %q: %w", refund.ThirdOrderNo, err)
	}

	// Pair verify con el orderNo de GS.
	if orden.GsOrderNo != req.OrderNo {
		return nil, fmt.Errorf("%w: pair mismatch orderNo %q vs gs_order_no %q",
			domain.ErrInvalidInput, req.OrderNo, orden.GsOrderNo)
	}
	// Si GS mando thirdOrderNo, tambien debe matchear.
	if req.ThirdOrderNo != "" && orden.ThirdOrderNo != req.ThirdOrderNo {
		return nil, fmt.Errorf("%w: pair mismatch thirdOrderNo %q vs %q",
			domain.ErrInvalidInput, req.ThirdOrderNo, orden.ThirdOrderNo)
	}

	status, msg := mapRefundStatusForQuery(orden, refund)
	refundTime := FormatGSTime(refund.CompletedAt)
	if refundTime == "" {
		refundTime = FormatGSTime(refund.UpdatedAt)
	}
	if refundTime == "" {
		refundTime = FormatGSTime(refund.CreatedAt)
	}
	if refundTime == "" {
		refundTime = FormatGSTime(time.Now())
	}

	return &RefundStatusResponse{
		RefundNo:      refund.RefundNo,
		OrderNo:       orden.GsOrderNo,
		ThirdOrderNo:  orden.ThirdOrderNo,
		ThirdRefundNo: refund.RefundNo,
		RefundStatus:  status,
		RefundMsg:     msg,
		RefundTime:    refundTime,
		TotalAmount:   fmt.Sprintf("%d.%02d", orden.PriceCents/100, orden.PriceCents%100),
		RefundAmount:  fmt.Sprintf("%d.%02d", refund.PriceCents/100, refund.PriceCents%100),
	}, nil
}
