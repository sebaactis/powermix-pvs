package domain

import "fmt"

// Money representa una cantidad de dinero. Siempre en centavos para
// evitar problemas de floats (redondeo, comparacion, precision).
// La moneda es siempre ARS por definicion del negocio.
type Money struct {
	Cents int64 // centavos, ej: 15050 = $150.50
}

// NewMoney crea un Money a partir de centavos.
func NewMoney(cents int64) Money {
	return Money{Cents: cents}
}

// String devuelve una representacion legible, ej: "ARS 150.50".
func (m Money) String() string {
	return fmt.Sprintf("ARS %d.%02d", m.Cents/100, m.Cents%100)
}

// EsZero devuelve true si la cantidad es 0.
func (m Money) EsZero() bool { return m.Cents == 0 }

// EsValida devuelve true si la cantidad es positiva.
func (m Money) EsValida() bool { return m.Cents > 0 }
