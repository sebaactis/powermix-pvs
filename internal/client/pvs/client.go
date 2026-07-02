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

	"golang.org/x/time/rate"
	"golang.org/x/sync/singleflight"

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

// New crea un Cliente listo para usar.
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

// Opcion permite configurar el cliente (functional options pattern).
type Opcion func(*Cliente)

// ConHTTPClient reemplaza el http.Client por defecto (util en tests).
func ConHTTPClient(hc *http.Client) Opcion {
	return func(c *Cliente) { c.httpClient = hc }
}

// ConRateLimit cambia el rate limiter (requests/segundo, burst).
func ConRateLimit(rps int, burst int) Opcion {
	return func(c *Cliente) {
		c.rateLimiter = rate.NewLimiter(rate.Limit(rps), burst)
	}
}

// GenerateQR implementa ports.PVSClient.
// Endpoint real: POST /external/connect/api/v1/qr/pvs
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/external/connect/api/v1/qr/pvs",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creando request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doConRetry(ctx, httpReq)
	if err != nil {
		return nil, &httpStatusError{err: err, codigoHTTP: 0}
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, c.mapearError(resp.StatusCode, respBytes)
	}

	var pvsResp struct {
		QrID    string `json:"qrId"`
		QrImage string `json:"qrImage"`
		ExpiresAt string `json:"expiresAt,omitempty"`
	}
	if err := json.Unmarshal(respBytes, &pvsResp); err != nil {
		return nil, fmt.Errorf("parseando respuesta PVS: %w", err)
	}

	return &ports.PVSQRResponse{
		QrID:    pvsResp.QrID,
		QrImage: pvsResp.QrImage,
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

	resp, err := c.doConRetry(ctx, httpReq)
	if err != nil {
		return nil, &httpStatusError{err: err, codigoHTTP: 0}
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, c.mapearError(resp.StatusCode, respBytes)
	}

	var pvsResp struct {
		StateID int    `json:"stateId"`
		Status  string `json:"status,omitempty"`
	}
	if err := json.Unmarshal(respBytes, &pvsResp); err != nil {
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/external/connect/api/v1/qr/pvs/reverse",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creando request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doConRetry(ctx, httpReq)
	if err != nil {
		return nil, &httpStatusError{err: err, codigoHTTP: 0}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, c.mapearError(resp.StatusCode, respBytes)
	}

	return &ports.PVSReverseResponse{Success: true}, nil
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

// httpStatusError preserva el codigo HTTP original para que el handler
// pueda decidir si es 400 (bad request) o 500 (retry).
type httpStatusError struct {
	err        error
	codigoHTTP int
}

func (e *httpStatusError) Error() string { return e.err.Error() }
func (e *httpStatusError) Unwrap() error { return e.err }
func (e *httpStatusError) CodigoHTTP() int { return e.codigoHTTP }

// Garantia de compilacion: Cliente implementa ports.PVSClient
var _ ports.PVSClient = (*Cliente)(nil)

// --- TokenCache con Singleflight ---

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

// NewTokenCache crea un cache de token OAuth2.
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

// Invalidate fuerza a Get() a pedir un token nuevo en el proximo llamado.
func (c *TokenCache) Invalidate(ctx context.Context) {
	c.mu.Lock()
	c.token = ""
	c.expiresAt = time.Time{}
	c.mu.Unlock()
	// No limpiamos singleflight: el proximo Get() va a cache miss
	// y ejecutara fetchToken con singleflight.
}

// fetchToken hace POST a /oauth2/token con client_credentials.
func (c *TokenCache) fetchToken(ctx context.Context) (string, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/oauth2/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creando request de token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("solicitando token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("PVS OAuth respondio %d: %s", resp.StatusCode, string(body))
	}

	var resultado struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resultado); err != nil {
		return "", fmt.Errorf("parseando respuesta OAuth: %w", err)
	}

	c.mu.Lock()
	c.token = resultado.AccessToken
	// Renovamos 60 segundos antes de que expire, para evitar race conditions
	c.expiresAt = time.Now().Add(time.Duration(resultado.ExpiresIn-60) * time.Second)
	c.mu.Unlock()

	return c.token, nil
}

// Garantia de compilacion: TokenCache implementa ports.TokenCache
var _ ports.TokenCache = (*TokenCache)(nil)
