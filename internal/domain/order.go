// Package domain contiene las entidades del negocio, las reglas de
// transicion de estado, y los tipos de error del sistema.
// Es el circulo mas interno de la arquitectura hexagonal: NO importa
// NADA del exterior (ni base de datos, ni HTTP, ni config).
package domain

import "time"

// Order es la entidad central del sistema. Representa un pedido entre
// la maquina expendedora (GS) y el proveedor de pagos (PVS).
type Order struct {
	// Identificadores
	ID           int64
	ThirdOrderNo string // nuestro id (SQL third_order_no); GS lo llama thirdOrderNo
	GsOrderNo    string // serial de GS (SQL gs_order_no); GS lo llama orderNo
	DeviceID     string // ID del dispositivo GS
	DeviceNo     string // Numero de serie del dispositivo

	// Producto
	ObjectID   string // SKU del producto (bebida)
	PriceCents int64  // Precio en centavos, SIEMPRE entero
	// Currency es siempre ARS. No manejamos otra moneda.

	// Metodo de pago original que envio GS
	PayMethod string // ej: "wxpay", "alipay"
	WayCode   string // ej: "qr"

	// Estados
	Status        OrderStatus   // nuestro estado interno
	GsOrderStatus GSOrderStatus // 1-6 de la maquina GS
	PvsStatus     PVSStatus     // estado reportado por PVS

	// QR (PVS)
	PvsQrID    string // ID interno del QR en PVS
	PvsQrImage string // QR en base64 (lo que devuelve PVS)

	// Callback GS (Open API v2)
	NotifyURL    string    // URL absoluta donde avisamos el pago a GS
	GsNotifiedAt time.Time // cuando notificamos con exito (zero = pendiente)

	// Timestamps de ciclo de vida
	QrGeneratedAt      time.Time
	QrExpiresAt        time.Time
	PaymentConfirmedAt time.Time // cuando PVS confirmo el pago (stateId=5)
	GsCompletedAt      time.Time // cuando GS aviso que entrego (outStockStatus=2)
	GsCancelledAt      time.Time // cuando GS cancelo la orden
	RefundedAt         time.Time // cuando PVS confirmo el reverse

	// Error
	FailureReason string // motivo de falla, timeout, cancelacion

	// Metadata
	CreatedAt time.Time
	UpdatedAt time.Time
}
