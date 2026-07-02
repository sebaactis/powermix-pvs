# 📋 RESUMEN DEL PROYECTO — vps-powermix

**Integración entre máquina expendedora GSWYIT (GS) y proveedor de pagos QR (PVS)**

---

## 🎯 Objetivo

Servicio Go 1.25.1 que actúa de **bridge** entre la máquina expendedora GSWYIT (GS) y el proveedor argentino de pagos QR (PVS). El cliente selecciona una bebida, la máquina genera un QR de pago a través de nuestro servicio, el cliente paga con su billetera, y la máquina dispensa automáticamente.

---

## 🏗️ Arquitectura

```
    ┌─────────────────────┐
    │  GS (GWYIT Machine) │  ←— POST /order/create, /order/query,
    │  (China, Android)   │      /order/refund, /order/complete,
    └──────────┬──────────┘      /order/cancel
               │ HTTP (key-md5 signed)
    ┌──────────▼──────────┐
    │  vps-powermix (Go)  │  5 endpoints inbound + 1 PVS webhook
    │                     │  + reconciler + sync log + healthz
    └──────────┬──────────┘
               │ HTTP (OAuth2 Bearer)
    ┌──────────▼──────────┐
    │  PVS (QR Provider)  │  QR generate, status, reverse
    │  (Argentina)        │
    └─────────────────────┘
```

**Patrón de comunicación: Path A (polling-only).** GS consulta el estado de la orden vía polling. Nosotros nunca enviamos notificaciones de pago a GS.

---

## ✅ Estado de implementación

### PR1 — Foundation (COMPLETO)
| Componente | Archivos | Tests | Estado |
|---|---|---|---|
| `internal/config/` | `config.go`, `config_test.go` | ✅ 5 (19 sub) | ✅ |
| `internal/domain/` | `order.go`, `status.go`, `money.go`, `errors.go`, `refund.go`, `order_test.go` | ✅ 5 (33 sub) | ✅ |
| `internal/ports/` | `repository.go`, `client_gs.go`, `client_pvs.go`, `health.go`, `ports_test.go` | ✅ 1 | ✅ |
| `migrations/` | `001-003 up/down SQL`, `migration_test.go` | ✅ 1 (skip sin DB) | ✅ |
| `internal/store/` | `order_repo.go`, `idempotency_store.go`, `sync_log_repo.go`, `store_test.go` | ✅ 6 (skip sin DB) | ✅ |

### PR2 — PVS Client (COMPLETO)
| Componente | Archivos | Tests | Estado |
|---|---|---|---|
| `internal/client/pvs/` | `client.go`, `client_test.go` | ✅ 8 | ✅ |

### PR3 — GS Client (COMPLETO)
| Componente | Archivos | Tests | Estado |
|---|---|---|---|
| `internal/client/gs/` | `client.go`, `client_test.go` | ✅ 6 | ✅ |

### PR4 — Handlers + State Machine + Reconciler (PENDIENTE)
- 5 handlers para GS: `POST /v1/gs/orders` (create), `POST /v1/gs/orders/{orderNo}/query`, `POST /v1/gs/orders/{orderNo}/refund`, `POST /v1/gs/orders/{orderNo}/complete`, `POST /v1/gs/orders/{orderNo}/cancel`
- 1 handler para PVS webhook: `POST /v1/pvs/webhook`
- Service layer con state machine (transición de estados, row-lock con FOR UPDATE, traducción qrImage→qrUrl)
- Reconciler skeleton (goroutine + ticker, sin lógica de batch todavía)
- Migraciones 004-005 (reconciler_runs + refunds)

### PR5 — Observability + E2E + Load (PENDIENTE)
- Reconciler loop completo (batch logic, stuck thresholds 180s/300s)
- slog redaction Handler
- Prometheus metrics (10 métricas)
- `/healthz` con DB ping + clock drift
- Golden-file HTTP tests
- E2E con docker-compose + load test (vegeta/k6) + security audit

---

## 📦 Stack

| Capa | Tecnología |
|---|---|
| Lenguaje | **Go 1.25.1** |
| Base de datos | **PostgreSQL** (golang-migrate, sqlx + pgx v5) |
| HTTP | `net/http` (Go 1.22+ ServeMux) |
| OAuth2 cache | `golang.org/x/sync/singleflight` |
| Rate limiter | `golang.org/x/time/rate` |
| Logs | `log/slog` con Handler redaction |
| Métricas | `prometheus/client_golang` |
| Tests | `testing` + `httptest` (sin Docker para unit) |

---

## 🔑 Decisiones arquitectónicas claves

| Decisión | Detalle |
|---|---|
| **Path A (polling)** | GS consulta estado vía polling. Sin webhook de nosotros → GS |
| **Currency ARS fijo** | Solo ARS, sin multi-moneda |
| **No verify GS key-md5** | GS no ha confirmado que firme sus requests entrantes |
| **qrImage → qrUrl** | PVS devuelve base64 (qrImage). GS espera qrUrl (base64). Service traduce |
| **No PVS webhook signature** | Aceptado por riesgo (red cerrada) |
| **Dedup 20s GS** | `(deviceId + objectId + price_cents)` en ventana de 20s |
| **Refunds en v1** | GS-driven: GS pide refund → nosotros llamamos PVS reverse |
| **Comentarios en español** | Por requerimiento del usuario |

---

## 📁 Estructura del proyecto

```
vps-powermix/
├── cmd/server/          (pendiente PR4)
├── internal/
│   ├── config/          ✅ Configuración por env vars
│   ├── domain/          ✅ Entidades del negocio
│   ├── ports/           ✅ Interfaces (puertos)
│   ├── client/
│   │   ├── pvs/         ✅ Cliente HTTP PVS (OAuth2 + QR + reverse)
│   │   └── gs/          ✅ Cliente HTTP GS (key-md5 + query + refund)
│   ├── handler/         (pendiente PR4)
│   ├── service/         (pendiente PR4)
│   ├── reconciler/      (pendiente PR4+PR5)
│   ├── observability/   (pendiente PR5)
│   └── store/           ✅ Implementaciones Postgres
├── migrations/          ✅ 3 migraciones (orders, idempotency, sync_log)
├── docs/                📄 Documentación de análisis
├── go.mod / go.sum      ✅ Dependencias
└── Makefile             (pendiente)
```

---

## 📊 Tests

```
go test -count=1 ./internal/config/ ./internal/domain/ ./internal/ports/
             ./internal/client/pvs/ ./internal/client/gs/
```

**Total: 22 tests, 0 fallos.** (6 tests de store/migrations skip sin Postgres local)

---

## ⏳ Pendiente más inmediato (PR4)

- **Endpoint GS `order complete`**: confirmar payload exacto con vendor (asumimos outStockStatus + outStockTime del DOCX)
- **Endpoint GS `order cancel`**: confirmar payload + triggers con vendor
- **Resolución OQ-1**: confirmar PVS OAuth2 form body con curl en sandbox (opcional, implementación asume standard)
