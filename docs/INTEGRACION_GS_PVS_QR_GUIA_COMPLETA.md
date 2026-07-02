# 🌉 GUÍA DE INTEGRACIÓN: SISTEMA GS + PVS QR
## Creando el Puente Entre Pagos y Generación de QR

---

## 📊 ANÁLISIS COMPARATIVO DE ARQUITECTURA

### **SISTEMA 1: GS (Payment System)**
- **Rol**: Procesador de pagos principal
- **Función**: Gestiona transacciones, ordenes y notificaciones
- **Métodos Soportados**: Card Payment, QR Payment
- **Ubicación Lógica**: Backend centralizado

### **SISTEMA 2: PVS QR (QR Generation System)**
- **Rol**: Generador de códigos QR
- **Función**: Crea QR dinámicos para pagos
- **Métodos Soportados**: Pagos por QR, account management
- **Ubicación Lógica**: Gateway de pagos QR

### **Relación Entre Sistemas**
```
Tu Aplicación
     ↓
  ┌─────────────────────────────────────┐
  │   PVS QR (Generador de Códigos)      │
  │   - Crea QR dinámicos                 │
  │   - Administra transacciones QR       │
  └─────────────────────────────────────┘
     ↓↑ (Sincronización de datos)
  ┌─────────────────────────────────────┐
  │   GS (Procesador de Pagos)           │
  │   - Procesa pagos                     │
  │   - Maneja reembolsos                 │
  │   - Notificaciones de estado          │
  └─────────────────────────────────────┘
```

---

## 🔄 FLUJO DE INTEGRACIÓN PROPUESTO

### **ESCENARIO 1: Generación de Código QR**

```
1. Tu App solicita crear pago
   │
   └─→ PVS QR: Generar QR
       │
       └─→ Retorna: URL del QR + ID de transacción
           │
           └─→ (Opcional) Registrar en GS para tracking
               │
               └─→ Almacenar relación:
                   - QR ID (PVS)
                   - Order ID (GS)
                   - Timestamp
```

### **ESCENARIO 2: Confirmación de Pago**

```
1. Usuario escanea QR y paga
   │
   ├─→ PVS QR: Detecta pago
   │   │
   │   └─→ Notifica a callback de PVS
   │
   └─→ GS: Registra transacción
       │
       └─→ Notifica a callback de GS
           │
           └─→ Tu app recibe confirmación
               │
               └─→ Actualizar estado de orden
```

---

## 🔐 MAPEO DE CAMPOS Y AUTENTICACIÓN

### **Headers de Autenticación Requeridos**

#### Para GS:
```json
{
  "key": "Tu clave API (generada en backend GS)",
  "key-md5": "MD5(key + secret + timestamp)",
  "timestamp": "Timestamp actual en milisegundos",
  "Content-Type": "application/json"
}
```

#### Para PVS QR:
```json
{
  "Authorization": "Bearer {access_token}",
  "Content-Type": "application/json"
}
```
*Nota: Verificar con PVS el método de autenticación exacto*

---

## 📝 MAPEO DE ENDPOINTS

### **1. CREAR ORDEN Y GENERAR QR**

#### Opción A: PVS Primero → Luego GS
```
POST /pvs/qr/generate
├─ Input:
│  {
│    "amount": 100.50,
│    "currency": "ARS",
│    "reference": "ORDER-2024-001",
│    "description": "Bebida Café",
│    "callbackUrl": "https://tuapp.com/webhook/pvs"
│  }
├─ Output:
│  {
│    "qrId": "qr_123456",
│    "qrUrl": "https://qr.pvs.com.ar/qr_123456",
│    "expiresAt": "2024-12-06T14:30:00Z",
│    "status": "pending"
│  }
└─ Luego: Registrar en GS
   POST /gs/payments/order
   {
     "orderNo": "ORDER-2024-001",
     "subject": "Bebida Café",
     "totalAmount": "100.50",
     "payMethod": "qr_payment",
     "wayCode": "qr",
     "notifyUrl": "https://tuapp.com/webhook/gs",
     "externalQrId": "qr_123456"  // Vincular con PVS
   }
```

#### Opción B: GS Primero → Luego PVS
```
POST /gs/payments/order
├─ Input:
│  {
│    "orderNo": "ORDER-2024-001",
│    "subject": "Bebida Café",
│    "totalAmount": "100.50",
│    "payMethod": "qr_payment",
│    "wayCode": "qr",
│    "notifyUrl": "https://tuapp.com/webhook/gs"
│  }
├─ Output:
│  {
│    "code": 200,
│    "data": {
│      "id": "1864912228214394882",
│      "orderId": "1864912228214394883",
│      "orderNo": "ORDER-2024-001",
│      "status": 1  // Pendiente
│    }
│  }
└─ Luego: Generar QR en PVS
   POST /pvs/qr/generate
   {
     "amount": 100.50,
     "reference": "1864912228214394882",  // ID de GS
     "callbackUrl": "https://tuapp.com/webhook/pvs"
   }
```

---

## 🔔 WEBHOOKS Y NOTIFICACIONES

### **Webhook de PVS (Confirmación QR)**

Tu app debe recibir en: `https://tuapp.com/webhook/pvs`

```json
{
  "event": "payment.completed",
  "qrId": "qr_123456",
  "reference": "ORDER-2024-001",
  "amount": 100.50,
  "paidAt": "2024-12-06T14:01:15Z",
  "payerInfo": {
    "method": "qr_scan",
    "paymentMethod": "debit_card",
    "last4": "1234"
  },
  "status": "success"
}
```

**Tu App Debe:**
1. Validar la firma del webhook
2. Actualizar estado en base de datos
3. Notificar a GS (si no ya ocurrió)
4. Retornar `{"status": "ok"}`

### **Webhook de GS (Confirmación General)**

Tu app debe recibir en: `https://tuapp.com/webhook/gs`

```json
{
  "orderNo": "ORDER-2024-001",
  "orderStatus": 2,  // Pago exitoso
  "payTime": "2024-12-06T14:01:15",
  "payAmt": "100.50",
  "payMethod": "wxpay",
  "wayCode": "qr",
  "tradeNo": "4200002450202412063058717692",
  "thirdOrderNo": "qr_123456"  // Puede vincular con PVS
}
```

---

## 🛠️ ESTRUCTURA DE BASE DE DATOS RECOMENDADA

```sql
-- Tabla de Órdenes Integradas
CREATE TABLE orders (
  id BIGINT PRIMARY KEY,
  order_no VARCHAR(255) UNIQUE,           -- ID único de orden
  
  -- Información de PVS
  pvs_qr_id VARCHAR(255),                 -- ID de PVS
  pvs_qr_url VARCHAR(500),                -- URL del QR
  
  -- Información de GS  
  gs_order_id BIGINT,                     -- ID de GS
  gs_trade_no VARCHAR(255),               -- Trade number de GS
  
  -- Datos de Transacción
  amount DECIMAL(10,2),
  currency VARCHAR(3),
  payment_method VARCHAR(50),             -- qr_payment, card, etc.
  
  -- Estados
  order_status INT,                       -- 1-6 (según GS)
  pvs_status VARCHAR(50),                 -- pending, completed, failed
  
  -- Timestamps
  created_at TIMESTAMP,
  qr_generated_at TIMESTAMP,
  payment_completed_at TIMESTAMP,
  
  -- Auditoría
  webhook_gs_received BOOLEAN,
  webhook_pvs_received BOOLEAN,
  webhook_gs_data JSON,
  webhook_pvs_data JSON
);

-- Tabla de Sincronización
CREATE TABLE api_sync_log (
  id INT PRIMARY KEY AUTO_INCREMENT,
  order_id BIGINT,
  api_name VARCHAR(20),                   -- 'GS' o 'PVS'
  endpoint VARCHAR(255),
  request_data JSON,
  response_data JSON,
  http_status INT,
  created_at TIMESTAMP,
  FOREIGN KEY (order_id) REFERENCES orders(id)
);
```

---

## ⚙️ IMPLEMENTACIÓN: FLUJO COMPLETO EN PSEUDOCÓDIGO

### **Backend - Crear Orden con QR**

```javascript
async function crearOrdenConQR(datosOrden) {
  try {
    // 1. Generar QR en PVS
    console.log('📱 Generando QR en PVS...');
    const pvsResponse = await fetch('https://api.pvssa.com.ar/qr/generate', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${PVS_TOKEN}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        amount: datosOrden.amount,
        currency: 'ARS',
        reference: datosOrden.reference,
        description: datosOrden.description,
        callbackUrl: 'https://tuapp.com/webhook/pvs'
      })
    });
    
    const pvsData = await pvsResponse.json();
    
    if (!pvsResponse.ok) {
      throw new Error(`PVS Error: ${pvsData.message}`);
    }
    
    console.log(`✅ QR generado: ${pvsData.qrUrl}`);
    
    // 2. Registrar orden en GS
    console.log('💳 Registrando orden en GS...');
    const gsResponse = await fetch('https://api.gs.com/payments/order', {
      method: 'POST',
      headers: {
        'key': GS_KEY,
        'key-md5': md5(GS_KEY + GS_SECRET + Date.now()),
        'timestamp': Date.now().toString(),
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        orderNo: datosOrden.reference,
        subject: datosOrden.description,
        totalAmount: datosOrden.amount.toString(),
        payMethod: 'qr_payment',
        wayCode: 'qr',
        notifyUrl: 'https://tuapp.com/webhook/gs'
      })
    });
    
    const gsData = await gsResponse.json();
    
    if (gsData.code !== 200) {
      throw new Error(`GS Error: ${gsData.msg}`);
    }
    
    console.log(`✅ Orden registrada en GS: ${gsData.data.orderNo}`);
    
    // 3. Guardar en base de datos
    await guardarOrden({
      order_no: datosOrden.reference,
      pvs_qr_id: pvsData.qrId,
      pvs_qr_url: pvsData.qrUrl,
      gs_order_id: gsData.data.orderId,
      amount: datosOrden.amount,
      order_status: 1,
      pvs_status: 'pending'
    });
    
    // 4. Retornar QR al frontend
    return {
      success: true,
      qrUrl: pvsData.qrUrl,
      orderNo: datosOrden.reference,
      expiresAt: pvsData.expiresAt
    };
    
  } catch (error) {
    console.error('❌ Error creando orden:', error);
    throw error;
  }
}
```

### **Backend - Webhook de PVS**

```javascript
app.post('/webhook/pvs', async (req, res) => {
  try {
    const payload = req.body;
    
    console.log('🔔 Webhook PVS recibido:', payload.reference);
    
    // 1. Validar firma (si es necesario)
    // const isValid = validateSignature(payload);
    // if (!isValid) return res.status(401).json({error: 'Invalid signature'});
    
    // 2. Buscar orden
    const orden = await Orden.findOne({ 
      order_no: payload.reference 
    });
    
    if (!orden) {
      return res.status(404).json({ error: 'Orden no encontrada' });
    }
    
    // 3. Actualizar estado en base de datos
    orden.pvs_status = payload.status;
    orden.webhook_pvs_received = true;
    orden.webhook_pvs_data = payload;
    
    if (payload.status === 'success') {
      orden.payment_completed_at = new Date();
    }
    
    await orden.save();
    
    // 4. Si GS aún no ha notificado, actualizar manualmente
    if (!orden.webhook_gs_received) {
      console.log('⏳ Esperando confirmación de GS...');
      // Opcional: Consultar estado en GS
      // const gsStatus = await consultarEstadoGS(orden.gs_order_id);
    }
    
    // 5. Notificar a tu aplicación frontend
    io.to(`order-${orden.order_no}`).emit('payment_status', {
      orderNo: orden.order_no,
      status: 'completed',
      amount: orden.amount
    });
    
    // 6. Responder a PVS
    res.json({ status: 'ok', received: true });
    
  } catch (error) {
    console.error('❌ Error procesando webhook PVS:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});
```

### **Backend - Webhook de GS**

```javascript
app.post('/webhook/gs', async (req, res) => {
  try {
    const payload = req.body;
    
    console.log('🔔 Webhook GS recibido:', payload.orderNo);
    
    // 1. Buscar orden por orderNo de GS
    const orden = await Orden.findOne({ 
      order_no: payload.orderNo 
    });
    
    if (!orden) {
      return res.status(404).json({ error: 'Orden no encontrada' });
    }
    
    // 2. Mapear estado de GS
    const estadoMap = {
      1: 'pending',
      2: 'completed',
      3: 'failed',
      4: 'pending_refund',
      5: 'refunded',
      6: 'timeout'
    };
    
    orden.order_status = payload.orderStatus;
    orden.gs_trade_no = payload.tradeNo;
    orden.webhook_gs_received = true;
    orden.webhook_gs_data = payload;
    
    if (payload.orderStatus === 2) {
      orden.payment_completed_at = new Date();
    }
    
    await orden.save();
    
    // 3. Sincronizar con PVS si es necesario
    if (payload.orderStatus === 2 && !orden.webhook_pvs_received) {
      console.log('🔄 Consultando estado en PVS...');
      const pvsStatus = await consultarEstadoPVS(orden.pvs_qr_id);
      orden.pvs_status = pvsStatus.status;
      await orden.save();
    }
    
    // 4. Registrar en log de sincronización
    await guardarSyncLog({
      order_id: orden.id,
      api_name: 'GS',
      response_status: payload.orderStatus
    });
    
    // 5. Notificar al frontend
    io.to(`order-${orden.order_no}`).emit('payment_status', {
      orderNo: orden.order_no,
      status: estadoMap[payload.orderStatus],
      amount: orden.amount,
      timestamp: new Date()
    });
    
    res.json({ status: 'ok', received: true });
    
  } catch (error) {
    console.error('❌ Error procesando webhook GS:', error);
    res.status(500).json({ error: 'Internal server error' });
  }
});
```

---

## 🔍 CONSULTAS DE ESTADO (Fallback)

Si los webhooks fallan, implementar consultas periódicas:

```javascript
// Consultar estado en PVS
async function consultarEstadoPVS(qrId) {
  const response = await fetch(`https://api.pvssa.com.ar/qr/${qrId}/status`, {
    method: 'GET',
    headers: {
      'Authorization': `Bearer ${PVS_TOKEN}`
    }
  });
  
  return await response.json();
}

// Consultar estado en GS
async function consultarEstadoGS(orderNo) {
  const response = await fetch('https://api.gs.com/payments/query', {
    method: 'POST',
    headers: {
      'key': GS_KEY,
      'key-md5': md5(GS_KEY + GS_SECRET + Date.now()),
      'timestamp': Date.now().toString()
    },
    body: JSON.stringify({
      orderNo: orderNo
    })
  });
  
  const data = await response.json();
  return data.data;
}

// Ejecutar reconciliación cada minuto
setInterval(async () => {
  const ordenesPendientes = await Orden.find({ 
    order_status: 1,
    created_at: { $gt: new Date(Date.now() - 30*60*1000) } // Últimas 30 min
  });
  
  for (const orden of ordenesPendientes) {
    try {
      const gsStatus = await consultarEstadoGS(orden.order_no);
      if (gsStatus.orderStatus !== 1) {
        // Actualizar si hay cambio
        orden.order_status = gsStatus.orderStatus;
        await orden.save();
      }
    } catch (error) {
      console.error(`Error consultando estado de ${orden.order_no}:`, error);
    }
  }
}, 60000);
```

---

## 📋 CHECKLIST DE IMPLEMENTACIÓN

- [ ] Obtener credenciales de API de ambos sistemas
- [ ] Configurar CORS en ambas APIs
- [ ] Implementar tabla de orders con campos de sincronización
- [ ] Crear endpoints de webhook en tu backend
- [ ] Implementar autenticación para GS
- [ ] Implementar autenticación para PVS
- [ ] Crear servicio de generación de QR
- [ ] Crear servicio de consulta de estado
- [ ] Implementar logging de sincronización
- [ ] Crear endpoint de reconciliación
- [ ] Hacer pruebas con transacciones de prueba
- [ ] Configurar alertas para fallos de sincronización
- [ ] Documentar mappeo de estados
- [ ] Capacitar al equipo en troubleshooting
- [ ] Crear runbook para escalación

---

## 🚨 MANEJO DE ERRORES COMUNES

| Escenario | Causa Probable | Solución |
|-----------|---|---|
| QR generado pero pago no aparece en GS | Demora en sincronización | Implementar polling cada 5-10 seg |
| Webhook no recibido | Timeout de red | Agregar reintentos con backoff exponencial |
| Órdenes duplicadas | Race condition | Usar idempotency key en requests |
| Estado inconsistente | Fallo parcial | Implementar reconciliación periódica |
| Reembolso rechazado | Campo obligatorio faltante | Validar todos los campos requeridos |

---

## 📞 CONTACTOS Y RECURSOS

**Sistema GS:**
- Documentación: Archivo DOCX proporcionado
- Support: [Verificar con equipo GS]

**Sistema PVS QR:**
- Documentación: https://qr-integration.pvssa.com.ar/
- Support: [Verificar con equipo PVS]

---

## 📌 NOTAS IMPORTANTES

1. **Testing**: Usar siempre ambientes de prueba primero
2. **Seguridad**: Nunca expongas las claves API en el frontend
3. **Logging**: Registra TODOS los webhooks para auditoría
4. **Monitoreo**: Configura alertas para fallos de sincronización
5. **Backup**: Implementa rollback en caso de inconsistencias

---

**Versión**: 1.0  
**Última actualización**: Diciembre 2024  
**Estado**: Listo para implementación
