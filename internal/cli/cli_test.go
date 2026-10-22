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
	"github.com/xhelodev/tg/pkg/telegram"
)

// TestMain aísla los tests del entorno real: sin config del usuario
// (~/.config/tg con token real) ni tokens en el entorno. Sin esto, los
// tests hacían llamadas reales a la Bot API cuando la máquina tenía token.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tg-test-xdg")
	if err == nil {
		os.Setenv("XDG_CONFIG_HOME", dir)
		defer os.RemoveAll(dir)
	}
	os.Unsetenv("TG_BOT_TOKEN")
	os.Unsetenv("TELEGRAM_BOT_TOKEN")
	os.Exit(m.Run())
}

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
		"@canal": true, "589215": true, "-1001": true, "-100123:42": true,
		// palabras sueltas ya no son chats (evita tragar el texto como destino)
		"alertas": false, "hola": false,
		"-": false, "": false,
	}
	for s, want := range cases {
		if got := looksLikeChat(s); got != want {
			t.Errorf("%q got %v want %v", s, got, want)
		}
	}
}

func TestResolveChat_AliasYTexto(t *testing.T) {
	prof := config.Profile{
		Aliases: map[string]string{"alertas": "-1001234567890:42"},
	}
	g := &GlobalFlags{}
	// alias posicional: se consume como chat
	chat, thread, consumed, err := resolveChat(g, prof, []string{"alertas", "hola"})
	if err != nil || chat != "-1001234567890" || thread != 42 || !consumed {
		t.Fatalf("chat=%q thread=%d consumed=%v err=%v", chat, thread, consumed, err)
	}
	// palabra suelta sin default: error de uso, no destino inventado
	_, _, _, err = resolveChat(g, prof, []string{"hola"})
	if err == nil {
		t.Fatal("esperado error con palabra suelta sin chat")
	}
	// con default_chat la palabra es texto y no se consume
	prof2 := config.Profile{DefaultChat: "589215"}
	chat, _, consumed, err = resolveChat(g, prof2, []string{"hola"})
	if err != nil || chat != "589215" || consumed {
		t.Fatalf("chat=%q consumed=%v err=%v", chat, consumed, err)
	}
}

func TestClassifyMediaRef(t *testing.T) {
	id, url, path, err := classifyMediaRef("https://x.com/a.jpg")
	if err != nil || url == "" || id != "" || path != "" {
		t.Fatal(id, url, path, err)
	}
	id, url, path, err = classifyMediaRef("AgAC123")
	if err != nil || id != "AgAC123" || url != "" || path != "" {
		t.Fatal(id, url, path, err)
	}
	// ruta inexistente con forma de ruta: error local, no file_id
	_, _, _, err = classifyMediaRef("./fto.jpg")
	if err == nil || !strings.Contains(err.Error(), "no encontrado") {
		t.Fatalf("esperado error de archivo, got %v", err)
	}
	// ruta existente
	dir := t.TempDir()
	p := filepath.Join(dir, "foto.jpg")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, url, path, err = classifyMediaRef(p)
	if err != nil || path != p || id != "" || url != "" {
		t.Fatal(id, url, path, err)
	}
}
