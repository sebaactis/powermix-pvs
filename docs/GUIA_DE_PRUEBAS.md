# 🧪 Guía de Pruebas Locales (API + PVS Mock)

Esta guía describe cómo probar todo el ciclo de vida del servicio `vps-powermix` de forma local, simulando tanto las interacciones de la máquina expendedora GSWYIT (GS) como el comportamiento del proveedor PVS mediante un mock server dedicado.

---

## 🚀 Camino Rápido (Quick Setup)

1. **Configuración Inicial:**
   Copia el archivo `.env.example` a `.env` y configura tu string de conexión a PostgreSQL en `DATABASE_URL`.
   ```bash
   cp .env.example .env
   ```

2. **Levantar base de datos y correr migraciones:**
   Asegúrate de tener la DB Postgres creada y ejecuta las migraciones (`migrations/`).

3. **Iniciar el servidor Mock de PVS:**
   ```bash
   go run cmd/mockpvs/main.go
   ```
   *Escuchará en `http://localhost:8081`.*

4. **Iniciar el servidor del Bridge (vps-powermix):**
   *(Asegúrate de exportar las variables de entorno de tu `.env`)*
   ```bash
   # En Linux/macOS
   export $(cat .env | xargs) && go run cmd/server/main.go
   
   # En Windows (PowerShell)
   Get-Content .env | Foreach-Object {
       if ($_ -match "([^=]+)=(.*)") {
           [System.Environment]::SetEnvironmentVariable($Matches[1], $Matches[2])
       }
   }
   go run cmd/server/main.go
   ```
   *Escuchará en `http://localhost:8080`.*

---

## 🔄 Secuencias de Pruebas (Flujos de Negocio)

Actuaremos en rol de la máquina expendedora GS usando comandos `curl`.

### 1. Flujo Feliz (Compra y Entrega)

El cliente selecciona una bebida, paga el código QR generado, y la máquina entrega el producto.

#### Paso A: Crear la orden (GS -> Nosotros)
```bash
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -d '{"objectId":"bebida-001","totalAmount":"150.00","deviceId":"maquina-local-1"}'
```
* **Respuesta Esperada (200 OK):**
  ```json
  {"qrUrl":"iVBORw0KGgoAAAANS...","orderStatus":1,"thirdOrderNo":"ORD-<UUID>"}
  ```
  *(Copia el `thirdOrderNo` y el id de QR que figura en la consola del mock de PVS, por ejemplo `qr-ORD-<UUID>`)*

#### Paso B: Simular Pago Aprobado (PVS Webhook -> Nosotros)
PVS notifica que el pago del QR fue aprobado (`stateId: 5`).
```bash
curl -X POST http://localhost:8080/webhook/pvs \
  -H "Content-Type: application/json" \
  -d '{"qrId":"qr-ORD-<UUID>","stateId":5}'
```
* **Respuesta Esperada (200 OK):**
  ```json
  {"status":"ok"}
  ```

#### Paso C: La máquina consulta el estado (GS -> Nosotros, Polling)
GS realiza polling periódicamente para confirmar si puede dispensar.
```bash
curl -X GET http://localhost:8080/api/v1/orders/ORD-<UUID>
```
* **Respuesta Esperada (200 OK):**
  * `orderStatus: 2` (Significa `PAYMENT_CONFIRMED/APPROVED` para GS).

#### Paso D: Confirmar Entrega del Producto (GS -> Nosotros)
Una vez entregado el producto, la máquina cierra el ciclo.
```bash
curl -X POST http://localhost:8080/api/v1/orders/ORD-<UUID>/complete
```
* **Respuesta Esperada (200 OK):**
  ```json
  {"status":"ok"}
  ```
  *(El estado interno de la orden en DB pasa a `DONE`).*

---

### 2. Flujo de Cancelación (Expiración o Cancelación por el cliente)

El cliente desiste de la compra antes de pagar y presiona "Cancelar" en la máquina.

#### Paso A: Crear la orden
```bash
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -d '{"objectId":"bebida-002","totalAmount":"200.00","deviceId":"maquina-local-1"}'
```

#### Paso B: Cancelar la orden (GS -> Nosotros)
```bash
curl -X POST http://localhost:8080/api/v1/orders/ORD-<UUID>/cancel
```
* **Respuesta Esperada (200 OK):**
  ```json
  {"status":"ok"}
  ```
  *(El estado en DB pasa a `CANCELLED`).*

---

### 3. Flujo de Reembolso (Falla de entrega/Out of stock)

El cliente paga, pero la máquina expendedora no logra entregar la bebida y solicita la devolución del dinero.

#### Paso A: Crear la orden y simular pago aprobado
1. Crea la orden (`POST /api/v1/orders`).
2. Aprueba el pago en el webhook (`POST /webhook/pvs` con `stateId: 5`).

#### Paso B: Solicitar Reembolso (GS -> Nosotros)
La máquina informa que la entrega falló y nos pide hacer el refund.
```bash
curl -X POST http://localhost:8080/api/v1/orders/ORD-<UUID>/refund \
  -H "Content-Type: application/json" \
  -d '{"refundNo":"REF-1001","refundAmount":"150.00","refundReason":"Falla en espiral dispensador"}'
```
* **Respuesta Esperada (200 OK):**
  ```json
  {"refundNo":"REF-1001","thirdOrderNo":"ORD-<UUID>","refundStatus":"success"}
  ```
  *(Nuestro servicio llama en background a `PVS.Reverse` en nuestro Mock Server. La orden pasa a `REFUND_PENDING`).*

#### Paso C: Confirmación de Reverso (PVS Webhook -> Nosotros)
El reverso se procesa y PVS nos avisa por webhook (`stateId: 4`).
```bash
curl -X POST http://localhost:8080/webhook/pvs \
  -H "Content-Type: application/json" \
  -d '{"qrId":"qr-ORD-<UUID>","stateId":4}'
```
* **Respuesta Esperada (200 OK):**
  ```json
  {"status":"ok"}
  ```
  *(La orden en DB pasa al estado final `REFUNDED`).*

---

## 🕰️ Reconciliación en Background (Reconciler Loop)

El reconciliador del bridge se encarga de resolver órdenes estancadas sin intervención manual.

### Caso de Prueba: Pago confirmado pero Webhook de PVS perdido
1. Crea una orden (`POST /api/v1/orders`).
2. **Forzar estado en el Mock de PVS** simulando que el cliente pagó pero PVS nunca nos envió el webhook:
   ```bash
   curl -X POST http://localhost:8081/admin/transactions/qr-ORD-<UUID>/status \
     -H "Content-Type: application/json" \
     -d '{"stateId":5}'
   ```
3. Espera a que la orden expire en la base de datos (según `QR_EXPIRY_SEC` de tu configuración).
4. El reconciliador escaneará la orden, le consultará el estado real al Mock de PVS, detectará que está aprobada (`stateId: 5`) y actualizará automáticamente la base de datos del bridge a `PAYMENT_CONFIRMED` de manera automática. Puedes ver los logs del bridge para verificar esta acción.

---

## 📊 Métricas y Salud

* **Healthcheck:** `GET http://localhost:8080/healthz` (Chequea conexión viva a la base de datos).
* **Prometheus Metrics:** `GET http://localhost:8080/metrics` (Muestra contadores de requests, latencias y ejecuciones del reconciliador).
