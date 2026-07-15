package domain

import "time"

type Order struct {
	ID           int64
	ThirdOrderNo string // nuestro id; GS lo llama thirdOrderNo
	GsOrderNo    string // serial de GS; GS lo llama orderNo
	DeviceID     string
	DeviceNo     string

	ObjectID   string
	PriceCents int64 // siempre entero; moneda fija ARS

	PayMethod string
	WayCode   string

	Status        OrderStatus
	GsOrderStatus GSOrderStatus
	PvsStatus     PVSStatus

	PvsQrID    string
	PvsQrImage string // base64 devuelto por PVS → qrUrl hacia GS

	NotifyURL    string    // callback absoluto hacia GS (v2)
	GsNotifiedAt time.Time // zero = notify pendiente/fallido

	QrGeneratedAt      time.Time
	QrExpiresAt        time.Time
	PaymentConfirmedAt time.Time
	GsCompletedAt      time.Time
	GsCancelledAt      time.Time
	RefundedAt         time.Time

	FailureReason string
	RequestID     string // correlación HTTP de origen

	CreatedAt time.Time
	UpdatedAt time.Time
}
