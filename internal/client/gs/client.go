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

	"github.com/seba/vps-powermix/internal/logging"
	"github.com/seba/vps-powermix/internal/ports"
)

// Cliente es la implementacion concreta de ports.GSClient.
type Cliente struct {
	httpClient *http.Client
	baseURL    string // reservado; NotifyPayment usa URL absoluta de notifyUrl
	key        string
	secret     string
}

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

type Opcion func(*Cliente)

func ConTimeoutHTTP(d time.Duration) Opcion {
	return func(c *Cliente) { c.httpClient.Timeout = d }
}

// SignRequest firma un request saliente con los headers key, key-md5 y timestamp.
// Algoritmo: md5(key + secret + timestamp), donde timestamp es epoch millis.
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

// NotifyPayment avisa a GS el resultado de pago en la notifyUrl de la orden.
// POST a URL absoluta (no se usa baseURL).
func (c *Cliente) NotifyPayment(ctx context.Context, req *ports.GSNotifyPaymentRequest) (*ports.GSNotifyPaymentResponse, error) {
	if req == nil || req.NotifyURL == "" {
		return nil, fmt.Errorf("notifyUrl es obligatorio")
	}

	cuerpo, err := json.Marshal(map[string]string{
		"orderNo":       req.OrderNo,
		"thirdOrderNo":  req.ThirdOrderNo,
		"orderStatus":   req.OrderStatus,
		"orderTime":     req.OrderTime,
		"payTime":       req.PayTime,
		"totalAmount":   req.TotalAmount,
		"channelUserId": req.ChannelUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("serializando NotifyPayment: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", req.NotifyURL, bytes.NewReader(cuerpo))
	if err != nil {
		return nil, fmt.Errorf("creando request NotifyPayment: %w", err)
	}
	SignRequest(httpReq, c.key, c.secret)

	logGSRequest(ctx, "POST", req.NotifyURL, cuerpo)

	start := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		logGSResponse(ctx, "POST", req.NotifyURL, 0, durationMs, nil)
		return nil, fmt.Errorf("NotifyPayment fallo: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	logGSResponse(ctx, "POST", req.NotifyURL, resp.StatusCode, durationMs, respBytes)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GS respondio %d: %s", resp.StatusCode, string(respBytes))
	}

	var gsResp struct {
		Code int `json:"code"`
		Data struct {
			ReturnCode string `json:"returnCode"`
			ReturnMsg  string `json:"returnMsg"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(respBytes, &gsResp); err != nil {
		// Algunos ejemplos de la doc devuelven data vacia; si HTTP 2xx, OK.
		return &ports.GSNotifyPaymentResponse{ReturnCode: "success"}, nil
	}
	if gsResp.Code != 0 && gsResp.Code != 200 {
		return nil, fmt.Errorf("GS NotifyPayment error code %d: %s", gsResp.Code, gsResp.Msg)
	}

	returnCode := gsResp.Data.ReturnCode
	if returnCode == "" {
		returnCode = "success"
	}
	return &ports.GSNotifyPaymentResponse{
		ReturnCode: returnCode,
		ReturnMsg:  gsResp.Data.ReturnMsg,
	}, nil
}

// logGSRequest / logGSResponse: bodies sanitizados.
// No loguean headers de firma (key, key-md5, secret).
// Body solo si LOG_HTTP_BODIES esta ON (logging.ConfigureHTTPBodyLogging).
func logGSRequest(ctx context.Context, method, endpoint string, reqBody []byte) {
	attrs := []any{"method", method, "endpoint", endpoint}
	if body, ok := logging.FormatBodyForLog(reqBody); ok {
		attrs = append(attrs, "body", body)
	}
	logging.From(ctx).Info("gs.http.request", attrs...)
}

func logGSResponse(ctx context.Context, method, endpoint string, status int, durationMs int64, respBody []byte) {
	attrs := []any{
		"method", method,
		"endpoint", endpoint,
		"status_code", status,
		"duration_ms", durationMs,
	}
	if body, ok := logging.FormatBodyForLog(respBody); ok {
		attrs = append(attrs, "body", body)
	}
	logging.From(ctx).Info("gs.http.response", attrs...)
}

// Garantia de compilacion: Cliente implementa ports.GSClient
var _ ports.GSClient = (*Cliente)(nil)
