# 📋 RESUMEN DEL PROYECTO — vps-powermix

**Integración entre máquina expendedora GSWYIT (GS) y proveedor de pagos QR (PVS)**

---

## 🎯 Objetivo

Servicio Go que actúa de **bridge** entre la máquina expendedora GSWYIT (GS) y el proveedor argentino de pagos QR (PVS). El cliente selecciona una bebida, la máquina genera un QR de pago a través de nuestro servicio, el cliente paga con su billetera, y la máquina dispensa automáticamente.

---

## 🏗️ Arquitectura

```
    ┌─────────────────────┐
    │  GS (GWYIT Machine) │  ←— POST /api/v1/orders, /api/v1/orders/{orderNo}/query,
    │  (China, Android)   │      /api/v1/orders/{orderNo}/refund,
    └──────────┬──────────┘      /api/v1/orders/{orderNo}/complete,
               │ HTTP (key-md5 signed)  /api/v1/orders/{orderNo}/cancel
    ┌──────────▼──────────┐
    │  vps-powermix (Go)  │  5 endpoints inbound + 1 PVS webhook
    │                     │  + reconciler + sync log + /healthz + /metrics
    └──────────┬──────────┘
               │ HTTP (OAuth2 Bearer)
    ┌──────────▼──────────┐
    │  PVS (QR Provider)  │  QR generate, status, reverse
    │  (Argentina)        │
    └─────────────────────┘
```

**Patrón de comunicación: Path A (polling-only).** GS consulta el estado de la orden vía polling. Nosotros nunca enviamos notificaciones de pago a GS.

---

## ✅ Estado de implementación — COMPLETADO

### PR1 — Foundation ✅
| Componente | Archivos | Tests | Estado |
|---|---|---|---|
| `internal/config/` | `config.go`, `config_test.go` | ✅ 5 (19 sub) | ✅ |
| `internal/domain/` | `order.go`, `status.go`, `money.go`, `errors.go`, `refund.go`, `order_test.go`, `status_test.go` | ✅ | ✅ |
| `internal/ports/` | `repository.go`, `client_gs.go`, `client_pvs.go`, `health.go`, `ports_test.go` | ✅ | ✅ |
| `migrations/` | `001-005 up/down SQL`, `migration_test.go` | ✅ (skip sin DB) | ✅ |
| `internal/store/` | `order_repo.go`, `idempotency_store.go`, `sync_log_repo.go`, `reconciler_store.go`, `refund_repo.go`, `scanning.go`, `store_test.go` | ✅ (skip sin DB) | ✅ |

### PR2 — PVS Client ✅
| Componente | Archivos | Tests | Estado |
|---|---|---|---|
| `internal/client/pvs/` | `client.go`, `client_test.go` | ✅ 8 | ✅ |

### PR3 — GS Client ✅
| Componente | Archivos | Tests | Estado |
|---|---|---|---|
| `internal/client/gs/` | `client.go`, `client_test.go` | ✅ 6 | ✅ |

### PR4 — Handlers + State Machine + Reconciler ✅
| Componente | Archivos | Tests | Estado |
|---|---|---|---|
| `internal/handler/` | `handler.go`, `metrics.go`, `redact.go`, `handler_test.go`, `e2e_test.go`, `security_test.go` | ✅ | ✅ |
| `internal/service/` | `order_service.go`, `order_service_test.go`, `refund_service.go`, `refund_service_test.go` | ✅ | ✅ |
| `internal/reconciler/` | `reconciler.go`, `reconciler_test.go` | ✅ | ✅ |
| `cmd/server/` | `main.go` | — | ✅ |
| `migrations/` | `004_refunds`, `005_reconciler_runs` | ✅ | ✅ |

### PR5 — Observability + E2E + Tests de Seguridad ✅
| Componente | Detalle | Estado |
|---|---|---|
| Reconciler loop completo | Goroutine + ticker, batch de 200, stuck thresholds QR_SHOWN/REFUND_PENDING | ✅ |
| `slog` redaction handler | `RedactingHandler` en `handler/redact.go` | ✅ |
| Prometheus metrics | `http_requests_total`, `http_request_duration_seconds`, `pvs_calls_total`, `pvs_call_duration_seconds`, `reconciler_runs_total`, `reconciler_fixed_total` | ✅ |
| `/healthz` | DB ping + respuesta structured JSON | ✅ |
| `/metrics` | Endpoint Prometheus en handler/metrics.go | ✅ |
| Tests E2E HTTP | `e2e_test.go`: flujo crear→pagar→completar, refund flow, cancel flow | ✅ |
| Tests de seguridad | `security_test.go` | ✅ |

---

## 📦 Stack

| Capa | Tecnología |
|---|---|
| Lenguaje | **Go** |
| Base de datos | **PostgreSQL** (golang-migrate, sqlx + pgx v5) |
| HTTP | `net/http` (Go 1.22+ ServeMux) |
| OAuth2 cache | `golang.org/x/sync/singleflight` |
| Rate limiter | `golang.org/x/time/rate` |
| Logs | `log/slog` con `RedactingHandler` |
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
├── cmd/server/          ✅ main.go — wiring completo
├── internal/
│   ├── config/          ✅ Configuración por env vars
│   ├── domain/          ✅ Entidades del negocio + state machine
│   ├── ports/           ✅ Interfaces (puertos)
│   ├── client/
│   │   ├── pvs/         ✅ Cliente HTTP PVS (OAuth2 + QR + reverse)
│   │   └── gs/          ✅ Cliente HTTP GS (key-md5 + query + refund)
│   ├── handler/         ✅ HTTP handlers + middleware + metrics + redact
│   ├── service/         ✅ Lógica de negocio + state machine
│   ├── reconciler/      ✅ Worker background con batch logic
│   └── store/           ✅ Implementaciones Postgres
├── migrations/          ✅ 5 migraciones (orders, idempotency, sync_log, refunds, reconciler_runs)
├── docs/                📄 Documentación de análisis
└── go.mod / go.sum      ✅ Dependencias
```

---

## 📊 Tests

```
go test ./...
```

| Paquete | Resultado |
|---|---|
| `internal/client/gs` | ✅ ok |
| `internal/client/pvs` | ✅ ok |
| `internal/config` | ✅ ok |
| `internal/domain` | ✅ ok |
| `internal/handler` | ✅ ok |
| `internal/ports` | ✅ ok |
| `internal/reconciler` | ✅ ok |
| `internal/service` | ✅ ok |
| `internal/store` | ✅ ok (skip sin Postgres) |
| `migrations` | ✅ ok (skip sin Postgres) |

**10 paquetes, 0 fallos.**
