// Package pvs implementa ports.PVSClient y ports.TokenCache para
// comunicarse con el proveedor de pagos QR PVS (Argentina).
package pvs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"

	"github.com/seba/vps-powermix/internal/logging"
	"github.com/seba/vps-powermix/internal/ports"
)

// Cliente es la implementacion concreta de ports.PVSClient.
// Usa un TokenCache interno para el OAuth2 y un rate limiter.
type Cliente struct {
	httpClient  *http.Client
	baseURL     string
	tokenCache  *TokenCache
	rateLimiter *rate.Limiter
}

// Opciones por defecto: timeout 10s, rate limit 50 req/s.
func New(baseURL string, clientID, clientSecret string, opts ...Opcion) *Cliente {
	c := &Cliente{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:     baseURL,
		tokenCache:  NewTokenCache(baseURL, clientID, clientSecret),
		rateLimiter: rate.NewLimiter(50, 50),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type Opcion func(*Cliente)

func ConHTTPClient(hc *http.Client) Opcion {
	return func(c *Cliente) { c.httpClient = hc }
}

func ConRateLimit(rps int, burst int) Opcion {
	return func(c *Cliente) {
		c.rateLimiter = rate.NewLimiter(rate.Limit(rps), burst)
	}
}

// GenerateQR implementa ports.PVSClient.
func (c *Cliente) GenerateQR(ctx context.Context, req *ports.PVSQRRequest) (*ports.PVSQRResponse, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	token, err := c.tokenCache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("obteniendo token PVS: %w", err)
	}

	// PVS espera el monto en decimal: "150.00"
	montoDecimal := fmt.Sprintf("%.2f", float64(req.Amount)/100)

	cuerpo := map[string]interface{}{
		"amount":     montoDecimal,
		"externalId": req.ExternalID,
		"reference":  req.Reference,
	}
	// Solo agregar callbackUrl si esta presente
	if req.CallbackURL != "" {
		cuerpo["callbackUrl"] = req.CallbackURL
	}

	bodyBytes, _ := json.Marshal(cuerpo)

	endpoint := c.baseURL + "/external/connect/api/v1/qr/pvs"
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		endpoint,
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creando request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	logPVSRequest(ctx, "POST", endpoint, bodyBytes)

	start := time.Now()
	resp, err := c.doConRetry(ctx, httpReq)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		logPVSResponse(ctx, "POST", endpoint, 0, durationMs, nil)
		return nil, &httpStatusError{err: err, codigoHTTP: 0}
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	logPVSResponse(ctx, "POST", endpoint, resp.StatusCode, durationMs, respBytes)

	if resp.StatusCode >= 400 {
		return nil, c.mapearError(resp.StatusCode, respBytes)
	}

	pvsResp, err := decodePVSData[struct {
		QrID    string `json:"qrId"`
		QrImage string `json:"qrImage"`
		Qr      string `json:"qr"` // ejemplo live a veces usa "qr" en vez de "qrImage"
	}](respBytes)
	if err != nil {
		return nil, fmt.Errorf("parseando respuesta PVS: %w", err)
	}

	img := pvsResp.QrImage
	if img == "" {
		img = pvsResp.Qr
	}
	if img == "" {
		return nil, fmt.Errorf("respuesta PVS sin imagen QR (qrImage/qr vacíos)")
	}

	return &ports.PVSQRResponse{
		QrID:    pvsResp.QrID,
		QrImage: img,
	}, nil
}

// QueryStatus implementa ports.PVSClient.
// Endpoint: GET /external/connect/api/v1/transactions/qrpvs/{qrId}
func (c *Cliente) QueryStatus(ctx context.Context, qrID string) (*ports.PVSQueryResponse, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	token, err := c.tokenCache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("obteniendo token PVS: %w", err)
	}

	url := fmt.Sprintf("%s/external/connect/api/v1/transactions/qrpvs/%s",
		c.baseURL, url.PathEscape(qrID))

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creando request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)

	logPVSRequest(ctx, "GET", url, nil)

	start := time.Now()
	resp, err := c.doConRetry(ctx, httpReq)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		logPVSResponse(ctx, "GET", url, 0, durationMs, nil)
		return nil, &httpStatusError{err: err, codigoHTTP: 0}
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	logPVSResponse(ctx, "GET", url, resp.StatusCode, durationMs, respBytes)

	if resp.StatusCode >= 400 {
		return nil, c.mapearError(resp.StatusCode, respBytes)
	}

	pvsResp, err := decodePVSData[struct {
		StateID int    `json:"stateId"`
		Status  string `json:"status,omitempty"`
	}](respBytes)
	if err != nil {
		return nil, fmt.Errorf("parseando respuesta PVS: %w", err)
	}

	return &ports.PVSQueryResponse{
		QrID:    qrID,
		StateID: pvsResp.StateID,
	}, nil
}

// Reverse implementa ports.PVSClient.
// Endpoint: POST /external/connect/api/v1/qr/pvs/reverse
func (c *Cliente) Reverse(ctx context.Context, qrID string) (*ports.PVSReverseResponse, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	token, err := c.tokenCache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("obteniendo token PVS: %w", err)
	}

	cuerpo := map[string]string{"qrId": qrID}
	bodyBytes, _ := json.Marshal(cuerpo)

	endpoint := c.baseURL + "/external/connect/api/v1/qr/pvs/reverse"
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		endpoint,
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creando request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	logPVSRequest(ctx, "POST", endpoint, bodyBytes)

	start := time.Now()
	resp, err := c.doConRetry(ctx, httpReq)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		logPVSResponse(ctx, "POST", endpoint, 0, durationMs, nil)
		return nil, &httpStatusError{err: err, codigoHTTP: 0}
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logPVSResponse(ctx, "POST", endpoint, resp.StatusCode, durationMs, nil)
		return nil, fmt.Errorf("leyendo respuesta reverse PVS: %w", err)
	}
	logPVSResponse(ctx, "POST", endpoint, resp.StatusCode, durationMs, respBytes)

	if resp.StatusCode >= 400 {
		return nil, c.mapearError(resp.StatusCode, respBytes)
	}

	// Doc PVS: reverse también viene en envelope { code, ok, data:{ txeId } }
	pvsResp, err := decodePVSData[struct {
		TxEID string `json:"txeId"`
	}](respBytes)
	if err != nil {
		return nil, fmt.Errorf("parseando respuesta reverse PVS: %w", err)
	}

	return &ports.PVSReverseResponse{
		Success: true,
		TxEID:   pvsResp.TxEID,
	}, nil
}

// doConRetry ejecuta el request. Si obtiene 401, invalida el cache
// de token, obtiene uno nuevo, y re-ejecuta exactamente una vez.
func (c *Cliente) doConRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request fallido: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		c.tokenCache.Invalidate(ctx)

		token, err := c.tokenCache.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("refrescando token tras 401: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)
		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("reintento tras 401 fallido: %w", err)
		}
	}

	return resp, nil
}

// mapearError convierte una respuesta HTTP de error en un error Go
// con el codigo HTTP preservado (via httpStatusError).
func (c *Cliente) mapearError(statusCode int, body []byte) error {
	msg := string(body)
	if msg == "" {
		msg = http.StatusText(statusCode)
	}
	return &httpStatusError{
		err:        fmt.Errorf("PVS respondio %d: %s", statusCode, msg),
		codigoHTTP: statusCode,
	}
}

// pvsEnvelope es la caja que PVS pone alrededor de toda respuesta OK.
type pvsEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data"`
}

// decodePVSData abre envelope y devuelve data tipada.
func decodePVSData[T any](body []byte) (T, error) {
	var zero T

	var env pvsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return zero, fmt.Errorf("parseando envelope PVS: %w", err)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return zero, fmt.Errorf("envelope PVS sin data (code=%s message=%s ok=%v)",
			env.Code, env.Message, env.OK)
	}

	var dest T
	if err := json.Unmarshal(env.Data, &dest); err != nil {
		return zero, fmt.Errorf("parseando data PVS: %w", err)
	}
	return dest, nil
}

// httpStatusError preserva el codigo HTTP original para que el handler
// pueda decidir si es 400 (bad request) o 500 (retry).
type httpStatusError struct {
	err        error
	codigoHTTP int
}

func (e *httpStatusError) Error() string   { return e.err.Error() }
func (e *httpStatusError) Unwrap() error   { return e.err }
func (e *httpStatusError) CodigoHTTP() int { return e.codigoHTTP }

// Garantia de compilacion: Cliente implementa ports.PVSClient
var _ ports.PVSClient = (*Cliente)(nil)

// TokenCache implementa ports.TokenCache usando OAuth2 client_credentials
// con singleflight para evitar N goroutines pidiendo token a la vez.
type TokenCache struct {
	baseURL      string
	clientID     string
	clientSecret string
	mu           sync.Mutex
	token        string
	expiresAt    time.Time
	sf           singleflight.Group
}

func NewTokenCache(baseURL, clientID, clientSecret string) *TokenCache {
	return &TokenCache{
		baseURL:      baseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// Get devuelve un token valido. Si el cache expiro, pide uno nuevo
// usando singleflight para que solo una goroutine haga el request.
func (c *TokenCache) Get(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.expiresAt) {
		c.mu.Unlock()
		return c.token, nil
	}
	c.mu.Unlock()

	// Singleflight: si N goroutines piden token a la vez, solo una
	// ejecuta fetchToken y las demas esperan el mismo resultado.
	result, err, _ := c.sf.Do("pvs-oauth", func() (interface{}, error) {
		return c.fetchToken(ctx)
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

func (c *TokenCache) Invalidate(ctx context.Context) {
	c.mu.Lock()
	c.token = ""
	c.expiresAt = time.Time{}
	c.mu.Unlock()
	// No limpiamos singleflight: el proximo Get() va a cache miss
	// y ejecutara fetchToken con singleflight.
}

// fetchToken hace POST a /oauth2/token con client_credentials.
// Doc PVS: HTTP Basic (clientID:secret) + form solo grant_type=client_credentials.
// No mandar client_id/client_secret en el body.
// Logs: body form + response sanitizada. NUNCA loguea Basic auth ni client_secret.
func (c *TokenCache) fetchToken(ctx context.Context) (string, error) {
	form := url.Values{
		"grant_type": {"client_credentials"},
	}
	formBody := form.Encode()
	oauthURL := c.baseURL + "/oauth2/token"

	req, err := http.NewRequestWithContext(ctx, "POST",
		oauthURL,
		strings.NewReader(formBody))
	if err != nil {
		return "", fmt.Errorf("creando request de token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.clientID, c.clientSecret)

	logPVSRequest(ctx, "POST", oauthURL, []byte(formBody))

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		logPVSResponse(ctx, "POST", oauthURL, 0, durationMs, nil)
		return "", fmt.Errorf("solicitando token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	logPVSResponse(ctx, "POST", oauthURL, resp.StatusCode, durationMs, body)

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("PVS OAuth respondio %d: %s", resp.StatusCode, string(body))
	}

	var resultado struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &resultado); err != nil {
		return "", fmt.Errorf("parseando respuesta OAuth: %w", err)
	}

	c.mu.Lock()
	c.token = resultado.AccessToken
	// Renovamos 60 segundos antes de que expire, para evitar race conditions
	c.expiresAt = time.Now().Add(time.Duration(resultado.ExpiresIn-60) * time.Second)
	c.mu.Unlock()

	logging.From(ctx).Debug("pvs.token.acquired",
		"expires_in_s", resultado.ExpiresIn,
	)

	return c.token, nil
}

// logPVSRequest / logPVSResponse: bodies sanitizados (secrets, QR base64).
// No loguean headers (Authorization/Bearer quedan fuera a proposito).
// Body solo si LOG_HTTP_BODIES esta ON (logging.ConfigureHTTPBodyLogging).
func logPVSRequest(ctx context.Context, method, endpoint string, reqBody []byte) {
	attrs := []any{"method", method, "endpoint", endpoint}
	if body, ok := logging.FormatBodyForLog(reqBody); ok {
		attrs = append(attrs, "body", body)
	}
	logging.From(ctx).Info("pvs.http.request", attrs...)
}

func logPVSResponse(ctx context.Context, method, endpoint string, status int, durationMs int64, respBody []byte) {
	attrs := []any{
		"method", method,
		"endpoint", endpoint,
		"status_code", status,
		"duration_ms", durationMs,
	}
	if body, ok := logging.FormatBodyForLog(respBody); ok {
		attrs = append(attrs, "body", body)
	}
	logging.From(ctx).Info("pvs.http.response", attrs...)
}

// Garantia de compilacion: TokenCache implementa ports.TokenCache
var _ ports.TokenCache = (*TokenCache)(nil)
