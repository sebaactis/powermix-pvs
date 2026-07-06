package domain

import (
	"testing"
)

// TestToGSStatus: verifica que cada estado interno mapea al
// status 1-6 correcto que entiende la maquina GS.
func TestToGSStatus(t *testing.T) {
	tests := []struct {
		name   string
		estado OrderStatus
		espera GSOrderStatus
	}{
		// Pendientes de pago -> 1
		{"received", OrderReceived, GSPending},
		{"qr_requested", OrderQRRequested, GSPending},
		{"qr_shown", OrderQRShown, GSPending},
		// Pagados -> 2
		{"payment_confirmed", OrderPaymentConfirmed, GSPaid},
		{"done", OrderDone, GSPaid},
		// Fallidos -> 3
		{"failed", OrderFailed, GSFailed},
		{"cancelled", OrderCancelled, GSFailed},
		// Reembolso -> 4 y 5
		{"refund_pending", OrderRefundPending, GSPendingRefund},
		{"refunded", OrderRefunded, GSRefunded},
		{"refund_failed", OrderRefundFailed, GSRefunded},
		// Timeout -> 6
		{"timeout", OrderTimeout, GSTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.estado.ToGSStatus()
			if got != tt.espera {
				t.Errorf("ToGSStatus(%q) = %d, esperaba %d",
					tt.estado, got, tt.espera)
			}
		})
	}
}

// TestPVSStatusFromStateID: verifica el mapeo stateId -> PVSStatus.
func TestPVSStatusFromStateID(t *testing.T) {
	tests := []struct {
		stateID int
		espera  PVSStatus
		wantErr bool
	}{
		{6, PVSInProcess, false},
		{5, PVSApproved, false},
		{4, PVSReversed, false},
		{3, PVSRejected, false},
		{99, "", true},   // desconocido
		{0, "", true},    // invalido
		{-1, "", true},   // negativo
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got, err := PVSStatusFromStateID(tt.stateID)
			if tt.wantErr && err == nil {
				t.Errorf("PVSStatusFromStateID(%d) esperaba error, fue nil", tt.stateID)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("PVSStatusFromStateID(%d) error inesperado: %v", tt.stateID, err)
			}
			if !tt.wantErr && got != tt.espera {
				t.Errorf("PVSStatusFromStateID(%d) = %q, esperaba %q",
					tt.stateID, got, tt.espera)
			}
		})
	}
}
