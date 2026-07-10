package domain

import (
	"errors"
	"testing"
)

// TestOrderStatusTransition: verifica que las transiciones de estado
// permitidas funcionan y las prohibidas son rechazadas.
func TestOrderStatusTransition(t *testing.T) {
	tests := []struct {
		name      string
		desde     OrderStatus
		hacia     OrderStatus
		esperaVal bool
	}{
		// Transiciones validas
		{"received -> qr_requested", OrderReceived, OrderQRRequested, true},
		{"qr_requested -> qr_shown", OrderQRRequested, OrderQRShown, true},
		{"qr_shown -> payment_confirmed", OrderQRShown, OrderPaymentConfirmed, true},
		{"qr_shown -> timeout", OrderQRShown, OrderTimeout, true},
		{"qr_shown -> cancelled", OrderQRShown, OrderCancelled, true},
		{"payment_confirmed -> done", OrderPaymentConfirmed, OrderDone, true},
		{"payment_confirmed -> refund_pending", OrderPaymentConfirmed, OrderRefundPending, true},
		{"done -> refund_pending", OrderDone, OrderRefundPending, true},
		{"failed -> refund_pending", OrderFailed, OrderRefundPending, true},
		{"refund_pending -> refunded", OrderRefundPending, OrderRefunded, true},
		{"refund_pending -> refund_failed", OrderRefundPending, OrderRefundFailed, true},
		// Transiciones invalidas
		{"received -> done (salta pasos)", OrderReceived, OrderDone, false},
		{"received -> payment_confirmed (salta PVS)", OrderReceived, OrderPaymentConfirmed, false},
		{"done -> failed (no permitido)", OrderDone, OrderFailed, false},
		{"failed -> received (no permitido)", OrderFailed, OrderReceived, false},
		{"timeout -> cualquier (terminal)", OrderTimeout, OrderRefundPending, false},
		{"refunded -> cualquier (terminal)", OrderRefunded, OrderRefundPending, false},
		{"received -> timeout (sin QR)", OrderReceived, OrderTimeout, false},
		{"qr_shown -> refund_pending (sin pago)", OrderQRShown, OrderRefundPending, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.desde.CanTransitionTo(tt.hacia)
			if got != tt.esperaVal {
				t.Errorf("CanTransitionTo(%q -> %q) = %v, esperaba %v",
					tt.desde, tt.hacia, got, tt.esperaVal)
			}
		})
	}
}

// TestEstadosTerminales: verifica que los estados terminales reportan
// correctamente que no pueden transicionar.
func TestEstadosTerminales(t *testing.T) {
	terminales := []OrderStatus{
		OrderTimeout, OrderCancelled, OrderRefunded, OrderRefundFailed,
	}

	for _, s := range terminales {
		if !s.EsEstadoTerminal() {
			t.Errorf("se esperaba que %q sea terminal", s)
		}
	}

	noTerminales := []OrderStatus{
		OrderReceived, OrderQRRequested, OrderQRShown,
		OrderPaymentConfirmed, OrderRefundPending, OrderDone,
		OrderFailed, // reembolsable post-pago (GS v2 complete success=false)
	}

	for _, s := range noTerminales {
		if s.EsEstadoTerminal() {
			t.Errorf("no se esperaba que %q sea terminal", s)
		}
	}
}

// TestMoney: verifica creacion, representacion y validacion.
func TestMoney(t *testing.T) {
	t.Run("creacion y string", func(t *testing.T) {
		m := NewMoney(15050)
		esperado := "ARS 150.50"
		if m.String() != esperado {
			t.Errorf("Money.String() = %q, esperaba %q", m.String(), esperado)
		}
	})

	t.Run("es cero", func(t *testing.T) {
		if !NewMoney(0).EsZero() {
			t.Error("NewMoney(0).EsZero() = false, esperaba true")
		}
		if NewMoney(1).EsZero() {
			t.Error("NewMoney(1).EsZero() = true, esperaba false")
		}
	})

	t.Run("es valida", func(t *testing.T) {
		if !NewMoney(100).EsValida() {
			t.Error("NewMoney(100).EsValida() = false, esperaba true")
		}
		if NewMoney(0).EsValida() {
			t.Error("NewMoney(0).EsValida() = true, esperaba false")
		}
	})

	t.Run("valor extrano funciona", func(t *testing.T) {
		m := NewMoney(1)
		if m.String() != "ARS 0.01" {
			t.Errorf("1 centavo = %q, esperaba \"ARS 0.01\"", m.String())
		}
	})
}

// TestErroresTipo: verifica que los errores tipados del dominio
// se pueden identificar con errors.Is().
func TestErroresTipo(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrOrderNotFound", ErrOrderNotFound},
		{"ErrOrderNotCancellable", ErrOrderNotCancellable},
		{"ErrOrderNotRefundable", ErrOrderNotRefundable},
		{"ErrRefundNotFound", ErrRefundNotFound},
		{"ErrDuplicateOrder", ErrDuplicateOrder},
		{"ErrInvalidAmount", ErrInvalidAmount},
		{"ErrInvalidTransition", ErrInvalidTransition},
		{"ErrPVSServiceError", ErrPVSServiceError},
		{"ErrGSServiceError", ErrGSServiceError},
		{"ErrIdempotencyViolation", ErrIdempotencyViolation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.err) {
				t.Errorf("errors.Is(%v, %v) = false, esperaba true", tt.err, tt.err)
			}
		})
	}
}

// TestOrderStruct: verifica que la estructura Order compila y tiene
// los campos esperados. No es un test exhaustivo, solo de cordura.
func TestOrderStruct(t *testing.T) {
	o := Order{
		ThirdOrderNo: "test-001",
		DeviceID:     "dev-1",
		ObjectID:     "drink-001",
		PriceCents:   15000,
		Status:       OrderReceived,
	}

	if o.ThirdOrderNo != "test-001" {
		t.Errorf("ThirdOrderNo no se seteo correctamente")
	}
	if o.Status != OrderReceived {
		t.Errorf("Status deberia ser RECEIVED")
	}
}
