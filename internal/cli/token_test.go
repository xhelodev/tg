package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xhelodev/tg/internal/config"
)

const testToken = "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"

func TestResolveToken_Env(t *testing.T) {
	t.Setenv("TG_BOT_TOKEN", testToken)
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	ts, err := ResolveToken("", "", "", &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if ts.Token != testToken || ts.Origin != "TG_BOT_TOKEN" {
		t.Fatalf("%+v", ts)
	}
}

func TestResolveToken_TokenFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tok")
	if err := os.WriteFile(p, []byte(testToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TG_BOT_TOKEN", "should-not-use-this-invalid")
	ts, err := ResolveToken("", p, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ts.Origin != "--token-file" {
		t.Fatal(ts.Origin)
	}
}

func TestResolveToken_Missing(t *testing.T) {
	t.Setenv("TG_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	_, err := ResolveToken("", "", "", &config.Config{})
	if err == nil {
		t.Fatal("esperado error")
	}
	if !strings.Contains(err.Error(), "TG_BOT_TOKEN") {
		t.Fatalf("debe listar orígenes: %v", err)
	}
}

func TestResolveToken_Malformed(t *testing.T) {
	t.Setenv("TG_BOT_TOKEN", "bad")
	_, err := ResolveToken("", "", "", nil)
	if err == nil {
		t.Fatal("esperado error")
	}
}

// El invariant "nunca imprimir el token" (AGENTS.md): un valor inválido
// pasado por el usuario no debe aparecer jamás en los mensajes de error.
func TestValidateTokenShape_NoLeak(t *testing.T) {
	secret := "AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
	bad := "12:" + secret // malformado (pocas cifras) pero con "secreto"
	err := validateTokenShape(bad)
	if err == nil {
		t.Fatal("esperado error de token malformado")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), bad) {
		t.Fatalf("el error filtra el valor del token: %v", err)
	}
}
