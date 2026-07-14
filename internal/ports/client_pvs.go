package ports

import "context"

// PVSQRRequest es el payload para generar un QR en PVS (Service Mode).
type PVSQRRequest struct {
	Amount      int64  // monto en centavos
	ExternalID  string // nuestro orderNo (PVS lo usa para idempotencia)
	Reference   string // referencia para el webhook de PVS
	CallbackURL string // donde PVS nos va a notificar el resultado
}

// PVSQRResponse es la respuesta de PVS al generar un QR.
type PVSQRResponse struct {
	QrID      string // ID interno del QR en PVS
	QrImage   string // QR en base64 (lo que va a GS como qrUrl)
	ExpiresAt string // timestamp de expiracion (opcional)
}

// PVSQueryResponse es la respuesta de PVS al consultar el estado de un QR.
// stateId: 6=In Process, 5=Approved, 4=Reverse, 3=Rejected.
type PVSQueryResponse struct {
	QrID    string
	StateID int    // 6, 5, 4, o 3
	Status  string // descripcion textual del estado
}

// PVSReverseResponse es la respuesta de PVS a una solicitud de reversa.
type PVSReverseResponse struct {
	Success bool
	Message string
	TxEID   string // id transacción reverse en PVS (data.txeId)
}

// PVSClient define las llamadas HTTP que hacemos hacia PVS.
type PVSClient interface {
	GenerateQR(ctx context.Context, req *PVSQRRequest) (*PVSQRResponse, error)
	QueryStatus(ctx context.Context, qrID string) (*PVSQueryResponse, error)
	Reverse(ctx context.Context, qrID string) (*PVSReverseResponse, error)
}

// TokenCache es un cache de tokens OAuth2 para PVS.
// La implementacion usa singleflight.Group para evitar que N goroutines
// pidan el token simultaneamente cuando expira.
type TokenCache interface {
	// Get devuelve un token valido. Si el cache expiro, pide uno nuevo.
	Get(ctx context.Context) (token string, err error)
	// Invalidate fuerza a Get() a pedir un token nuevo en el proximo llamado.
	Invalidate(ctx context.Context)
}
