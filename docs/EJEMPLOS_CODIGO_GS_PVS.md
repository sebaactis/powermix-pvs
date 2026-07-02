# 💻 EJEMPLOS DE IMPLEMENTACIÓN - GS + PVS QR

## Ejemplos en múltiples lenguajes para integración rápida

---

## 🟦 TypeScript / Node.js

### Configuración Inicial

```typescript
// config.ts
export const GS_CONFIG = {
  baseUrl: 'https://api.gs.com',
  key: process.env.GS_KEY || '',
  secret: process.env.GS_SECRET || ''
};

export const PVS_CONFIG = {
  baseUrl: 'https://api.pvssa.com.ar',
  token: process.env.PVS_TOKEN || ''
};

export const WEBHOOK_CONFIG = {
  gsCallbackUrl: 'https://tuapp.com/webhook/gs',
  pvsCallbackUrl: 'https://tuapp.com/webhook/pvs'
};
```

### Cliente de GS

```typescript
// services/gsPaymentService.ts
import crypto from 'crypto';
import axios from 'axios';

class GSPaymentService {
  private key: string;
  private secret: string;
  private baseUrl: string;

  constructor(key: string, secret: string, baseUrl: string) {
    this.key = key;
    this.secret = secret;
    this.baseUrl = baseUrl;
  }

  private generateHeaders() {
    const timestamp = Date.now().toString();
    const md5Hash = crypto
      .createHash('md5')
      .update(this.key + this.secret + timestamp)
      .digest('hex');

    return {
      'key': this.key,
      'key-md5': md5Hash,
      'timestamp': timestamp,
      'Content-Type': 'application/json'
    };
  }

  async createOrder(orderData: {
    orderNo: string;
    subject: string;
    totalAmount: string;
    payMethod: string;
    wayCode: string;
    notifyUrl: string;
    externalQrId?: string;
  }) {
    try {
      const response = await axios.post(
        `${this.baseUrl}/payments/order`,
        orderData,
        { headers: this.generateHeaders() }
      );

      return response.data;
    } catch (error) {
      console.error('❌ Error creando orden en GS:', error);
      throw error;
    }
  }

  async queryPaymentStatus(orderNo: string) {
    try {
      const response = await axios.post(
        `${this.baseUrl}/payments/query`,
        { orderNo },
        { headers: this.generateHeaders() }
      );

      return response.data;
    } catch (error) {
      console.error('❌ Error consultando estado en GS:', error);
      throw error;
    }
  }

  async refundPayment(refundData: {
    refundNo: string;
    orderNo: string;
    refundAmount: string;
    refundReason: string;
    refundNotifyUrl: string;
  }) {
    try {
      const response = await axios.post(
        `${this.baseUrl}/payments/refund`,
        refundData,
        { headers: this.generateHeaders() }
      );

      return response.data;
    } catch (error) {
      console.error('❌ Error procesando reembolso en GS:', error);
      throw error;
    }
  }
}

export default GSPaymentService;
```

### Cliente de PVS

```typescript
// services/pvsQrService.ts
import axios from 'axios';

class PVSQrService {
  private token: string;
  private baseUrl: string;

  constructor(token: string, baseUrl: string) {
    this.token = token;
    this.baseUrl = baseUrl;
  }

  private getHeaders() {
    return {
      'Authorization': `Bearer ${this.token}`,
      'Content-Type': 'application/json'
    };
  }

  async generateQr(qrData: {
    amount: number;
    currency: string;
    reference: string;
    description: string;
    callbackUrl: string;
  }) {
    try {
      const response = await axios.post(
        `${this.baseUrl}/qr/generate`,
        qrData,
        { headers: this.getHeaders() }
      );

      return response.data;
    } catch (error) {
      console.error('❌ Error generando QR en PVS:', error);
      throw error;
    }
  }

  async getQrStatus(qrId: string) {
    try {
      const response = await axios.get(
        `${this.baseUrl}/qr/${qrId}/status`,
        { headers: this.getHeaders() }
      );

      return response.data;
    } catch (error) {
      console.error('❌ Error consultando estado QR en PVS:', error);
      throw error;
    }
  }

  async cancelQr(qrId: string) {
    try {
      const response = await axios.post(
        `${this.baseUrl}/qr/${qrId}/cancel`,
        {},
        { headers: this.getHeaders() }
      );

      return response.data;
    } catch (error) {
      console.error('❌ Error cancelando QR en PVS:', error);
      throw error;
    }
  }
}

export default PVSQrService;
```

### Controlador Principal

```typescript
// controllers/paymentController.ts
import { Request, Response } from 'express';
import GSPaymentService from '../services/gsPaymentService';
import PVSQrService from '../services/pvsQrService';
import Order from '../models/Order';
import { GS_CONFIG, PVS_CONFIG, WEBHOOK_CONFIG } from '../config';

class PaymentController {
  private gsService: GSPaymentService;
  private pvsService: PVSQrService;

  constructor() {
    this.gsService = new GSPaymentService(
      GS_CONFIG.key,
      GS_CONFIG.secret,
      GS_CONFIG.baseUrl
    );
    this.pvsService = new PVSQrService(
      PVS_CONFIG.token,
      PVS_CONFIG.baseUrl
    );
  }

  // 1. Crear orden con QR
  async createOrderWithQR(req: Request, res: Response) {
    try {
      const { amount, description, reference } = req.body;

      console.log(`📱 Iniciando proceso de creación de orden ${reference}...`);

      // Paso 1: Generar QR en PVS
      console.log('🔄 Paso 1: Generando QR en PVS...');
      const pvsResult = await this.pvsService.generateQr({
        amount,
        currency: 'ARS',
        reference,
        description,
        callbackUrl: WEBHOOK_CONFIG.pvsCallbackUrl
      });

      if (!pvsResult.success) {
        throw new Error(`PVS Error: ${pvsResult.message}`);
      }

      console.log(`✅ QR generado: ${pvsResult.data.qrUrl}`);

      // Paso 2: Registrar orden en GS
      console.log('🔄 Paso 2: Registrando orden en GS...');
      const gsResult = await this.gsService.createOrder({
        orderNo: reference,
        subject: description,
        totalAmount: amount.toString(),
        payMethod: 'qr_payment',
        wayCode: 'qr',
        notifyUrl: WEBHOOK_CONFIG.gsCallbackUrl,
        externalQrId: pvsResult.data.qrId
      });

      if (gsResult.code !== 200) {
        throw new Error(`GS Error: ${gsResult.msg}`);
      }

      console.log(`✅ Orden registrada en GS: ${gsResult.data.orderNo}`);

      // Paso 3: Guardar en base de datos
      const order = new Order({
        order_no: reference,
        pvs_qr_id: pvsResult.data.qrId,
        pvs_qr_url: pvsResult.data.qrUrl,
        gs_order_id: gsResult.data.orderId,
        amount,
        currency: 'ARS',
        order_status: 1,
        pvs_status: 'pending'
      });

      await order.save();

      // Respuesta exitosa
      res.json({
        success: true,
        orderNo: reference,
        qrUrl: pvsResult.data.qrUrl,
        expiresAt: pvsResult.data.expiresAt,
        message: 'Orden creada exitosamente'
      });

    } catch (error) {
      console.error('❌ Error:', error);
      res.status(500).json({
        success: false,
        error: error instanceof Error ? error.message : 'Error desconocido'
      });
    }
  }

  // 2. Webhook de PVS
  async handlePVSWebhook(req: Request, res: Response) {
    try {
      const payload = req.body;
      console.log(`🔔 Webhook PVS recibido para: ${payload.reference}`);

      // Buscar orden
      const order = await Order.findOne({ order_no: payload.reference });
      if (!order) {
        return res.status(404).json({ error: 'Orden no encontrada' });
      }

      // Actualizar estado
      order.pvs_status = payload.status;
      order.webhook_pvs_received = true;
      order.webhook_pvs_data = payload;

      if (payload.status === 'success') {
        order.payment_completed_at = new Date();
      }

      await order.save();

      console.log(`✅ Orden ${payload.reference} actualizada desde PVS`);

      // Notificar a clientes conectados (WebSocket)
      // io.to(`order-${order.order_no}`).emit('payment_update', {
      //   orderNo: order.order_no,
      //   pvs_status: order.pvs_status
      // });

      res.json({ status: 'ok', received: true });

    } catch (error) {
      console.error('❌ Error procesando webhook PVS:', error);
      res.status(500).json({ error: 'Internal server error' });
    }
  }

  // 3. Webhook de GS
  async handleGSWebhook(req: Request, res: Response) {
    try {
      const payload = req.body;
      console.log(`🔔 Webhook GS recibido para: ${payload.orderNo}`);

      // Buscar orden
      const order = await Order.findOne({ order_no: payload.orderNo });
      if (!order) {
        return res.status(404).json({ error: 'Orden no encontrada' });
      }

      // Mapear estado
      const statusMap: { [key: number]: string } = {
        1: 'pending',
        2: 'completed',
        3: 'failed',
        4: 'pending_refund',
        5: 'refunded',
        6: 'timeout'
      };

      order.order_status = payload.orderStatus;
      order.gs_trade_no = payload.tradeNo;
      order.webhook_gs_received = true;
      order.webhook_gs_data = payload;

      if (payload.orderStatus === 2) {
        order.payment_completed_at = new Date();
      }

      await order.save();

      console.log(`✅ Orden ${payload.orderNo} actualizada desde GS`);

      res.json({ status: 'ok', received: true });

    } catch (error) {
      console.error('❌ Error procesando webhook GS:', error);
      res.status(500).json({ error: 'Internal server error' });
    }
  }

  // 4. Consultar estado
  async getOrderStatus(req: Request, res: Response) {
    try {
      const { orderNo } = req.params;

      const order = await Order.findOne({ order_no: orderNo });
      if (!order) {
        return res.status(404).json({ error: 'Orden no encontrada' });
      }

      // Consultar en GS para verificar estado actual
      const gsStatus = await this.gsService.queryPaymentStatus(orderNo);

      // Actualizar si hay cambios
      if (gsStatus.data.orderStatus !== order.order_status) {
        order.order_status = gsStatus.data.orderStatus;
        await order.save();
      }

      res.json({
        orderNo: order.order_no,
        amount: order.amount,
        order_status: order.order_status,
        pvs_status: order.pvs_status,
        payment_completed_at: order.payment_completed_at,
        gs_data: gsStatus.data
      });

    } catch (error) {
      console.error('❌ Error consultando estado:', error);
      res.status(500).json({ error: 'Error consultando estado' });
    }
  }

  // 5. Procesar reembolso
  async refundOrder(req: Request, res: Response) {
    try {
      const { orderNo, refundAmount, refundReason } = req.body;

      const order = await Order.findOne({ order_no: orderNo });
      if (!order) {
        return res.status(404).json({ error: 'Orden no encontrada' });
      }

      const refundNo = `REFUND-${Date.now()}`;

      const gsRefund = await this.gsService.refundPayment({
        refundNo,
        orderNo,
        refundAmount: refundAmount.toString(),
        refundReason,
        refundNotifyUrl: WEBHOOK_CONFIG.gsCallbackUrl
      });

      if (gsRefund.code !== 200) {
        throw new Error(`GS Refund Error: ${gsRefund.msg}`);
      }

      order.order_status = 4; // Pendiente de reembolso
      await order.save();

      res.json({
        success: true,
        refundNo,
        message: 'Reembolso procesado exitosamente'
      });

    } catch (error) {
      console.error('❌ Error procesando reembolso:', error);
      res.status(500).json({ 
        error: error instanceof Error ? error.message : 'Error procesando reembolso' 
      });
    }
  }
}

export default new PaymentController();
```

### Setup de Express

```typescript
// app.ts
import express from 'express';
import paymentController from './controllers/paymentController';

const app = express();

// Middleware
app.use(express.json());

// Routes
app.post('/api/payments/create-order-with-qr', 
  paymentController.createOrderWithQR.bind(paymentController)
);

app.post('/webhook/pvs', 
  paymentController.handlePVSWebhook.bind(paymentController)
);

app.post('/webhook/gs', 
  paymentController.handleGSWebhook.bind(paymentController)
);

app.get('/api/payments/:orderNo/status', 
  paymentController.getOrderStatus.bind(paymentController)
);

app.post('/api/payments/:orderNo/refund', 
  paymentController.refundOrder.bind(paymentController)
);

app.listen(3000, () => {
  console.log('🚀 Server running on port 3000');
});

export default app;
```

---

## 🐍 Python / Flask

```python
# payment_service.py
import hashlib
import json
import requests
from datetime import datetime
from typing import Dict, Any
from flask import Flask, request, jsonify

class GSPaymentClient:
    def __init__(self, key: str, secret: str, base_url: str):
        self.key = key
        self.secret = secret
        self.base_url = base_url
    
    def _generate_headers(self) -> Dict[str, str]:
        timestamp = str(int(datetime.now().timestamp() * 1000))
        md5_hash = hashlib.md5(
            f"{self.key}{self.secret}{timestamp}".encode()
        ).hexdigest()
        
        return {
            'key': self.key,
            'key-md5': md5_hash,
            'timestamp': timestamp,
            'Content-Type': 'application/json'
        }
    
    def create_order(self, order_data: Dict[str, Any]) -> Dict:
        """Crear orden en GS"""
        try:
            response = requests.post(
                f"{self.base_url}/payments/order",
                json=order_data,
                headers=self._generate_headers(),
                timeout=10
            )
            return response.json()
        except Exception as e:
            print(f"❌ Error creando orden en GS: {e}")
            raise
    
    def query_payment_status(self, order_no: str) -> Dict:
        """Consultar estado de pago"""
        try:
            response = requests.post(
                f"{self.base_url}/payments/query",
                json={'orderNo': order_no},
                headers=self._generate_headers(),
                timeout=10
            )
            return response.json()
        except Exception as e:
            print(f"❌ Error consultando estado en GS: {e}")
            raise
    
    def refund_payment(self, refund_data: Dict[str, Any]) -> Dict:
        """Procesar reembolso"""
        try:
            response = requests.post(
                f"{self.base_url}/payments/refund",
                json=refund_data,
                headers=self._generate_headers(),
                timeout=10
            )
            return response.json()
        except Exception as e:
            print(f"❌ Error reembolsando en GS: {e}")
            raise


class PVSQrClient:
    def __init__(self, token: str, base_url: str):
        self.token = token
        self.base_url = base_url
    
    def _get_headers(self) -> Dict[str, str]:
        return {
            'Authorization': f'Bearer {self.token}',
            'Content-Type': 'application/json'
        }
    
    def generate_qr(self, qr_data: Dict[str, Any]) -> Dict:
        """Generar código QR"""
        try:
            response = requests.post(
                f"{self.base_url}/qr/generate",
                json=qr_data,
                headers=self._get_headers(),
                timeout=10
            )
            return response.json()
        except Exception as e:
            print(f"❌ Error generando QR en PVS: {e}")
            raise
    
    def get_qr_status(self, qr_id: str) -> Dict:
        """Obtener estado de QR"""
        try:
            response = requests.get(
                f"{self.base_url}/qr/{qr_id}/status",
                headers=self._get_headers(),
                timeout=10
            )
            return response.json()
        except Exception as e:
            print(f"❌ Error consultando QR en PVS: {e}")
            raise


# Flask App
app = Flask(__name__)

# Instanciar clientes
gs_client = GSPaymentClient(
    key='TU_GS_KEY',
    secret='TU_GS_SECRET',
    base_url='https://api.gs.com'
)

pvs_client = PVSQrClient(
    token='TU_PVS_TOKEN',
    base_url='https://api.pvssa.com.ar'
)

# Variables globales (usar BD en producción)
orders_db = {}

@app.route('/api/payments/create-order-with-qr', methods=['POST'])
def create_order_with_qr():
    """Crear orden con QR"""
    try:
        data = request.json
        amount = data.get('amount')
        description = data.get('description')
        reference = data.get('reference')
        
        print(f"📱 Iniciando proceso de creación de orden {reference}...")
        
        # Paso 1: Generar QR en PVS
        print("🔄 Paso 1: Generando QR en PVS...")
        pvs_result = pvs_client.generate_qr({
            'amount': amount,
            'currency': 'ARS',
            'reference': reference,
            'description': description,
            'callbackUrl': 'https://tuapp.com/webhook/pvs'
        })
        
        print(f"✅ QR generado: {pvs_result['data']['qrUrl']}")
        
        # Paso 2: Registrar orden en GS
        print("🔄 Paso 2: Registrando orden en GS...")
        gs_result = gs_client.create_order({
            'orderNo': reference,
            'subject': description,
            'totalAmount': str(amount),
            'payMethod': 'qr_payment',
            'wayCode': 'qr',
            'notifyUrl': 'https://tuapp.com/webhook/gs'
        })
        
        print(f"✅ Orden registrada en GS: {gs_result['data']['orderNo']}")
        
        # Paso 3: Guardar en BD (simulada)
        order_key = reference
        orders_db[order_key] = {
            'order_no': reference,
            'pvs_qr_id': pvs_result['data']['qrId'],
            'pvs_qr_url': pvs_result['data']['qrUrl'],
            'gs_order_id': gs_result['data']['orderId'],
            'amount': amount,
            'currency': 'ARS',
            'order_status': 1,
            'pvs_status': 'pending',
            'created_at': datetime.now().isoformat()
        }
        
        return jsonify({
            'success': True,
            'orderNo': reference,
            'qrUrl': pvs_result['data']['qrUrl'],
            'expiresAt': pvs_result['data'].get('expiresAt'),
            'message': 'Orden creada exitosamente'
        }), 200
        
    except Exception as error:
        print(f"❌ Error: {error}")
        return jsonify({
            'success': False,
            'error': str(error)
        }), 500

@app.route('/webhook/pvs', methods=['POST'])
def handle_pvs_webhook():
    """Webhook de PVS"""
    try:
        payload = request.json
        print(f"🔔 Webhook PVS recibido para: {payload['reference']}")
        
        # Actualizar orden
        if payload['reference'] in orders_db:
            order = orders_db[payload['reference']]
            order['pvs_status'] = payload['status']
            order['webhook_pvs_received'] = True
            order['webhook_pvs_data'] = payload
            
            if payload['status'] == 'success':
                order['payment_completed_at'] = datetime.now().isoformat()
            
            print(f"✅ Orden {payload['reference']} actualizada desde PVS")
        
        return jsonify({'status': 'ok', 'received': True}), 200
        
    except Exception as error:
        print(f"❌ Error procesando webhook PVS: {error}")
        return jsonify({'error': 'Internal server error'}), 500

@app.route('/webhook/gs', methods=['POST'])
def handle_gs_webhook():
    """Webhook de GS"""
    try:
        payload = request.body.get_json()
        print(f"🔔 Webhook GS recibido para: {payload['orderNo']}")
        
        # Actualizar orden
        if payload['orderNo'] in orders_db:
            order = orders_db[payload['orderNo']]
            order['order_status'] = payload['orderStatus']
            order['gs_trade_no'] = payload['tradeNo']
            order['webhook_gs_received'] = True
            order['webhook_gs_data'] = payload
            
            if payload['orderStatus'] == 2:
                order['payment_completed_at'] = datetime.now().isoformat()
            
            print(f"✅ Orden {payload['orderNo']} actualizada desde GS")
        
        return jsonify({'status': 'ok', 'received': True}), 200
        
    except Exception as error:
        print(f"❌ Error procesando webhook GS: {error}")
        return jsonify({'error': 'Internal server error'}), 500

@app.route('/api/payments/<order_no>/status', methods=['GET'])
def get_order_status(order_no):
    """Consultar estado de orden"""
    try:
        if order_no not in orders_db:
            return jsonify({'error': 'Orden no encontrada'}), 404
        
        order = orders_db[order_no]
        
        # Consultar en GS
        gs_status = gs_client.query_payment_status(order_no)
        
        # Actualizar si cambió
        if gs_status['data']['orderStatus'] != order['order_status']:
            order['order_status'] = gs_status['data']['orderStatus']
        
        return jsonify({
            'orderNo': order['order_no'],
            'amount': order['amount'],
            'order_status': order['order_status'],
            'pvs_status': order['pvs_status'],
            'payment_completed_at': order.get('payment_completed_at'),
            'gs_data': gs_status['data']
        }), 200
        
    except Exception as error:
        print(f"❌ Error: {error}")
        return jsonify({'error': str(error)}), 500

@app.route('/api/payments/<order_no>/refund', methods=['POST'])
def refund_order(order_no):
    """Procesar reembolso"""
    try:
        if order_no not in orders_db:
            return jsonify({'error': 'Orden no encontrada'}), 404
        
        data = request.json
        refund_amount = data.get('refundAmount')
        refund_reason = data.get('refundReason')
        
        refund_no = f"REFUND-{int(datetime.now().timestamp()*1000)}"
        
        gs_refund = gs_client.refund_payment({
            'refundNo': refund_no,
            'orderNo': order_no,
            'refundAmount': str(refund_amount),
            'refundReason': refund_reason,
            'refundNotifyUrl': 'https://tuapp.com/webhook/gs'
        })
        
        if gs_refund['code'] != 200:
            raise Exception(f"GS Refund Error: {gs_refund['msg']}")
        
        orders_db[order_no]['order_status'] = 4  # Pendiente de reembolso
        
        return jsonify({
            'success': True,
            'refundNo': refund_no,
            'message': 'Reembolso procesado exitosamente'
        }), 200
        
    except Exception as error:
        print(f"❌ Error: {error}")
        return jsonify({'error': str(error)}), 500

if __name__ == '__main__':
    app.run(debug=True, port=5000)
```

---

## 🔴 Java / Spring Boot

```java
// GsPaymentClient.java
package com.example.payment.client;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpEntity;
import org.springframework.http.HttpHeaders;
import org.springframework.http.ResponseEntity;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestTemplate;
import org.apache.commons.codec.digest.DigestUtils;
import com.fasterxml.jackson.databind.JsonNode;

import java.util.HashMap;
import java.util.Map;

@Component
public class GsPaymentClient {
    
    @Value("${gs.api.key}")
    private String key;
    
    @Value("${gs.api.secret}")
    private String secret;
    
    @Value("${gs.api.baseUrl}")
    private String baseUrl;
    
    private final RestTemplate restTemplate;
    
    public GsPaymentClient(RestTemplate restTemplate) {
        this.restTemplate = restTemplate;
    }
    
    private HttpHeaders generateHeaders() {
        long timestamp = System.currentTimeMillis();
        String md5Input = key + secret + timestamp;
        String md5Hash = DigestUtils.md5Hex(md5Input);
        
        HttpHeaders headers = new HttpHeaders();
        headers.set("key", key);
        headers.set("key-md5", md5Hash);
        headers.set("timestamp", String.valueOf(timestamp));
        headers.set("Content-Type", "application/json");
        
        return headers;
    }
    
    public ResponseEntity<JsonNode> createOrder(Map<String, String> orderData) {
        HttpEntity<Map<String, String>> entity = 
            new HttpEntity<>(orderData, generateHeaders());
        
        return restTemplate.postForEntity(
            baseUrl + "/payments/order",
            entity,
            JsonNode.class
        );
    }
    
    public ResponseEntity<JsonNode> queryPaymentStatus(String orderNo) {
        Map<String, String> body = new HashMap<>();
        body.put("orderNo", orderNo);
        
        HttpEntity<Map<String, String>> entity = 
            new HttpEntity<>(body, generateHeaders());
        
        return restTemplate.postForEntity(
            baseUrl + "/payments/query",
            entity,
            JsonNode.class
        );
    }
    
    public ResponseEntity<JsonNode> refundPayment(Map<String, String> refundData) {
        HttpEntity<Map<String, String>> entity = 
            new HttpEntity<>(refundData, generateHeaders());
        
        return restTemplate.postForEntity(
            baseUrl + "/payments/refund",
            entity,
            JsonNode.class
        );
    }
}
```

---

## 🔗 cURL (Ejemplos Rápidos)

### Crear Orden en GS

```bash
curl --location --request POST 'https://api.gs.com/payments/order' \
  --header 'key: YOUR_GS_KEY' \
  --header 'key-md5: YOUR_MD5_HASH' \
  --header 'timestamp: 1733446562634' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "orderNo": "ORDER-2024-001",
    "subject": "Bebida Café",
    "totalAmount": "100.50",
    "payMethod": "qr_payment",
    "wayCode": "qr",
    "notifyUrl": "https://tuapp.com/webhook/gs"
  }'
```

### Generar QR en PVS

```bash
curl --location --request POST 'https://api.pvssa.com.ar/qr/generate' \
  --header 'Authorization: Bearer YOUR_PVS_TOKEN' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "amount": 100.50,
    "currency": "ARS",
    "reference": "ORDER-2024-001",
    "description": "Bebida Café",
    "callbackUrl": "https://tuapp.com/webhook/pvs"
  }'
```

### Consultar Estado en GS

```bash
curl --location --request POST 'https://api.gs.com/payments/query' \
  --header 'key: YOUR_GS_KEY' \
  --header 'key-md5: YOUR_MD5_HASH' \
  --header 'timestamp: 1733446562634' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "orderNo": "ORDER-2024-001"
  }'
```

---

## 📱 Frontend / React

```jsx
// paymentService.js
const API_BASE_URL = 'http://localhost:3000/api';

export const paymentService = {
  async createOrderWithQr(orderData) {
    const response = await fetch(
      `${API_BASE_URL}/payments/create-order-with-qr`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(orderData)
      }
    );
    return response.json();
  },

  async getOrderStatus(orderNo) {
    const response = await fetch(
      `${API_BASE_URL}/payments/${orderNo}/status`
    );
    return response.json();
  },

  async refundOrder(orderNo, refundData) {
    const response = await fetch(
      `${API_BASE_URL}/payments/${orderNo}/refund`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(refundData)
      }
    );
    return response.json();
  }
};
```

```jsx
// PaymentComponent.jsx
import React, { useState } from 'react';
import QRCode from 'qrcode.react';
import { paymentService } from './paymentService';

export default function PaymentComponent() {
  const [orderNo, setOrderNo] = useState('');
  const [qrUrl, setQrUrl] = useState('');
  const [status, setStatus] = useState('pending');
  const [loading, setLoading] = useState(false);

  const handleCreateOrder = async () => {
    setLoading(true);
    try {
      const reference = `ORDER-${Date.now()}`;
      const result = await paymentService.createOrderWithQr({
        amount: 100.50,
        description: 'Bebida Café',
        reference
      });

      if (result.success) {
        setOrderNo(reference);
        setQrUrl(result.qrUrl);
        setStatus('pending');
        
        // Polling para estado
        const pollInterval = setInterval(async () => {
          const statusResult = await paymentService.getOrderStatus(reference);
          setStatus(statusResult.order_status);
          
          if (statusResult.order_status === 2) {
            clearInterval(pollInterval);
          }
        }, 2000);
      }
    } catch (error) {
      console.error('Error:', error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="payment-container">
      <h1>Pagar con QR</h1>
      
      {!qrUrl ? (
        <button onClick={handleCreateOrder} disabled={loading}>
          {loading ? 'Generando QR...' : 'Crear Orden'}
        </button>
      ) : (
        <div>
          <h2>Orden: {orderNo}</h2>
          <img src={qrUrl} alt="QR Code" />
          <p>Estado: {status === 2 ? '✅ Pagado' : '⏳ Pendiente'}</p>
        </div>
      )}
    </div>
  );
}
```

---

**Nota**: Reemplaza valores de `YOUR_GS_KEY`, `YOUR_PVS_TOKEN`, etc. con tus credenciales reales.
