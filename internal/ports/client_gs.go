package ports

import "context"

// GSNotifyPaymentRequest es el POST saliente a notifyUrl (GS Open API v2).
// Machine Server = GS; nosotros somos Third Party.
type GSNotifyPaymentRequest struct {
	NotifyURL     string // URL absoluta; no se concatena con baseURL
	OrderNo       string // serial de GS
	ThirdOrderNo  string // nuestro id
	OrderStatus   string // "2" en este change (pago exitoso)
	OrderTime     string // yyyy-MM-dd HH:mm:ss UTC
	PayTime       string
	TotalAmount   string
	ChannelUserID string
}

// GSNotifyPaymentResponse es lo que nos interesa de la respuesta de GS.
type GSNotifyPaymentResponse struct {
	ReturnCode string
	ReturnMsg  string
}

// GSClient define las llamadas HTTP salientes hacia GS.
// En v2 el inbound es handler nuestro; el outbound es notify de pago.
type GSClient interface {
	NotifyPayment(ctx context.Context, req *GSNotifyPaymentRequest) (*GSNotifyPaymentResponse, error)
}
