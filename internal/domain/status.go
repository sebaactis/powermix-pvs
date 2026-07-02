package domain

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
	// Terminales: no pueden transicionar a ningun otro estado
	OrderDone:       {},
	OrderFailed:     {},
	OrderTimeout:    {},
	OrderCancelled:  {},
	OrderRefunded:   {},
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
