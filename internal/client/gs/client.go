// Package gs implementa ports.GSClient para comunicarse con la
// maquina expendedora GSWYIT. Usa autenticacion key-md5 HMAC.
package gs

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/seba/vps-powermix/internal/ports"
)

// Cliente es la implementacion concreta de ports.GSClient.
type Cliente struct {
	httpClient *http.Client
	baseURL    string
	key        string
	secret     string
}

// New crea un Cliente GS listo para usar.
func New(baseURL, key, secret string, opts ...Opcion) *Cliente {
	c := &Cliente{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    baseURL,
		key:        key,
		secret:     secret,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Opcion permite configurar el cliente (functional options).
type Opcion func(*Cliente)

// ConTimeoutHTTP cambia el timeout del http.Client.
func ConTimeoutHTTP(d time.Duration) Opcion {
	return func(c *Cliente) { c.httpClient.Timeout = d }
}

// SignRequest firma un request saliente con los headers key, key-md5 y timestamp.
// Algoritmo: md5(key + secret + timestamp), donde timestamp es epoch millis.
// Los headers se setean en el request modificandolo in-place.
func SignRequest(req *http.Request, key, secret string) {
	ts := time.Now().UnixMilli()
	tsStr := strconv.FormatInt(ts, 10)

	input := key + secret + tsStr
	hash := md5.Sum([]byte(input))
	keyMD5 := hex.EncodeToString(hash[:])

	req.Header.Set("key", key)
	req.Header.Set("key-md5", keyMD5)
	req.Header.Set("timestamp", tsStr)
	req.Header.Set("Content-Type", "application/json")
}

// -- GSClient implementation --

// QueryStatus consulta el estado de una orden en GS.
// En Path A (polling) no se usa, pero se implementa por si el
// reconciler necesita consultar o si el patron cambia en el futuro.
func (c *Cliente) QueryStatus(ctx context.Context, req *ports.GSQueryRequest) (*ports.GSQueryResponse, error) {
	cuerpo, _ := json.Marshal(map[string]string{
		"orderNo":      req.OrderNo,
		"thirdOrderNo": req.ThirdOrderNo,
	})

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/payments/query", bytes.NewReader(cuerpo))
	if err != nil {
		return nil, fmt.Errorf("creando request QueryStatus: %w", err)
	}

	SignRequest(httpReq, c.key, c.secret)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("QueryStatus fallo: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GS respondio %d: %s", resp.StatusCode, string(respBytes))
	}

	var gsResp struct {
		Code int `json:"code"`
		Data struct {
			OrderNo      string `json:"orderNo"`
			OrderStatus  int    `json:"orderStatus"`
			ThirdOrderNo string `json:"thirdOrderNo"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &gsResp); err != nil {
		return nil, fmt.Errorf("parseando respuesta GS: %w", err)
	}

	if gsResp.Code != 200 {
		return nil, fmt.Errorf("GS error %d", gsResp.Code)
	}

	return &ports.GSQueryResponse{
		OrderNo:      gsResp.Data.OrderNo,
		ThirdOrderNo: gsResp.Data.ThirdOrderNo,
		OrderStatus:  gsResp.Data.OrderStatus,
	}, nil
}

// Refund solicita un reembolso a GS.
func (c *Cliente) Refund(ctx context.Context, req *ports.GSRefundRequest) (*ports.GSRefundResponse, error) {
	cuerpo, _ := json.Marshal(map[string]string{
		"refundNo":        req.RefundNo,
		"orderNo":         req.OrderNo,
		"thirdOrderNo":    req.ThirdOrderNo,
		"refundAmount":    req.RefundAmount,
		"refundReason":    req.RefundReason,
		"refundNotifyUrl": req.RefundNotifyURL,
	})

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/payments/refund", bytes.NewReader(cuerpo))
	if err != nil {
		return nil, fmt.Errorf("creando request Refund: %w", err)
	}

	SignRequest(httpReq, c.key, c.secret)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Refund fallo: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GS respondio %d: %s", resp.StatusCode, string(respBytes))
	}

	var gsResp struct {
		Code int `json:"code"`
		Data struct {
			RefundNo     string `json:"refundNo"`
			OrderNo      string `json:"orderNo"`
			ThirdOrderNo string `json:"thirdOrderNo"`
			RefundStatus string `json:"refundStatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &gsResp); err != nil {
		return nil, fmt.Errorf("parseando respuesta GS Refund: %w", err)
	}

	if gsResp.Code != 200 {
		return nil, fmt.Errorf("GS Refund error %d", gsResp.Code)
	}

	return &ports.GSRefundResponse{
		RefundNo:     gsResp.Data.RefundNo,
		OrderNo:      gsResp.Data.OrderNo,
		ThirdOrderNo: gsResp.Data.ThirdOrderNo,
		RefundStatus: gsResp.Data.RefundStatus,
	}, nil
}

// Garantia de compilacion: Cliente implementa ports.GSClient
var _ ports.GSClient = (*Cliente)(nil)
