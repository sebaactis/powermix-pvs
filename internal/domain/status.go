package domain

import (
	"fmt"
	"strings"
)

// OrderStatus es el state machine interno (distinto de GS 1–6 y PVS stateId).
type OrderStatus string

const (
	OrderReceived         OrderStatus = "RECEIVED"
	OrderQRRequested      OrderStatus = "QR_REQUESTED"
	OrderQRShown          OrderStatus = "QR_SHOWN"
	OrderPaymentConfirmed OrderStatus = "PAYMENT_CONFIRMED"
	OrderDone             OrderStatus = "DONE"
	OrderFailed           OrderStatus = "FAILED"
	OrderTimeout          OrderStatus = "TIMEOUT"
	OrderCancelled        OrderStatus = "CANCELLED"
	OrderRefundPending    OrderStatus = "REFUND_PENDING"
	OrderRefunded         OrderStatus = "REFUNDED"
	OrderRefundFailed     OrderStatus = "REFUND_FAILED"
)

// PVSStatus mapea stateId PVS: 6 IN_PROCESS, 5 APPROVED, 4 REVERSED, 3 REJECTED.
type PVSStatus string

const (
	PVSInProcess PVSStatus = "IN_PROCESS"
	PVSApproved  PVSStatus = "APPROVED"
	PVSReversed  PVSStatus = "REVERSED"
	PVSRejected  PVSStatus = "REJECTED"
	PVSExpired   PVSStatus = "EXPIRED" // lo detectamos nosotros, no viene de PVS
)

// GSOrderStatus: códigos 1–6 de la máquina (Open API / doc GS).
type GSOrderStatus int

const (
	GSPending       GSOrderStatus = 1
	GSPaid          GSOrderStatus = 2
	GSFailed        GSOrderStatus = 3
	GSPendingRefund GSOrderStatus = 4
	GSRefunded      GSOrderStatus = 5
	GSTimeout       GSOrderStatus = 6
)

var transitionTable = map[OrderStatus][]OrderStatus{
	OrderReceived:         {OrderQRRequested, OrderFailed},
	OrderQRRequested:      {OrderQRShown, OrderFailed},
	OrderQRShown:          {OrderPaymentConfirmed, OrderFailed, OrderTimeout, OrderCancelled},
	OrderPaymentConfirmed: {OrderDone, OrderRefundPending, OrderFailed, OrderCancelled},
	OrderRefundPending:    {OrderRefunded, OrderRefundFailed},
	// DONE/FAILED siguen reembolsables (complete fail post-pago, etc.)
	OrderDone:         {OrderRefundPending},
	OrderFailed:       {OrderRefundPending},
	OrderTimeout:      {},
	OrderCancelled:    {},
	OrderRefunded:     {},
	OrderRefundFailed: {},
}

func (s OrderStatus) CanTransitionTo(nuevo OrderStatus) bool {
	for _, permitido := range transitionTable[s] {
		if permitido == nuevo {
			return true
		}
	}
	return false
}

func (s OrderStatus) EsEstadoTerminal() bool {
	return len(transitionTable[s]) == 0
}

// ToGSStatus traduce estado interno → código 1–6 que entiende GS.
func (s OrderStatus) ToGSStatus() GSOrderStatus {
	switch s {
	case OrderReceived, OrderQRRequested, OrderQRShown:
		return GSPending
	case OrderPaymentConfirmed, OrderDone:
		return GSPaid
	case OrderFailed, OrderCancelled:
		return GSFailed
	case OrderRefundPending:
		return GSPendingRefund
	case OrderRefunded, OrderRefundFailed:
		return GSRefunded
	case OrderTimeout:
		return GSTimeout
	default:
		return GSFailed
	}
}

// PVSStatusFromStateID: 6/5/4/3 → PVSStatus.
func PVSStatusFromStateID(stateID int) (PVSStatus, error) {
	switch stateID {
	case 6:
		return PVSInProcess, nil
	case 5:
		return PVSApproved, nil
	case 4:
		return PVSReversed, nil
	case 3:
		return PVSRejected, nil
	default:
		return "", fmt.Errorf("stateId desconocido: %d", stateID)
	}
}

// PVSStatusFromCallback mapea status texto del callback oficial PVS.
func PVSStatusFromCallback(status string) (PVSStatus, error) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "APPROVED":
		return PVSApproved, nil
	case "REJECTED":
		return PVSRejected, nil
	case "REVERSED", "REVERSE":
		return PVSReversed, nil
	case "IN_PROCESS", "IN PROCESS", "PENDING":
		return PVSInProcess, nil
	default:
		return "", fmt.Errorf("status callback desconocido: %q", status)
	}
}
