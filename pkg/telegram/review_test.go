package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Un 502 con body HTML (proxy/gateway) debe ser error de red, no bug local.
func TestNonJSON_502_EsRed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
	}))
	defer srv.Close()
	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil), WithNoRetry())
	_, err := c.GetMe(context.Background())
	var ne *NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("esperado NetworkError, got %v", err)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("mensaje sin código HTTP: %v", err)
	}
}

// Un 4xx con body no JSON debe salir como error de API (exit 4), no bug (1).
func TestNonJSON_400_EsAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request sin json"))
	}))
	defer srv.Close()
	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil), WithNoRetry())
	_, err := c.GetMe(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != 400 {
		t.Fatalf("esperado APIError 400, got %v", err)
	}
}

// El balde global arranca con 1 token: la segunda petición inmediata al
// mismo chat debe esperar (~1 msg/s por chat), no disparar 30 de golpe.
func TestLimiter_SinRafagaInicial(t *testing.T) {
	l := NewLimiter()
	ctx := context.Background()
	if err := l.Wait(ctx, "1"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := l.Wait(ctx, "1"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 500*time.Millisecond {
		t.Fatalf("segunda petición inmediata (%s): ráfaga no limitada", d)
	}
}

// multipart en streaming: el filename con comillas debe llegar intacto y
// parseable (CreateFormFile escapa el Content-Disposition).
func TestSendMedia_Multipart_FilenameConComillas(t *testing.T) {
	dir := t.TempDir()
	name := `mi "foto" rara.jpg`
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("DATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	var gotName string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart no parseable: %v", err)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": 400, "description": "bad"})
			return
		}
		f, _, err := r.FormFile("photo")
		if err != nil {
			t.Errorf("campo photo ausente: %v", err)
			return
		}
		fh := r.MultipartForm.File["photo"][0]
		gotName = fh.Filename
		f.Close()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "result": map[string]any{
				"message_id": 1, "date": 1,
				"chat": map[string]any{"id": 1, "type": "private"},
			},
		})
	}))
	defer srv.Close()
	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	if _, err := c.SendMedia(context.Background(), SendMediaRequest{
		ChatID: "1", Kind: MediaPhoto, FilePath: path,
	}); err != nil {
		t.Fatal(err)
	}
	if gotName != name {
		t.Fatalf("filename=%q want %q", gotName, name)
	}
}

// Con reintentos 429, el archivo se reenvía completo en cada intento
// (el body streaming se reconstruye rebobinando el archivo).
func TestSendMedia_Multipart_Retry429_Reenvia(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.bin")
	content := strings.Repeat("Z", 5000)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		b, _ := readAllLimit(r)
		if n <= 2 {
			// responder 429 sin consumir todo el body (rechazo temprano)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false, "error_code": 429, "description": "Too Many Requests",
				"parameters": map[string]any{"retry_after": 0},
			})
			return
		}
		if !strings.Contains(string(b), content) {
			t.Errorf("reintento sin contenido completo: %d bytes", len(b))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "result": map[string]any{
				"message_id": 3, "date": 1,
				"chat": map[string]any{"id": 1, "type": "private"},
			},
		})
	}))
	defer srv.Close()
	c, _ := New(validToken,
		WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil),
		WithRetrySleep(func(ctx context.Context, d time.Duration) error { return nil }),
	)
	msg, err := c.SendMedia(context.Background(), SendMediaRequest{
		ChatID: "1", Kind: MediaDocument, FilePath: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.MessageID != 3 {
		t.Fatal(msg.MessageID)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func readAllLimit(r *http.Request) ([]byte, error) {
	buf := make([]byte, 0, 1<<16)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
		if len(buf) > 1<<20 {
			break
		}
	}
	return buf, nil
}
