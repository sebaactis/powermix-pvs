# ⚡ RESUMEN EJECUTIVO - Integración GS + PVS QR

**Para**: Tu equipo técnico  
**Objetivo**: Conectar sistema de pagos GS con generador de QR de PVS  
**Timeline**: 1-2 semanas de implementación  
**Complejidad**: Media  

---

## 🎯 VISIÓN DE 30 SEGUNDOS

Tu aplicación necesita procesar pagos. Usarás **PVS para generar códigos QR** y **GS para procesar las transacciones**. Ambos sistemas se comunicarán a través de webhooks.

**Flujo básico:**
```
1. Usuario pide pagar → Tu app solicita QR a PVS
2. PVS genera QR → Tu app registra orden en GS  
3. Usuario escanea y paga → Ambos sistemas notifican al mismo tiempo
4. Tu app recibe webhooks → Actualiza estado de orden
5. Usuario ve confirmación ✅
```

---

## 📋 LO QUE NECESITAS

### Credenciales
```
✅ GS:
   - API Key
   - API Secret
   - Base URL: https://api.gs.com

✅ PVS:
   - Bearer Token
   - Base URL: https://api.pvssa.com.ar
```

### Configuraciones
```
✅ Webhook URLs (comunica a ambos):
   - https://tuapp.com/webhook/gs
   - https://tuapp.com/webhook/pvs

✅ Servidor backend (Node/Python/Java)
✅ Base de datos con 3 tablas:
   - orders (órdenes)
   - webhooks_log (para debugging)
   - sync_history (para reconciliación)
```

---

## 🚀 IMPLEMENTACIÓN EN 5 PASOS

### PASO 1: Crear tabla de órdenes
```sql
CREATE TABLE orders (
  id BIGINT PRIMARY KEY,
  order_no VARCHAR(255) UNIQUE,
  pvs_qr_id VARCHAR(255),
  pvs_qr_url VARCHAR(500),
  gs_order_id BIGINT,
  gs_trade_no VARCHAR(255),
  amount DECIMAL(10,2),
  order_status INT,  -- 1-6
  pvs_status VARCHAR(50),
  webhook_gs_received BOOLEAN,
  webhook_pvs_received BOOLEAN,
  created_at TIMESTAMP
);
```

### PASO 2: Crear endpoint para solicitar QR
```
POST /api/payments/create-order-with-qr
Input: {amount, description, reference}
Output: {qrUrl, orderNo, expiresAt}

Internamente:
1. Call PVS → /qr/generate
2. Call GS → /payments/order
3. Save to BD
4. Return QR
```

### PASO 3: Recibir webhooks de PVS
```
POST /webhook/pvs
Input: {event, qrId, reference, status, paidAt}

Internamente:
1. Find order by reference
2. Update: pvs_status = status
3. If status == "success": payment_completed_at = now
4. Return {status: "ok"}
```

### PASO 4: Recibir webhooks de GS
```
POST /webhook/gs
Input: {orderNo, orderStatus, payTime, tradeNo}

Internamente:
1. Find order by orderNo
2. Update: order_status = orderStatus
3. Update: gs_trade_no = tradeNo
4. Notificar frontend (WebSocket/Polling)
5. Return {status: "ok"}
```

### PASO 5: Agregar endpoint de consulta (fallback)
```
GET /api/payments/{orderNo}/status
Output: {status, amount, gs_data, pvs_data}

Internamente:
1. Query BD
2. Verify en GS si estado = 1 (pending)
3. Return consolidated state
```

---

## 🔑 PUNTOS CLAVE

### Autenticación

**Para GS:**
```
Header: key = YOUR_GS_KEY
Header: key-md5 = MD5(key + secret + timestamp)
Header: timestamp = Current timestamp in milliseconds
```

**Para PVS:**
```
Header: Authorization = Bearer YOUR_PVS_TOKEN
```

### Estados de Orden (GS)

| Estado | Significado | Acción |
|--------|-------------|--------|
| 1 | Pendiente | Esperar pago |
| 2 | Pagado ✅ | Confirmar orden |
| 3 | Falló ❌ | Permitir reintentar |
| 4 | Reembolso pendiente | Procesar refund |
| 5 | Reembolsado | Finalizar |
| 6 | Timeout | Cancelar |

### Estados de QR (PVS)

| Estado | Significado |
|--------|-------------|
| pending | Esperando pago |
| success | Pagado ✅ |
| failed | Error ❌ |
| cancelled | Usuario canceló |

---

## 🔄 FLUJO DETALLADO (Backend)

### 1️⃣ Crear Orden (Node.js pseudocódigo)

```javascript
async function crearOrden(amount, description, reference) {
  // 1. Generar QR en PVS
  const pvsResponse = await fetch('https://api.pvssa.com.ar/qr/generate', {
    method: 'POST',
    headers: {'Authorization': `Bearer ${PVS_TOKEN}`},
    body: JSON.stringify({amount, reference, description})
  });
  
  const pvsData = await pvsResponse.json();
  if (!pvsResponse.ok) throw new Error(pvsData.message);
  
  // 2. Registrar en GS
  const gsHeaders = {
    'key': GS_KEY,
    'key-md5': md5(GS_KEY + GS_SECRET + Date.now()),
    'timestamp': Date.now().toString()
  };
  
  const gsResponse = await fetch('https://api.gs.com/payments/order', {
    method: 'POST',
    headers: gsHeaders,
    body: JSON.stringify({
      orderNo: reference,
      subject: description,
      totalAmount: amount.toString(),
      payMethod: 'qr_payment',
      wayCode: 'qr',
      notifyUrl: 'https://tuapp.com/webhook/gs'
    })
  });
  
  const gsData = await gsResponse.json();
  if (gsData.code !== 200) throw new Error(gsData.msg);
  
  // 3. Guardar en BD
  await db.orders.insert({
    order_no: reference,
    pvs_qr_id: pvsData.data.qrId,
    pvs_qr_url: pvsData.data.qrUrl,
    gs_order_id: gsData.data.orderId,
    amount: amount,
    order_status: 1,
    pvs_status: 'pending'
  });
  
  return {qrUrl: pvsData.data.qrUrl, orderNo: reference};
}
```

### 2️⃣ Webhook de PVS

```javascript
app.post('/webhook/pvs', async (req, res) => {
  const payload = req.body;
  const order = await db.orders.findOne({order_no: payload.reference});
  
  order.pvs_status = payload.status;
  order.webhook_pvs_received = true;
  if (payload.status === 'success') order.payment_completed_at = new Date();
  await order.save();
  
  // Notificar frontend
  io.emit('order_update', {orderNo: payload.reference, status: 'updated'});
  
  res.json({status: 'ok'});
});
```

### 3️⃣ Webhook de GS

```javascript
app.post('/webhook/gs', async (req, res) => {
  const payload = req.body;
  const order = await db.orders.findOne({order_no: payload.orderNo});
  
  order.order_status = payload.orderStatus;
  order.gs_trade_no = payload.tradeNo;
  order.webhook_gs_received = true;
  if (payload.orderStatus === 2) order.payment_completed_at = new Date();
  await order.save();
  
  // Notificar frontend
  io.emit('order_update', {orderNo: payload.orderNo, status: 'updated'});
  
  res.json({status: 'ok'});
});
```

---

## 🛠️ CHECKLIST RÁPIDO

### Pre-implementación
- [ ] Obtener credenciales de ambos sistemas
- [ ] Crear BD con tablas necesarias
- [ ] Configurar HTTPS en tu servidor
- [ ] Comunicar webhook URLs a PVS y GS

### Desarrollo
- [ ] Implementar cliente de GS (MD5 headers)
- [ ] Implementar cliente de PVS (Bearer token)
- [ ] Crear endpoint POST /create-order-with-qr
- [ ] Crear webhook handlers (/webhook/pvs y /webhook/gs)
- [ ] Crear endpoint GET /{orderNo}/status

### Testing
- [ ] Test con credenciales sandbox
- [ ] Simular pagos exitosos
- [ ] Simular pagos fallidos
- [ ] Simular timeout de webhook
- [ ] Test reconciliación manual
- [ ] Load testing (múltiples órdenes simultáneas)

### Producción
- [ ] Usar credenciales de producción
- [ ] Configurar logs y monitoreo
- [ ] Configurar alertas para fallos
- [ ] Implementar retry logic
- [ ] Documentar runbook de troubleshooting
- [ ] Capacitar al equipo

---

## ⚠️ ERRORES COMUNES A EVITAR

| Error | Causa | Solución |
|-------|-------|----------|
| "Invalid MD5" en GS | Timestamp desincronizado | Sincronizar reloj del servidor |
| Webhook no recibido | Firewall bloqueando | Whitelist IPs de PVS/GS |
| Órdenes duplicadas | Race condition | Usar idempotency keys |
| Estado inconsistente | Un webhook llegó tarde | Implementar polling de fallback |
| Timeout en PVS | Rate limit | Agregar retry con backoff |

---

## 📞 TROUBLESHOOTING RÁPIDO

### "Error 400 Bad Syntax en GS"
→ Validar que `totalAmount` sea string, no número

### "Error 401 Not Authorized"
→ Verificar que MD5 sea correcto: `md5(key+secret+timestamp)`

### "QR generado pero pago no aparece"
→ Normal, puede demorar 5-10 segundos

### "Webhook de PVS llegó pero no el de GS"
→ Implementar polling: cada 5 seg consultar estado en GS por 1 minuto

### "Pago aparece en GS pero no en PVS"
→ Usar estado de GS como source of truth, sincronizar hacia PVS

---

## 📊 RENDIMIENTO ESPERADO

```
Generar QR:          < 2 segundos
Registrar en GS:     < 3 segundos
Webhook latency:     < 5 segundos
Total end-to-end:    < 15 segundos
```

---

## 🎓 RECURSOS

1. **Documentación GS**: `/mnt/user-data/outputs/INTEGRACION_GS_PVS_QR_GUIA_COMPLETA.md`
2. **Ejemplos de código**: `/mnt/user-data/outputs/EJEMPLOS_CODIGO_GS_PVS.md`
3. **Diagramas y flujos**: `/mnt/user-data/outputs/DIAGRAMAS_MATRIZ_DECISION_GS_PVS.md`
4. **Documentación PVS**: https://qr-integration.pvssa.com.ar/

---

## 💡 TIPS FINALES

1. **Logging**: Loguea TODOS los webhooks para debugging
2. **Idempotencia**: Diseña endpoints para manejar llamadas duplicadas
3. **Monitoreo**: Configura alertas para fallso de webhook
4. **Database**: Usa transacciones para mantener consistencia
5. **Testing**: 10% de órdenes deben ser de test antes de producción

---

**¿Listo para empezar?**

1. Obtén las credenciales
2. Lee la guía completa (INTEGRACION_GS_PVS_QR_GUIA_COMPLETA.md)
3. Revisa ejemplos de código (EJEMPLOS_CODIGO_GS_PVS.md)
4. Sigue el checklist
5. ¡Implementa! 🚀

---

**Soporte**: Si surgen dudas, revisa la sección de troubleshooting o contacta a ambos providers.

