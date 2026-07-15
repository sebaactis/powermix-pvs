package domain

import "fmt"

// Money representa una cantidad de dinero. Siempre en centavos para
// evitar problemas de floats (redondeo, comparacion, precision).
type Money struct {
	Cents int64 // centavos, ej: 15050 = $150.50
}

func NewMoney(cents int64) Money {
	return Money{Cents: cents}
}

func (m Money) String() string {
	return fmt.Sprintf("ARS %d.%02d", m.Cents/100, m.Cents%100)
}

func (m Money) EsZero() bool { return m.Cents == 0 }

func (m Money) EsValida() bool { return m.Cents > 0 }
