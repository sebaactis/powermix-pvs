# Guía de Pruebas Locales (API + PVS Mock) — GS Open API v2

Esta guía describe cómo probar el ciclo de vida de `vps-powermix` localmente con **Payment Open API v2** (`/order/*`, envelope `{code,msg,data}`).

---

## Camino Rápido

1. **Config:** copiá `.env.example` → `.env` y setea `DATABASE_URL`.
2. **Migraciones:** corré las migraciones bajo `migrations/` (incluye campos GS v2: `gs_order_no`, `notify_url`, `gs_notified_at`, rename `third_order_no`).
3. **Mock PVS:**

   ```bash
   go run cmd/mockpvs/main.go
   ```

   Escucha en `http://localhost:8081`.
4. **Bridge:**

   ```bash
   go run cmd/server/main.go
   ```

   Escucha en `http://localhost:8080`.

---

## Dashboard GS (checklist de URLs)

Configurá en el panel de la máquina:

| Campo | URL |
| --- | --- |
| Order create URL | `https://<host>/order/qr` |
| Order Query URL | `https://<host>/order/status` |
| Order Refund URL | `https://<host>/order/refund` |
| Order Complete URL | `https://<host>/order/complete` |
| Order Cancel URL | `https://<host>/order/cancel` |

Opcional API: `POST /order/refundStatus`.

**IDs:** `orderNo` = serial de GS · `thirdOrderNo` = nuestro id.

---

## Flujos

### 1. Feliz: create → pay → status → complete

#### A. Create (`POST /order/qr`)

```bash
curl -X POST http://localhost:8080/order/qr \
  -H "Content-Type: application/json" \
  -d '{
    "orderNo":"GS-LOCAL-001",
    "objectId":"bebida-001",
    "subject":"Batido",
    "attach":"deviceNo=E001&deviceId=maquina-local-1",
    "totalAmount":"150.00",
    "notifyUrl":"https://gs.example/notify"
  }'
```

Respuesta esperada (envelope):

```json
{"code":200,"msg":"...","data":{"qrUrl":"...","orderStatus":"1","thirdOrderNo":"..."}}
```

Guardá `thirdOrderNo` y el `qrId` del mock PVS.

#### B. Webhook pago PVS (callback oficial)

```bash
curl -X POST "http://localhost:8080/webhook/pvs?qr.reference=<thirdOrderNo>" \
  -H "Content-Type: application/json" \
  -d '{
    "reference":"<thirdOrderNo>",
    "amount":150.00,
    "qrId":"<qrId>",
    "txeId":"txe-mock-1",
    "status":"APPROVED",
    "notified_at":"2024-10-10T18:00:23Z"
  }'
```

Doc PVS: `status` APPROVED|REJECTED. Dispara `PAYMENT_CONFIRMED` + notify best-effort a `notifyUrl` (`orderStatus "2"`).

#### C. Query status

```bash
curl -X POST http://localhost:8080/order/status \
  -H "Content-Type: application/json" \
  -d '{"orderNo":"GS-LOCAL-001","thirdOrderNo":"<thirdOrderNo>"}'
```

`orderStatus` string (`"2"` = pagado).

#### D. Complete (entrega OK)

```bash
curl -X POST http://localhost:8080/order/complete \
  -H "Content-Type: application/json" \
  -d '{
    "orderNo":"GS-LOCAL-001",
    "thirdOrderNo":"<thirdOrderNo>",
    "success":true,
    "orderStatus":2,
    "outStockStatus":2,
    "outStockTime":"2026-07-10 12:00:00"
  }'
```

→ orden `DONE`. `returnCode: "success"`.

---

### 2. Cancel (antes de pagar)

```bash
# create (igual que arriba)
curl -X POST http://localhost:8080/order/cancel \
  -H "Content-Type: application/json" \
  -d '{
    "orderNo":"GS-LOCAL-001",
    "thirdOrderNo":"<thirdOrderNo>",
    "orderStatus":0,
    "remark":"user cancel",
    "cancelTime":"2026-07-10 12:00:00"
  }'
```

→ `CANCELLED`.

---

### 3. Complete fail → refund

1. Create + webhook `status: APPROVED`.
2. Complete con `success:false`:

```bash
curl -X POST http://localhost:8080/order/complete \
  -H "Content-Type: application/json" \
  -d '{
    "orderNo":"GS-LOCAL-001",
    "thirdOrderNo":"<thirdOrderNo>",
    "success":false,
    "orderStatus":2,
    "outStockStatus":1
  }'
```

→ orden `FAILED` (refundable si hubo `payment_confirmed_at`).

1. Refund:

```bash
curl -X POST http://localhost:8080/order/refund \
  -H "Content-Type: application/json" \
  -d '{
    "orderNo":"GS-LOCAL-001",
    "thirdOrderNo":"<thirdOrderNo>",
    "refundNo":"REF-1001",
    "refundAmount":"150.00",
    "refundReason":"Falla en espiral"
  }'
```

`refundStatus` inmediato suele ser `"waiting"`.

1. Confirmar reverse PVS (flujo local; callback oficial de pago solo documenta APPROVED|REJECTED):

```bash
curl -X POST "http://localhost:8080/webhook/pvs?qr.reference=<thirdOrderNo>" \
  -H "Content-Type: application/json" \
  -d '{
    "reference":"<thirdOrderNo>",
    "qrId":"<qrId>",
    "status":"REVERSED",
    "txeId":"txe-rev-1"
  }'
```

1. Query refund:

```bash
curl -X POST http://localhost:8080/order/refundStatus \
  -H "Content-Type: application/json" \
  -d '{"orderNo":"GS-LOCAL-001","thirdOrderNo":"<thirdOrderNo>","refundNo":"REF-1001"}'
```

→ `pending` | `success` | `fail`.

---

## Reconciler

- Reintenta notify GS para `PAYMENT_CONFIRMED` sin `gs_notified_at` (edad ≥ 30s).
- Si se pierde el webhook de pago: forzá estado en mock PVS y esperá el scan:

```bash
curl -X POST http://localhost:8081/admin/transactions/<qrId>/status \
  -H "Content-Type: application/json" \
  -d '{"stateId":5}'
```

---

## Ops

- Health: `GET http://localhost:8080/healthz`
- Metrics: `GET http://localhost:8080/metrics`
- Tests: `go test ./...`
- Postman: `docs/vps-powermix.postman_collection.json`
  - Variables de colección (`baseUrl`, `orderNo`, `thirdOrderNo`, `qrId`, `refundNo`, …)
  - Flujos numerados: **01 feliz**, **02 cancel**, **03 fail→refund**, **04 reconciler**
  - Cada Create regenera IDs; correr los requests de la carpeta **en orden**
