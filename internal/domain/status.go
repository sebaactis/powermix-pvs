package domain

import (
	"fmt"
	"strings"
)

// OrderStatus representa el estado interno de una orden en nuestro
// sistema. NO es el mismo que el status de GS (1-6) ni el de PVS
// (stateId 6/5/4/3). Es nuestro propio state machine.
type OrderStatus string

const (
	OrderReceived         OrderStatus = "RECEIVED"          // GS nos envio la orden
	OrderQRRequested      OrderStatus = "QR_REQUESTED"      // Pedimos QR a PVS
	OrderQRShown          OrderStatus = "QR_SHOWN"          // QR devuelto a GS, esperando pago
	OrderPaymentConfirmed OrderStatus = "PAYMENT_CONFIRMED" // PVS confirmo el pago (stateId=5)
	OrderDone             OrderStatus = "DONE"              // GS entrego el producto (outStockStatus=2)
	OrderFailed           OrderStatus = "FAILED"            // Error irrecuperable
	OrderTimeout          OrderStatus = "TIMEOUT"           // QR expiro sin pago
	OrderCancelled        OrderStatus = "CANCELLED"         // Cancelado por GS
	OrderRefundPending    OrderStatus = "REFUND_PENDING"    // GS pidio reembolso, esperando PVS
	OrderRefunded         OrderStatus = "REFUNDED"          // PVS confirmo el reverse
	OrderRefundFailed     OrderStatus = "REFUND_FAILED"     // El reverse contra PVS fallo
)

// PVSStatus es el estado que PVS reporta para un QR.
// Mapea directamente de stateId: 6=IN_PROCESS, 5=APPROVED, 4=REVERSED, 3=REJECTED.
type PVSStatus string

const (
	PVSInProcess PVSStatus = "IN_PROCESS" // 6 - Pendiente de pago
	PVSApproved  PVSStatus = "APPROVED"   // 5 - Pago exitoso
	PVSReversed  PVSStatus = "REVERSED"   // 4 - Reversado (reembolso)
	PVSRejected  PVSStatus = "REJECTED"   // 3 - Rechazado
	PVSExpired   PVSStatus = "EXPIRED"    // QR vencio (lo detectamos nosotros)
)

// GSOrderStatus son los estados 1-6 que maneja la maquina expendedora
// segun el DOCX seccion 2.3.
type GSOrderStatus int

const (
	GSPending       GSOrderStatus = 1 // Pendiente de pago
	GSPaid          GSOrderStatus = 2 // Pagado exitosamente
	GSFailed        GSOrderStatus = 3 // Transaccion fallo
	GSPendingRefund GSOrderStatus = 4 // Pendiente de reembolso
	GSRefunded      GSOrderStatus = 5 // Reembolsado
	GSTimeout       GSOrderStatus = 6 // Tiempo excedido
)

// transitionTable define los estados a los que se puede mover cada estado.
// Es la unica fuente de verdad para validar transiciones.
var transitionTable = map[OrderStatus][]OrderStatus{
	OrderReceived:         {OrderQRRequested, OrderFailed},
	OrderQRRequested:      {OrderQRShown, OrderFailed},
	OrderQRShown:          {OrderPaymentConfirmed, OrderFailed, OrderTimeout, OrderCancelled},
	OrderPaymentConfirmed: {OrderDone, OrderRefundPending, OrderFailed, OrderCancelled},
	OrderRefundPending:    {OrderRefunded, OrderRefundFailed},
	// Terminales / semi-terminales
	OrderDone:         {OrderRefundPending}, // entregado pero reembolsable
	OrderFailed:       {OrderRefundPending}, // fallo post-pago (ej. complete success=false) sigue reembolsable
	OrderTimeout:      {},
	OrderCancelled:    {},
	OrderRefunded:     {},
	OrderRefundFailed: {},
}

// CanTransitionTo devuelve si la transicion de s a nuevo es valida
// segun la transitionTable.
func (s OrderStatus) CanTransitionTo(nuevo OrderStatus) bool {
	for _, permitido := range transitionTable[s] {
		if permitido == nuevo {
			return true
		}
	}
	return false
}

// EsEstadoTerminal devuelve true si el estado no puede transicionar
// a ningun otro (es un estado final).
func (s OrderStatus) EsEstadoTerminal() bool {
	return len(transitionTable[s]) == 0
}

// ToGSStatus traduce nuestro estado interno al status 1-6 que entiende GS.
// Se usa cuando GS hace polling de estado (seccion 2.3 del DOCX).
func (s OrderStatus) ToGSStatus() GSOrderStatus {
	switch s {
	case OrderReceived, OrderQRRequested, OrderQRShown:
		return GSPending // 1
	case OrderPaymentConfirmed, OrderDone:
		return GSPaid // 2
	case OrderFailed, OrderCancelled:
		return GSFailed // 3
	case OrderRefundPending:
		return GSPendingRefund // 4
	case OrderRefunded, OrderRefundFailed:
		return GSRefunded // 5
	case OrderTimeout:
		return GSTimeout // 6
	default:
		// Estado desconocido: fallback seguro (GS reintenta o cancela).
		return GSFailed
	}
}

// PVSStatusFromStateID mapea el stateId numerico de PVS (6/5/4/3) a PVSStatus.
// stateId: 6=In Process, 5=Approved, 4=Reverse, 3=Rejected.
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

// PVSStatusFromCallback mapea status texto del callback PVS.
// Doc: APPROVED | REJECTED (+ REVERSED/IN_PROCESS por compat).
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
