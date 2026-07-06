package service

import (
	"context"
	"fmt"

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

// RefundRequest es lo que GS nos envía para pedir un reembolso.
type RefundRequest struct {
	RefundNo     string `json:"refundNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
	Amount       string `json:"refundAmount"`
	Reason       string `json:"refundReason"`
}

// RefundResponse es lo que le devolvemos a GS.
type RefundResponse struct {
	RefundNo     string `json:"refundNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
	RefundStatus string `json:"refundStatus"` // "success" o "fail"
}

// Refund procesa una solicitud de reembolso de GS.
//
// Flujo:
//  1. Validar request
//  2. Idempotencia: si ya existe este refundNo, devolver resultado anterior
//  3. Buscar orden y validar que sea refundable (PAYMENT_CONFIRMED o DONE)
//  4. Crear registro de reembolso PENDING
//  5. Llamar a PVS.Reverse(qrId)
//     - Success  -> orden REFUND_PENDING, refund SUCCESS
//     - Rechazo  -> orden REFUND_FAILED, refund FAILED
//     - Error    -> orden REFUND_FAILED, refund FAILED + error propagado
//
// La confirmacion final del reembolso llega via webhook de PVS (stateId=4)
// que mueve REFUND_PENDING -> REFUNDED.
func (s *RefundService) Refund(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	// 1. Validar
	if req.RefundNo == "" {
		return nil, fmt.Errorf("%w: refundNo es obligatorio", domain.ErrInvalidInput)
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

	// 2. Idempotencia: si ya existe este refundNo, devolver resultado anterior
	existente, err := s.refundRepo.GetByRefundNo(ctx, req.RefundNo)
	if err != nil && err != domain.ErrRefundNotFound {
		return nil, fmt.Errorf("buscando reembolso existente: %w", err)
	}
	if existente != nil {
		status := "fail"
		if existente.Status == domain.RefundSuccess {
			status = "success"
		}
		return &RefundResponse{
			RefundNo:     existente.RefundNo,
			ThirdOrderNo: existente.OrderNo,
			RefundStatus: status,
		}, nil
	}

	// 3. Buscar y validar orden
	orden, err := s.orderRepo.GetByOrderNo(ctx, req.ThirdOrderNo)
	if err != nil {
		return nil, fmt.Errorf("buscando orden %q: %w", req.ThirdOrderNo, err)
	}
	if !orden.Status.CanTransitionTo(domain.OrderRefundPending) {
		return nil, domain.ErrOrderNotRefundable
	}

	// 4. Crear registro de reembolso
	refund := &domain.Refund{
		RefundNo:   req.RefundNo,
		OrderNo:    orden.OrderNo,
		PriceCents: montoCentavos,
		Motivo:     req.Reason,
		Status:     domain.RefundPending,
		GSRefundNo: req.RefundNo,
	}
	if err := s.refundRepo.Create(ctx, refund); err != nil {
		return nil, fmt.Errorf("creando reembolso: %w", err)
	}

	// 5. Llamar a PVS para revertir el pago
	pvsResp, err := s.pvsClient.Reverse(ctx, orden.PvsQrID)
	if err != nil {
		// Error de red/infra: marcar como fallido y propagar
		_ = s.refundRepo.UpdateStatus(ctx, req.RefundNo, domain.RefundFailed)
		_ = s.orderRepo.UpdateStatus(ctx, orden.OrderNo, domain.OrderRefundFailed)
		return nil, fmt.Errorf("reverse en PVS: %w", err)
	}
	if !pvsResp.Success {
		// PVS rechazo el reverse (negocio): devolver fail, no es error del sistema
		_ = s.refundRepo.UpdateStatus(ctx, req.RefundNo, domain.RefundFailed)
		_ = s.orderRepo.UpdateStatus(ctx, orden.OrderNo, domain.OrderRefundFailed)
		return &RefundResponse{
			RefundNo:     req.RefundNo,
			ThirdOrderNo: orden.OrderNo,
			RefundStatus: "fail",
		}, nil
	}

	// 6. Success: orden REFUND_PENDING (espera webhook confirmacion), refund SUCCESS
	_ = s.orderRepo.UpdateStatus(ctx, orden.OrderNo, domain.OrderRefundPending)
	_ = s.refundRepo.UpdateStatus(ctx, req.RefundNo, domain.RefundSuccess)

	// 7. Audit log (best-effort)
	_ = s.syncLog.Insert(ctx, &ports.SyncLogEntry{
		OrderNo:   orden.OrderNo,
		Vendor:    "PVS",
		Direction: "outbound",
		Endpoint:  "/reverse/pvs",
		Method:    "POST",
	})

	return &RefundResponse{
		RefundNo:     req.RefundNo,
		ThirdOrderNo: orden.OrderNo,
		RefundStatus: "success",
	}, nil
}
