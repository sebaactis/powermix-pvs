package domain

import "time"

// RefundStatus representa el estado de un reembolso en nuestro sistema.
type RefundStatus string

const (
	RefundPending RefundStatus = "PENDING"  // Solicitado, esperando PVS
	RefundSuccess RefundStatus = "SUCCESS"  // PVS confirmo el reverse
	RefundFailed  RefundStatus = "FAILED"   // El reverse contra PVS fallo
)

// Refund representa un reembolso solicitado por GS.
type Refund struct {
	ID          int64
	RefundNo    string       // numero de reembolso de GS (idempotencia)
	OrderNo     string       // orden original
	PriceCents  int64        // monto del reembolso en centavos
	Motivo      string       // razon del reembolso
	Status      RefundStatus

	// PVS
	PvsReverseID string // ID del reverse en PVS

	// GS
	GSRefundNo string // numero de reembolso de GS (puede ser diferente)

	// Timestamps
	RequestedAt  time.Time
	CompletedAt  time.Time
	Error        string

	CreatedAt time.Time
	UpdatedAt time.Time
}
