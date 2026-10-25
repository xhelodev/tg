package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const validToken = "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"

func TestNew_TokenValidation(t *testing.T) {
	cases := []struct {
		name  string
		token string
		ok    bool
	}{
		{"vacío", "", false},
		{"malformado corto", "1:abc", false},
		{"sin dos puntos", "123456789AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw", false},
		{"válido", validToken, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(tc.token)
			if tc.ok {
				if err != nil {
					t.Fatalf("esperado ok, got %v", err)
				}
				if c == nil {
					t.Fatal("client nil")
				}
			} else {
				if err == nil {
					t.Fatal("esperado error")
				}
				var ve *ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("esperado ValidationError, got %T", err)
				}
			}
		})
	}
}

func TestGetMe_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/getMe") {
			t.Errorf("path inesperado: %s", r.URL.Path)
		}
		// Verificar que el token está en la URL
		if !strings.Contains(r.URL.Path, validToken) {
			t.Error("token no presente en path")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"id": 123, "is_bot": true, "first_name": "Test", "username": "testbot",
			},
		})
	}))
	defer srv.Close()

	c, err := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	if err != nil {
		t.Fatal(err)
	}
	u, err := c.GetMe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "testbot" || u.ID != 123 {
		t.Fatalf("user inesperado: %+v", u)
	}
}

func TestGetMe_OKFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": false, "error_code": 401, "description": "Unauthorized",
		})
	}))
	defer srv.Close()

	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	_, err := c.GetMe(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != 401 {
		t.Fatalf("esperado APIError 401, got %v", err)
	}
}

func TestSendMessage_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if req["chat_id"] != "589215" || req["text"] != "hola" {
			t.Errorf("payload: %s", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 42,
				"date":       1,
				"chat":       map[string]any{"id": 589215, "type": "private"},
				"text":       "hola",
			},
		})
	}))
	defer srv.Close()

	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	msg, err := c.SendMessage(context.Background(), SendMessageRequest{
		ChatID: "589215", Text: "hola", ParseMode: "HTML",
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.MessageID != 42 {
		t.Fatalf("message_id=%d", msg.MessageID)
	}
}

func TestSendMessage_TooLong(t *testing.T) {
	c, _ := New(validToken, WithLimiter(nil))
	text := strings.Repeat("a", MaxTextLength+1)
	_, err := c.SendMessage(context.Background(), SendMessageRequest{ChatID: "1", Text: text})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("esperado ValidationError, got %v", err)
	}
}

func TestRetry429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false, "error_code": 429, "description": "Too Many Requests",
				"parameters": map[string]any{"retry_after": 0},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"id": 1, "is_bot": true, "first_name": "B", "username": "b",
			},
		})
	}))
	defer srv.Close()

	var sleeps int
	c, _ := New(validToken,
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithLimiter(nil),
		WithRetrySleep(func(ctx context.Context, d time.Duration) error {
			sleeps++
			return nil
		}),
	)
	u, err := c.GetMe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "b" {
		t.Fatal(u)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d want 3", calls.Load())
	}
	if sleeps != 2 {
		t.Fatalf("sleeps=%d want 2", sleeps)
	}
}

func TestRetry429_Exhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": false, "error_code": 429, "description": "Too Many Requests",
			"parameters": map[string]any{"retry_after": 0},
		})
	}))
	defer srv.Close()

	c, _ := New(validToken,
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithLimiter(nil),
		WithRetrySleep(func(ctx context.Context, d time.Duration) error { return nil }),
	)
	_, err := c.GetMe(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != 429 {
		t.Fatalf("esperado 429, got %v", err)
	}
}

func TestContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
	}))
	defer srv.Close()

	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.GetMe(ctx)
	if err == nil {
		t.Fatal("esperado error por cancelación")
	}
}

func TestChatNotFoundHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": false, "error_code": 400, "description": "Bad Request: chat not found",
		})
	}))
	defer srv.Close()

	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	_, err := c.GetChat(context.Background(), "@alguien")
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatal(err)
	}
	if !strings.Contains(ae.Description, "chat_id numérico") {
		t.Fatalf("falta hint: %s", ae.Description)
	}
}

func TestTokenNeverInRedacted(t *testing.T) {
	c, _ := New(validToken, WithLimiter(nil))
	r := c.TokenRedacted()
	if strings.Contains(r, "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw") {
		t.Fatalf("token secreto en redactado: %s", r)
	}
	if !strings.Contains(r, "***") {
		t.Fatalf("esperado ***: %s", r)
	}
}

func TestMigrateToChatID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": false, "error_code": 400, "description": "group chat was upgraded",
			"parameters": map[string]any{"migrate_to_chat_id": int64(-100123)},
		})
	}))
	defer srv.Close()
	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	_, err := c.SendMessage(context.Background(), SendMessageRequest{ChatID: "-1", Text: "x"})
	var ae *APIError
	if !errors.As(err, &ae) || ae.MigrateToChatID != -100123 {
		t.Fatalf("got %v", err)
	}
}

func TestDeletePinReact(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
	}))
	defer srv.Close()
	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	ctx := context.Background()
	if err := c.DeleteMessage(ctx, "1", 10); err != nil {
		t.Fatal(err)
	}
	if err := c.PinChatMessage(ctx, PinChatMessageRequest{ChatID: "1", MessageID: 10}); err != nil {
		t.Fatal(err)
	}
	if err := c.UnpinChatMessage(ctx, "1", 10); err != nil {
		t.Fatal(err)
	}
	if err := c.SetMessageReaction(ctx, SetMessageReactionRequest{
		ChatID: "1", MessageID: 10,
		Reaction: []MessageReactionType{{Type: "emoji", Emoji: "👍"}},
	}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(methods, ",")
	for _, want := range []string{"deleteMessage", "pinChatMessage", "unpinChatMessage", "setMessageReaction"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("falta %s en %s", want, joined)
		}
	}
}

// Los errores de red de net/http incluyen la URL, que lleva el token: nunca
// deben llegar al usuario sin redactar (ni romper errors.Is/As).
func TestNetworkErrorNoTokenLeak(t *testing.T) {
	// Servidor que corta la conexión sin responder.
	cut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("sin Hijacker")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer cut.Close()
	// Proxy que devuelve la URL pedida en una página de error no JSON.
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := http.StatusBadGateway
		if strings.HasSuffix(r.URL.Path, "/getChat") {
			code = http.StatusNotFound
		}
		w.WriteHeader(code)
		_, _ = io.WriteString(w, "<html>error en "+r.URL.Path+"</html>")
	}))
	defer echo.Close()

	cases := []struct {
		name string
		base string
		call func(c *Client) error
	}{
		{"conexión rechazada", "http://127.0.0.1:1", func(c *Client) error { _, err := c.GetMe(context.Background()); return err }},
		{"conexión cortada", cut.URL, func(c *Client) error { _, err := c.GetMe(context.Background()); return err }},
		{"proxy 502 con la URL", echo.URL, func(c *Client) error { _, err := c.GetMe(context.Background()); return err }},
		{"proxy 404 con la URL", echo.URL, func(c *Client) error { _, err := c.GetChat(context.Background(), "1"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := New(validToken, WithBaseURL(tc.base), WithLimiter(nil), WithNoRetry())
			err := tc.call(c)
			if err == nil {
				t.Fatal("esperado error")
			}
			if strings.Contains(err.Error(), validToken) {
				t.Fatalf("token filtrado: %v", err)
			}
			if !strings.Contains(err.Error(), RedactToken(validToken)) {
				t.Fatalf("esperada la forma redactada en %q", err)
			}
		})
	}

	// La cadena de errores sigue intacta: la cancelación se distingue.
	c, _ := New(validToken, WithBaseURL(cut.URL), WithLimiter(nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.GetMe(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(context.Canceled) perdido: %v", err)
	}
}
