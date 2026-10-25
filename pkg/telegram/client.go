package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DefaultBaseURL es la URL oficial de la Bot API.
const DefaultBaseURL = "https://api.telegram.org"

// BotAPIVersion es la versión objetivo de compatibilidad.
const BotAPIVersion = "10.3"

// Patrón de token válido: dígitos:alfanumérico.
var tokenPattern = regexp.MustCompile(`^[0-9]{6,}:[A-Za-z0-9_-]{30,}$`)

// Option configura el Client.
type Option func(*Client)

// WithBaseURL apunta a un Bot API server local u otro endpoint.
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

// WithHTTPClient inyecta un *http.Client personalizado (tests, proxies).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// WithTimeout fija el timeout por defecto del http.Client.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if c.http == nil {
			c.http = &http.Client{}
		}
		c.http.Timeout = d
	}
}

// WithNoRetry desactiva reintentos ante 429.
func WithNoRetry() Option {
	return func(c *Client) { c.noRetry = true }
}

// WithLimiter inyecta un limitador (nil desactiva).
func WithLimiter(l *Limiter) Option {
	return func(c *Client) { c.limiter = l }
}

// WithRetrySleep inyecta la función de espera (tests con retry_after=0).
func WithRetrySleep(fn func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.retrySleep = fn }
}

// VerboseFunc se llama con URL redactada, código HTTP y duración.
type VerboseFunc func(method, redactedURL string, statusCode int, duration time.Duration)

// WithVerbose registra peticiones (token siempre redactado).
func WithVerbose(fn VerboseFunc) Option {
	return func(c *Client) { c.verbose = fn }
}

// Client es el cliente tipado de la Telegram Bot API.
type Client struct {
	token      string
	baseURL    string
	http       *http.Client
	limiter    *Limiter
	noRetry    bool
	retrySleep func(context.Context, time.Duration) error
	verbose    VerboseFunc
}

// New crea un Client validando el token antes de cualquier petición.
func New(token string, opts ...Option) (*Client, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, &ValidationError{Message: "token ausente"}
	}
	if !tokenPattern.MatchString(token) {
		return nil, &ValidationError{
			Message: "token malformado: debe cumplir ^[0-9]{6,}:[A-Za-z0-9_-]{30,}$",
		}
	}
	c := &Client{
		token:   token,
		baseURL: DefaultBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
		limiter: NewLimiter(),
		retrySleep: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
	for _, o := range opts {
		o(c)
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 30 * time.Second}
	}
	return c, nil
}

// TokenRedacted devuelve el token con la parte secreta ocultada.
func (c *Client) TokenRedacted() string {
	return RedactToken(c.token)
}

// RedactToken oculta la parte secreta de un token (nunca lo imprime completo).
func RedactToken(token string) string {
	i := strings.IndexByte(token, ':')
	if i < 0 {
		return "***"
	}
	return token[:i+1] + "***"
}

// BaseURL expone la base configurada.
func (c *Client) BaseURL() string { return c.baseURL }

// IsLocalAPI indica si apunta a un servidor distinto del oficial (límites mayores).
func (c *Client) IsLocalAPI() bool {
	return c.baseURL != "" && c.baseURL != DefaultBaseURL
}

func (c *Client) methodURL(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
}

func (c *Client) redactedURL(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.TokenRedacted(), method)
}

// redact reemplaza el token por su forma redactada en s.
func (c *Client) redact(s string) string {
	if c.token == "" {
		return s
	}
	return strings.ReplaceAll(s, c.token, c.TokenRedacted())
}

// netErr envuelve err como NetworkError sin el token: los errores de
// net/http (*url.Error) incluyen la URL completa, y esa URL lleva el token
// ("…/bot<TOKEN>/getMe"). El *url.Error se corrige en sitio (es de esta
// petición y de nadie más), así la cadena sigue limpia y errors.Is/As
// funcionan. redactedError es la última red para cualquier otro mensaje.
func (c *Client) netErr(err error) *NetworkError {
	var ue *url.Error
	if errors.As(err, &ue) {
		ue.URL = c.redact(ue.URL)
	}
	if c.token != "" && strings.Contains(err.Error(), c.token) {
		err = &redactedError{msg: c.redact(err.Error()), err: err}
	}
	return &NetworkError{Err: err}
}

// redactedError sustituye el mensaje de un error que contenía el token.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// doJSON envía un body JSON y deserializa el result.
func (c *Client) doJSON(ctx context.Context, method string, payload any, chatID string, result any) error {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
	}
	return c.do(ctx, method, func() (io.Reader, string, error) {
		if body == nil {
			return nil, "application/json", nil
		}
		return bytes.NewReader(body), "application/json", nil
	}, chatID, result)
}

// do envía la petición con reintentos ante 429. newBody se invoca una vez
// por intento (permite streaming multipart: cada reintento reconstruye el
// cuerpo). Si el reader implementa io.ReadCloser se cierra tras cada
// intento, lo que desbloquea a un productor (pipe) si el servidor abortó.
func (c *Client) do(ctx context.Context, method string, newBody func() (io.Reader, string, error), chatID string, result any) error {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx, chatID); err != nil {
			return err
		}
	}

	maxAttempts := 3 // intento inicial + 2 reintentos ante 429
	if c.noRetry {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		body, contentType, err := newBody()
		if err != nil {
			// Fallo local de E/S (p.ej. f.Seek): no es error del usuario.
			return fmt.Errorf("construyendo body: %w", err)
		}

		endpoint := c.methodURL(method)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
		if err != nil {
			return c.netErr(err)
		}
		if contentType != "" && body != nil {
			req.Header.Set("Content-Type", contentType)
		}

		start := time.Now()
		resp, err := c.http.Do(req)
		dur := time.Since(start)
		if c.verbose != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			c.verbose(method, c.redactedURL(method), status, dur)
		}
		if err != nil {
			// Fallo de conexión previo al envío completo: reintentable en getMe etc.
			// Para send, solo reintentamos 429 (manejado abajo); aquí devolvemos red.
			return c.netErr(err)
		}

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		// Cerrar el body de la petición desbloquea al productor (pipe)
		// si el servidor cerró temprano sin consumir todo.
		if rc, ok := body.(io.ReadCloser); ok {
			_ = rc.Close()
		}
		if readErr != nil {
			return c.netErr(readErr)
		}

		var envelope apiResponse
		if err := json.Unmarshal(respBody, &envelope); err != nil {
			// Respuesta no JSON: clasificar por código HTTP, no como bug local.
			// Un proxy puede repetir la URL (con el token) en su página de
			// error: redactar antes de truncar, para no dejar un prefijo.
			snippet := c.redact(string(respBody))
			if resp.StatusCode >= 500 {
				return &NetworkError{Err: fmt.Errorf(
					"respuesta no JSON del servidor (HTTP %d): %.200s", resp.StatusCode, snippet)}
			}
			return &APIError{
				Code:        resp.StatusCode,
				Description: fmt.Sprintf("respuesta no JSON (HTTP %d): %.200s", resp.StatusCode, snippet),
			}
		}

		if envelope.OK {
			if result != nil && len(envelope.Result) > 0 {
				if err := json.Unmarshal(envelope.Result, result); err != nil {
					return fmt.Errorf("decode result: %w", err)
				}
			}
			return nil
		}

		apiErr := &APIError{
			Code:        envelope.ErrorCode,
			Description: envelope.Description,
		}
		if envelope.Parameters != nil {
			apiErr.RetryAfter = envelope.Parameters.RetryAfter
			apiErr.MigrateToChatID = envelope.Parameters.MigrateToChatID
		}
		// Mejorar mensaje si es chat not found con @username
		if apiErr.Code == 400 && strings.Contains(strings.ToLower(apiErr.Description), "chat not found") {
			apiErr.Description = apiErr.Description +
				"; nota: @username solo funciona para canales/supergrupos públicos. " +
				"Para usuarios privados use el chat_id numérico"
		}

		if apiErr.Code == 429 && !c.noRetry && attempt < maxAttempts-1 {
			wait := time.Duration(apiErr.RetryAfter) * time.Second
			if wait <= 0 {
				wait = time.Second
			}
			// jitter pequeño
			jitter := time.Duration(rand.Intn(200)) * time.Millisecond
			wait += jitter
			if err := c.retrySleep(ctx, wait); err != nil {
				return err
			}
			continue
		}
		return apiErr
	}
	return fmt.Errorf("agotados reintentos")
}

// BoolPtr ayuda a construir campos opcionales *bool.
func BoolPtr(v bool) *bool { return &v }

// IntPtr ayuda a construir campos opcionales *int.
func IntPtr(v int) *int { return &v }

// StringPtr ayuda a construir campos opcionales *string.
func StringPtr(v string) *string { return &v }
