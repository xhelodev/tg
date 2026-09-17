package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/xhelodev/tg/pkg/telegram"
)

func TestRun_Version(t *testing.T) {
	code := Run([]string{"version"}, VersionInfo{Version: "1.0.0", Commit: "abc"})
	if code != ExitOK {
		t.Fatal(code)
	}
}

func TestRun_NoToken(t *testing.T) {
	t.Setenv("TG_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	// capturar stderr
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	code := Run([]string{"me"}, VersionInfo{})
	w.Close()
	os.Stderr = old
	buf, _ := io.ReadAll(r)
	if code != ExitConfig {
		t.Fatalf("code=%d out=%s", code, buf)
	}
	if !strings.Contains(string(buf), "TG_BOT_TOKEN") {
		t.Fatalf("debe listar orígenes: %s", buf)
	}
}

func TestRun_Me_DryRun_NoTokenLeak(t *testing.T) {
	t.Setenv("TG_BOT_TOKEN", testToken)
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	code := Run([]string{"me", "--dry-run"}, VersionInfo{})
	w.Close()
	os.Stderr = old
	buf, _ := io.ReadAll(r)
	if code != ExitOK {
		t.Fatalf("code=%d %s", code, buf)
	}
	if strings.Contains(string(buf), "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw") {
		t.Fatalf("token filtrado: %s", buf)
	}
	if !strings.Contains(string(buf), "***") {
		t.Fatalf("esperado redactado: %s", buf)
	}
}

func TestRun_Me_WithServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"id": 99, "is_bot": true, "first_name": "Bot", "username": "mybot",
			},
		})
	}))
	defer srv.Close()

	t.Setenv("TG_BOT_TOKEN", testToken)
	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := Run([]string{"me", "--api-base", srv.URL, "--no-retry"}, VersionInfo{})
	w.Close()
	os.Stdout = oldOut
	out, _ := io.ReadAll(r)
	if code != ExitOK {
		t.Fatalf("code=%d %s", code, out)
	}
	if !strings.Contains(string(out), "mybot") {
		t.Fatalf("out=%s", out)
	}
}

func TestRun_Send_Quiet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 55, "date": 1,
				"chat": map[string]any{"id": 1, "type": "private"},
				"text": "hola",
			},
		})
	}))
	defer srv.Close()
	t.Setenv("TG_BOT_TOKEN", testToken)
	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := Run([]string{"send", "--api-base", srv.URL, "--chat", "1", "--quiet", "hola"}, VersionInfo{})
	w.Close()
	os.Stdout = oldOut
	out, _ := io.ReadAll(r)
	if code != ExitOK {
		t.Fatalf("code=%d %s", code, out)
	}
	if strings.TrimSpace(string(out)) != "55" {
		t.Fatalf("quiet out=%q", out)
	}
}

func TestRun_Send_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 1, "date": 1,
				"chat": map[string]any{"id": 1, "type": "private"},
				"text": "x",
			},
		})
	}))
	defer srv.Close()
	t.Setenv("TG_BOT_TOKEN", testToken)
	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := Run([]string{"send", "--api-base", srv.URL, "--to", "1", "--json", "x"}, VersionInfo{})
	w.Close()
	os.Stdout = oldOut
	out, _ := io.ReadAll(r)
	if code != ExitOK {
		t.Fatal(code, string(out))
	}
	var msg telegram.Message
	if err := json.Unmarshal(out, &msg); err != nil {
		t.Fatal(err, string(out))
	}
}

func TestLooksLikeChat(t *testing.T) {
	cases := map[string]bool{
		"@canal": true, "589215": true, "-1001": true, "alertas": true,
		"-": false, "": false,
	}
	for s, want := range cases {
		if got := looksLikeChat(s); got != want {
			t.Errorf("%q got %v want %v", s, got, want)
		}
	}
}

func TestClassifyMediaRef(t *testing.T) {
	id, url, path := classifyMediaRef("https://x.com/a.jpg")
	if url == "" || id != "" || path != "" {
		t.Fatal(id, url, path)
	}
	id, url, path = classifyMediaRef("AgAC123")
	if id != "AgAC123" {
		t.Fatal(id)
	}
}

// silenciar unused en tests
var _ = bytes.Buffer{}
