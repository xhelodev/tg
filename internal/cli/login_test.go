package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xhelodev/tg/internal/config"
)

func TestRun_Login_Help(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	code := Run([]string{"login", "--help"}, VersionInfo{})
	w.Close()
	os.Stderr = old
	buf, _ := io.ReadAll(r)
	if code != ExitOK {
		t.Fatalf("code=%d out=%s", code, buf)
	}
	out := string(buf)
	if !strings.Contains(out, "https://t.me/BotFather") {
		t.Fatalf("debe incluir URL de BotFather: %s", out)
	}
	if !strings.Contains(out, "--token-file") {
		t.Fatalf("debe mencionar --token-file: %s", out)
	}
}

func TestRun_Login_TokenFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getMe") {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"id": 42, "is_bot": true, "first_name": "TestBot", "username": "test_bot",
			},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	xdg := filepath.Join(dir, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("TG_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_BOT_TOKEN", "")

	tokPath := filepath.Join(dir, "token.txt")
	if err := os.WriteFile(tokPath, []byte(testToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := Run([]string{
		"login", "--token-file", tokPath, "--api-base", srv.URL,
	}, VersionInfo{})
	w.Close()
	os.Stdout = oldOut
	out, _ := io.ReadAll(r)
	if code != ExitOK {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if strings.Contains(string(out), testToken) {
		t.Fatalf("token filtrado en salida: %s", out)
	}
	if !strings.Contains(string(out), "test_bot") || !strings.Contains(string(out), "42") {
		t.Fatalf("esperado username/id: %s", out)
	}

	cfgPath := filepath.Join(xdg, "tg", "config.json")
	cfg, _, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("permisos=%o, se espera 0600", fi.Mode().Perm())
	}
	prof, name, err := cfg.ResolveProfile("")
	if err != nil || name != "default" {
		t.Fatalf("perfil=%q err=%v", name, err)
	}
	if prof.Token != testToken {
		t.Fatalf("token no guardado en perfil")
	}
}

func TestRun_Login_BadTokenShape(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	tokPath := filepath.Join(dir, "bad")
	if err := os.WriteFile(tokPath, []byte("not-a-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"login", "--token-file", tokPath}, VersionInfo{})
	if code != ExitConfig {
		t.Fatalf("code=%d", code)
	}
	cfgPath := filepath.Join(dir, "xdg", "tg", "config.json")
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("no debió crear config: %v", err)
	}
}

func TestRun_Login_GetMeFails_NoSave(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": false, "error_code": 401, "description": "Unauthorized",
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	xdg := filepath.Join(dir, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	tokPath := filepath.Join(dir, "token.txt")
	if err := os.WriteFile(tokPath, []byte(testToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	code := Run([]string{
		"login", "--token-file", tokPath, "--api-base", srv.URL,
	}, VersionInfo{})
	w.Close()
	os.Stderr = old
	_, _ = io.ReadAll(r)
	if code != ExitAPI {
		t.Fatalf("code=%d want %d", code, ExitAPI)
	}
	cfgPath := filepath.Join(xdg, "tg", "config.json")
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("no debió guardar config tras fallo getMe")
	}
}

func TestRun_Login_PromptMentionsBotFather(t *testing.T) {
	// Sin --token-file y sin TTY usable: forzamos fallo de openLoginInput
	// comprobando que printLoginUsage / prompt incluye BotFather vía --help.
	// (el prompt interactivo se ejercita solo en TTY real)
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	_ = Run([]string{"login", "-h"}, VersionInfo{})
	w.Close()
	os.Stderr = old
	buf, _ := io.ReadAll(r)
	if !strings.Contains(string(buf), botFatherURL) {
		t.Fatalf("%s", buf)
	}
}
