# 🎯 DIAGRAMAS Y MATRIZ DE DECISIÓN - Integración GS + PVS QR

---

## 📊 DIAGRAMA DE FLUJO COMPLETO

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         USUARIO FINAL                                       │
│                   (Cliente/Comprador)                                       │
└────────────────────────────────┬────────────────────────────────────────────┘
                                 │
                    1. Inicia proceso de compra
                                 │
                    ┌────────────▼────────────┐
                    │   TU APLICACIÓN         │
                    │  Frontend Web/Mobile    │
                    └────────────┬────────────┘
                                 │
              ┌──────────────────▼──────────────────┐
              │ 2. Solicita crear orden con QR      │
              │    POST /api/payments/create-order  │
              │    {amount, description, ref}      │
              └──────────────────┬──────────────────┘
                                 │
        ┌────────────────────────▼────────────────────────┐
        │            TU BACKEND                           │
        │         (Node/Python/Java)                      │
        │                                                │
        │  ┌─────────────────────────────────────────┐   │
        │  │ 3.1 Llamar a PVS QR                    │   │
        │  │     POST /qr/generate                  │   │
        │  │     {amount, reference, callback}     │   │
        │  └────────────┬────────────────────────────┘   │
        │               │                                │
        │               ▼                                │
        │  ┌─────────────────────────────────────────┐   │
        │  │ PVS QR System                          │   │
        │  │ - Genera QR único                      │   │
        │  │ - Retorna qrUrl + qrId                │   │
        │  │ - Configura webhook                   │   │
        │  └────────────┬────────────────────────────┘   │
        │               │                                │
        │               ▼ qrUrl + qrId                   │
        │  ┌─────────────────────────────────────────┐   │
        │  │ 3.2 Registrar en GS                    │   │
        │  │     POST /payments/order               │   │
        │  │     {orderNo, amount, notifyUrl}      │   │
        │  └────────────┬────────────────────────────┘   │
        │               │                                │
        │               ▼                                │
        │  ┌─────────────────────────────────────────┐   │
        │  │ GS Payment System                      │   │
        │  │ - Crea orden interna                   │   │
        │  │ - Retorna orderId + orderNo            │   │
        │  │ - Configura webhook                   │   │
        │  └────────────┬────────────────────────────┘   │
        │               │                                │
        │               ▼ orderId + orderNo              │
        │  ┌─────────────────────────────────────────┐   │
        │  │ 3.3 Guardar en BD Local               │   │
        │  │     - Vincular PVS ID con GS ID       │   │
        │  │     - Guardar URLs y estados           │   │
        │  └────────────┬────────────────────────────┘   │
        │               │                                │
        │               ▼ qrUrl                          │
        └────────────────┬───────────────────────────────┘
                         │
                    4. Retorna QR al usuario
                         │
         ┌───────────────▼───────────────┐
         │  USUARIO VE EL QR             │
         │  - Escanea con móvil          │
         │  - Completa pago              │
         │                               │
         │  >>> PAGO REALIZADO <<<       │
         └───────────────┬───────────────┘
                         │
                    ┌────┴────┐
                    │          │
        ┌───────────▼──┐   ┌───▼────────────┐
        │  PVS RECIBE  │   │  GS RECIBE     │
        │  CONFIRMACIÓN│   │  CONFIRMACIÓN  │
        └───────────┬──┘   └───┬────────────┘
                    │          │
                    └────┬─────┘
                         │
                    5. Envían webhooks
                         │
      ┌──────────────────▼──────────────────┐
      │    TU BACKEND RECIBE WEBHOOKS       │
      │                                     │
      │  POST /webhook/pvs                 │
      │  POST /webhook/gs                  │
      │                                     │
      │  - Actualiza estado en BD          │
      │  - Notifica al frontend            │
      │  - Dispara acciones (email, etc)  │
      └──────────────────┬──────────────────┘
                         │
                    6. Actualiza BD
                         │
      ┌──────────────────▼──────────────────┐
      │  FRONTEND RECIBE ACTUALIZACIÓN      │
      │  - Muestra confirmación             │
      │  - Cierra modal de pago             │
      │  - Redirige a orden                 │
      └──────────────────┬──────────────────┘
                         │
                    ✅ ORDEN PAGADA
```

---

## 🔄 FLUJO DE WEBHOOK (Detallado)

```
ESCENARIO: Usuario paga exitosamente

INICIO
  │
  ├─► Usuario escanea QR
  │   └─► Realiza pago en gateway externo (WeChat, Alipay, etc)
  │
  ├─► PVS detecta pago
  │   └─► Estado: success
  │       └─► Llamada: POST /webhook/pvs
  │           Headers: Authorization, Content-Type
  │           Body: {
  │             event: "payment.completed",
  │             qrId: "qr_123456",
  │             reference: "ORDER-001",
  │             amount: 100.50,
  │             status: "success",
  │             paidAt: "2024-12-06T14:01:15Z"
  │           }
  │
  ├─► Tu Backend recibe webhook de PVS
  │   ├─► Valida autenticidad
  │   ├─► Busca orden en BD
  │   ├─► Actualiza: pvs_status = "success"
  │   ├─► Guarda: webhook_pvs_received = true
  │   ├─► Retorna: {status: "ok"}
  │   │
  │   └─► MIENTRAS TANTO...
  │
  ├─► GS detecta intención de pago
  │   └─► Después de procesamiento
  │       └─► Llamada: POST /webhook/gs
  │           Headers: key, key-md5, timestamp
  │           Body: {
  │             orderNo: "ORDER-001",
  │             orderStatus: 2,  // Success
  │             payTime: "2024-12-06T14:01:15",
  │             payAmt: "100.50",
  │             tradeNo: "4200002450..."
  │           }
  │
  ├─► Tu Backend recibe webhook de GS
  │   ├─► Valida autenticidad
  │   ├─► Busca orden en BD
  │   ├─► Actualiza: order_status = 2
  │   ├─► Guarda: webhook_gs_received = true
  │   ├─► Retorna: {status: "ok"}
  │   │
  │   └─► RECONCILIACIÓN
  │       ├─► Verifica que ambos webhooks llegaron
  │       └─► Si uno falta, dispara polling
  │
  ├─► Tu Backend notifica al frontend
  │   └─► WebSocket: emit('payment_complete')
  │       O Polling: GET /api/payments/{orderNo}/status
  │
  ├─► Frontend actualiza UI
  │   ├─► Cierra modal de pago
  │   ├─► Muestra confirmación
  │   └─► Redirige a dashboard
  │
  └─► FIN - ORDEN PAGADA ✅
```

---

## 🚦 MATRIZ DE DECISIÓN

### ¿Qué hacer en cada escenario?

```
┌─────────────────────────┬──────────────────┬──────────────────────────────┐
│ ESCENARIO               │ CONDICIÓN        │ ACCIÓN RECOMENDADA           │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ Usuario inicia compra    │ N/A              │ 1. POST a /create-order-qr   │
│                         │                  │ 2. Retorna qrUrl             │
│                         │                  │ 3. Mostrar QR al usuario     │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ QR generado en PVS      │ success          │ 1. Guardar qrId en BD        │
│                         │                  │ 2. Registrar en GS           │
│                         │                  │ 3. Estado: pending           │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ QR falla en PVS         │ error            │ 1. Log del error             │
│                         │                  │ 2. Reintentar hasta 3 veces  │
│                         │                  │ 3. Notificar usuario         │
│                         │                  │ 4. Crear ticket de soporte   │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ Pago completado         │ webhook recibido │ 1. Validar firma             │
│                         │ de ambos         │ 2. Actualizar estado en BD   │
│                         │                  │ 3. Notificar al usuario      │
│                         │                  │ 4. Enviar confirmación       │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ Webhook de PVS OK,      │ PVS: success     │ 1. Esperar webhook de GS     │
│ pero falta webhook GS   │ GS: timeout      │ 2. Después de 30 seg, poll   │
│                         │                  │ 3. Si GS = success, sincronizar│
│                         │                  │ 4. Si GS ≠ success, alertar  │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ Webhook de GS OK,       │ GS: success      │ 1. Esperar webhook de PVS    │
│ pero falta webhook PVS  │ PVS: pending     │ 2. Después de 30 seg, poll   │
│                         │                  │ 3. Si PVS = success, sincronizar│
│                         │                  │ 4. Si timeout, marcar como OK│
│                         │                  │    (GS es source of truth)   │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ Ambos webhooks fallidos │ N/A              │ 1. Activar polling cada 5 seg│
│                         │                  │ 2. Consultar GS y PVS        │
│                         │                  │ 3. Actualizar según resultados│
│                         │                  │ 4. Después de 5 min, alertar │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ Usuario solicita refund │ order_status = 2 │ 1. Validar que sea reembolsable│
│                         │ (pagado)         │ 2. POST refund a GS          │
│                         │                  │ 3. Estado: pending_refund    │
│                         │                  │ 4. Esperar confirmación      │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ Reembolso procesado     │ refund_status:   │ 1. Actualizar estado en BD   │
│                         │ success          │ 2. Notificar al usuario      │
│                         │                  │ 3. Generar nota de crédito   │
│                         │                  │ 4. Enviar recibo             │
├─────────────────────────┼──────────────────┼──────────────────────────────┤
│ QR expirado             │ tiempo > timeout │ 1. Cancelar QR en PVS        │
│                         │                  │ 2. Actualizar estado en BD   │
│                         │                  │ 3. Permitir crear nuevo QR   │
│                         │                  │ 4. Notificar al usuario      │
└─────────────────────────┴──────────────────┴──────────────────────────────┘
```

---

## 🎯 MATRIZ DE RESPONSABILIDADES

```
┌──────────────────────┬────────────┬─────────┬───────────┬──────────┐
│ TAREA                │ TU APP     │ PVS QR  │ GS        │ USUARIO  │
├──────────────────────┼────────────┼─────────┼───────────┼──────────┤
│ Generar QR           │ Iniciar    │ Ejecutar│ Registrar │ Recibir  │
│ Almacenar QR ID      │ Guardar    │ Crear   │ Vincular  │ Escanear │
│ Recibir pago         │ Webhook    │ Detecta │ Procesa   │ Paga     │
│ Notificar pago       │ Enviar     │ Notifica│ Notifica  │ Recibe   │
│ Almacenar estado     │ Actualizar │ Reporta │ Confirma  │ Verifica │
│ Confirmar orden      │ Ejecutar   │ -       │ -         │ Recibe   │
│ Reembolsar          │ Iniciar    │ -       │ Procesar  │ Recibe   │
│ Auditoría            │ Loguear    │ Loguear │ Loguear   │ -        │
└──────────────────────┴────────────┴─────────┴───────────┴──────────┘
```

---

## 📊 DIAGRAMA DE ESTADOS

```
                     ┌────────────┐
                     │   START    │
                     └─────┬──────┘
                           │
                    Create Order
                           │
        ┌──────────────────▼──────────────────┐
        │      1. PENDING (Pendiente)          │
        │  - QR generado                      │
        │  - Esperando pago                   │
        │  - Timeout: 30 minutos              │
        └──────────────────┬──────────────────┘
                           │
                    ┌──────┴──────┐
                    │             │
        ┌───────────▼──┐  ┌──────▼──────────┐
        │ USUARIO PAGA │  │ TIMEOUT EXCEDIDO│
        │              │  │                 │
        │  SUCCESS ✅  │  │  6. TIMEOUT ⏱️  │
        └───────────┬──┘  └───────┬─────────┘
                    │             │
                    │      Cancelar orden
                    │             │
        ┌───────────▼────────────▼────────┐
        │  2. PAYMENT SUCCESS (Pagado)     │
        │   - Fondos recibidos              │
        │   - Orden completada              │
        │   - Disponible para reembolso     │
        └────────┬────────────────────┬────┘
                 │                    │
         ┌──────▼─────┐         ┌─────▼──────┐
         │ NO REEMBOLSO│         │  REEMBOLSO │
         │   FINAL ✅  │         │   INICIADO │
         └─────────────┘         └────┬───────┘
                                      │
                        ┌─────────────▼──────────────┐
                        │  4. PENDING REFUND (Pendiente)│
                        │   - Reembolso en proceso    │
                        │   - Esperando confirmación  │
                        └─────────────┬──────────────┘
                                      │
                            ┌─────────┴──────────┐
                            │                    │
                    ┌───────▼────────┐  ┌───────▼────────┐
                    │ REFUND SUCCESS │  │ REFUND FAILED  │
                    │   ✅           │  │   ❌           │
                    └───────┬────────┘  └───────┬────────┘
                            │                   │
        ┌───────────────────▼──────────────────▼──────────┐
        │    5. REFUND COMPLETED (Reembolsado)            │
        │     - Dinero devuelto al usuario                 │
        │     - Orden marcada como reembolsada             │
        │     - Genera recibo de reembolso                 │
        └───────────────┬──────────────────────────────────┘
                        │
                   3. SUCCESS ✅
                        │
                   ┌────▼──────────┐
                   │      END       │
                   └────────────────┘
```

---

## 🔌 DIAGRAMA DE INTEGRACIONES

```
┌─────────────────────────────────────────────────────────────────────┐
│                     TU APLICACIÓN (Backend)                         │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                   Business Logic Layer                       │  │
│  │  - Order Service                                            │  │
│  │  - Payment Orchestration                                    │  │
│  │  - Webhook Handler                                          │  │
│  │  - Reconciliation Engine                                    │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                       │                              │
          ┌────────────┘                              └────────────┐
          │                                                        │
          ▼                                                        ▼
┌──────────────────────────────┐              ┌──────────────────────────────┐
│     GS PAYMENT SYSTEM        │              │    PVS QR SYSTEM             │
│  (China-based)              │              │  (Argentina-based)           │
│                              │              │                              │
│  ┌─────────────────────────┐ │              │  ┌─────────────────────────┐│
│  │ API Endpoints:          │ │              │  │ API Endpoints:          ││
│  │ - /payments/order       │ │              │  │ - /qr/generate          ││
│  │ - /payments/query       │ │              │  │ - /qr/{id}/status       ││
│  │ - /payments/refund      │ │              │  │ - /qr/{id}/cancel       ││
│  │ - /payments/refund/list │ │              │  │ - /qr/list              ││
│  └──────────┬──────────────┘ │              │  └──────────┬──────────────┘│
│             │                │              │             │               │
│  ┌──────────▼──────────────┐ │              │  ┌──────────▼──────────────┐│
│  │ Auth: key + key-md5     │ │              │  │ Auth: Bearer Token      ││
│  │ Header: timestamp       │ │              │  │ Rate Limit: 100 req/min ││
│  │ Response: JSON          │ │              │  │ Response: JSON          ││
│  └──────────────────────────┘ │              │  └─────────────────────────┘│
│  ┌──────────────────────────┐ │              │  ┌─────────────────────────┐│
│  │ Webhook Callbacks:       │ │              │  │ Webhook Callbacks:      ││
│  │ - Payment notification   │ │              │  │ - Payment completed     ││
│  │ - Refund notification    │ │              │  │ - Payment failed        ││
│  │ - POST to notifyUrl      │ │              │  │ - Payment cancelled     ││
│  │ - POST to notifyUrl      │ │              │  │ - POST to callbackUrl   ││
│  └──────────────────────────┘ │              │  └─────────────────────────┘│
└──────────────────────────────┘              └──────────────────────────────┘
          │                                                        │
          └────────────────┬─────────────────────────────────────┘
                           │
                    ┌──────▼──────┐
                    │  Database   │
                    │  (Local BD) │
                    │             │
                    │ - orders    │
                    │ - webhooks  │
                    │ - sync_logs │
                    └─────────────┘
```

---

## 🛡️ MATRIZ DE VALIDACIÓN

```
┌─────────────────────────┬──────────────────────┬─────────────────────────┐
│ VALIDACIÓN              │ CUÁNDO               │ QUÉ VALIDAR             │
├─────────────────────────┼──────────────────────┼─────────────────────────┤
│ Crear Orden             │ Antes de POST        │ - Amount > 0            │
│                         │                      │ - Reference único       │
│                         │                      │ - Currency válida       │
├─────────────────────────┼──────────────────────┼─────────────────────────┤
│ Webhook PVS             │ Al recibir           │ - Firma autenticidad    │
│                         │                      │ - Orden existe en BD    │
│                         │                      │ - Reference válido      │
│                         │                      │ - Timestamp reciente    │
├─────────────────────────┼──────────────────────┼─────────────────────────┤
│ Webhook GS              │ Al recibir           │ - Headers: key, md5     │
│                         │                      │ - Orden existe en BD    │
│                         │                      │ - OrderNo válido        │
│                         │                      │ - Status en rango 1-6   │
├─────────────────────────┼──────────────────────┼─────────────────────────┤
│ Refund                  │ Antes de procesar    │ - Orden pagada (status 2│
│                         │                      │ - Monto <= monto original
│                         │                      │ - Motivo no vacío       │
│                         │                      │ - Sin refund previo     │
├─────────────────────────┼──────────────────────┼─────────────────────────┤
│ Query Status            │ Periodicamente       │ - OrderNo existe        │
│                         │                      │ - Timestamp no muy viejo│
│                         │                      │ - BD vs API consistente │
└─────────────────────────┴──────────────────────┴─────────────────────────┘
```

---

## 📈 MONITOREO Y ALERTAS

```
MÉTRICA                    UMBRAL              ACCIÓN
─────────────────────────────────────────────────────────
Webhook latency (PVS)     > 5 segundos        🔴 Alertar
Webhook latency (GS)      > 10 segundos       🔴 Alertar
Failed orders             > 5 en 1 hora       🔴 Alertar
QR generation failure     > 2 consecutivos    🔴 Alertar
Missing webhook           > 30 segundos       ⚠️  Log
Order reconciliation fail > 1 por hora        ⚠️  Log
API response time         > 2 segundos        ℹ️  Monitor
Payment success rate      < 95%               🔴 Investigar
Refund processing time    > 24 horas          ⚠️  Follow up
```

---

## 🔐 CHECKLIST DE SEGURIDAD

```
ANTES DE PRODUCCIÓN:

□ ✅ Validar TODAS las firmas de webhook
□ ✅ Usar HTTPS en todos los endpoints
□ ✅ Encriptar credenciales de API
□ ✅ No loguear datos sensibles (card numbers, tokens)
□ ✅ Implementar rate limiting
□ ✅ Usar database transactions para operaciones críticas
□ ✅ Auditoría: loguear TODOS los webhooks
□ ✅ Implementar CORS correctamente
□ ✅ Usar idempotency keys
□ ✅ Implementar timeouts en llamadas a API
□ ✅ Testing: 100+ casos de prueba
□ ✅ Backup: plan de recuperación de datos
□ ✅ Monitoreo: alertas configuradas
□ ✅ Documentación: runbook para troubleshooting
□ ✅ Capacitación: equipo conoce flujos
```

---

## 📞 DIAGRAMA DE CONTACTO Y ESCALACIÓN

```
PROBLEMA                    PASO 1                 PASO 2                 PASO 3
──────────────────────────────────────────────────────────────────────────────
Usuario reporta            → Verificar BD         → Check PVS logs       → PVS Support
pago no confirmado            y estado de pago       (webhook received?)

Webhook nunca              → Revisar logs         → Check firewall       → System Admin
llegó                         de backend              y connectivity

QR no se genera            → Validar credenciales → Check PVS rate limit → PVS Support
                              PVS                    (100 req/min)

Inconsistencia             → Ejecutar              → Reconciliar BD       → Ambos supports
PVS vs GS                     reconciliación       → Actualizar manualmente

Reembolso                  → Verificar order      → Check GS refund      → GS Support
no procesado                  status                 status

Error 401/403              → Rotar credenciales   → Check timestamps     → API providers
authentication                                      (reloj sincronizado)
```

---

**Versión**: 1.0  
**Última actualización**: Diciembre 2024  
**Status**: Referencia de diseño
