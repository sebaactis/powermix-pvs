package ports

import "context"

// GSQueryRequest es el payload para consultar el estado de una orden en GS.
type GSQueryRequest struct {
	OrderNo     string // numero de orden de GS
	ThirdOrderNo string // nuestro numero de orden
}

// GSQueryResponse es la respuesta de GS a una consulta de estado.
type GSQueryResponse struct {
	OrderNo     string
	ThirdOrderNo string
	OrderStatus  int // 1-6 segun DOCX
}

// GSRefundRequest es el payload para solicitar un reembolso a GS.
type GSRefundRequest struct {
	RefundNo        string // numero de reembolso (idempotencia)
	OrderNo         string // orden original de GS
	ThirdOrderNo    string // nuestro numero de orden
	RefundAmount    string // monto en string, ej: "100.50"
	RefundReason    string // motivo del reembolso
	RefundNotifyURL string // webhook para notificar resultado
}

// GSRefundResponse es la respuesta de GS a una solicitud de reembolso.
type GSRefundResponse struct {
	RefundNo     string
	OrderNo      string
	ThirdOrderNo string
	RefundStatus string // "success" o "fail"
}

// GSClient define las llamadas HTTP que hacemos hacia la maquina GS.
// Solo las necesarias para Path A (polling): consultar estado y pedir
// reembolso. No tenemos notificaciones salientes de pago.
type GSClient interface {
	QueryStatus(ctx context.Context, req *GSQueryRequest) (*GSQueryResponse, error)
	Refund(ctx context.Context, req *GSRefundRequest) (*GSRefundResponse, error)
}
