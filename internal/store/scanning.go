package store

import (
	"database/sql"
	"fmt"

	"github.com/seba/vps-powermix/internal/domain"
)

// scanOrderRow escanea una fila de la tabla orders (segun columnasOrden)
// y la mapea a domain.Order. Es un helper compartido entre
// PostgresOrderRepository y PostgresReconcilerStore.
func scanOrderRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*domain.Order, error) {
	var (
		o                       domain.Order
		gsOrderNo               sql.NullString
		pvsQrID                 sql.NullString
		gsNotified              sql.NullString
		qrGen, qrExp            sql.NullString
		payConf, gsComp, gsCanc sql.NullString
		refunded                sql.NullString
	)

	err := scanner.Scan(
		&o.ThirdOrderNo, &gsOrderNo, &o.DeviceID, &o.DeviceNo, &o.ObjectID, &o.PriceCents,
		&o.PayMethod, &o.WayCode, &o.Status, &o.GsOrderStatus, &o.PvsStatus,
		&pvsQrID, &o.PvsQrImage, &o.NotifyURL, &gsNotified,
		&qrGen, &qrExp, &payConf, &gsComp, &gsCanc, &refunded,
		&o.FailureReason, &o.CreatedAt, &o.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("escaneando orden: %w", err)
	}

	o.GsOrderNo = gsOrderNo.String
	o.PvsQrID = pvsQrID.String
	o.GsNotifiedAt = parseNullableTime(gsNotified)
	o.QrGeneratedAt = parseNullableTime(qrGen)
	o.QrExpiresAt = parseNullableTime(qrExp)
	o.PaymentConfirmedAt = parseNullableTime(payConf)
	o.GsCompletedAt = parseNullableTime(gsComp)
	o.GsCancelledAt = parseNullableTime(gsCanc)
	o.RefundedAt = parseNullableTime(refunded)

	return &o, nil
}
