package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitChatThread_Estricto(t *testing.T) {
	cases := []struct {
		in     string
		chat   string
		thread int
	}{
		{"-100123:42", "-100123", 42},
		// formas no canónicas: no se dividen (antes "+5"/"007" pasaban)
		{"-100123:+5", "-100123:+5", 0},
		{"-100123:007", "-100123:007", 0},
		{"-100123:-1", "-100123:-1", 0},
		{"@canal:7", "@canal", 7},
	}
	for _, tc := range cases {
		c, th := SplitChatThread(tc.in)
		if c != tc.chat || th != tc.thread {
			t.Errorf("%q -> %q/%d want %q/%d", tc.in, c, th, tc.chat, tc.thread)
		}
	}
}

func TestHasAlias(t *testing.T) {
	p := Profile{Aliases: map[string]string{"alertas": "-100123:42"}}
	if !p.HasAlias("alertas") || !p.HasAlias("@alertas") {
		t.Fatal("alias existente no detectado")
	}
	if p.HasAlias("otro") {
		t.Fatal("alias inexistente detectado")
	}
	if (Profile{}).HasAlias("x") {
		t.Fatal("perfil sin aliases")
	}
}

// Save no debe seguir symlinks preexistentes tipo config.json.tmp: el temp
// se crea con os.CreateTemp (nombre aleatorio) y se hace rename atómico.
func TestSave_SymlinkTmpNoSeguido(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// señuelo: symlink con el nombre viejo del temp apuntando a un inocente
	victim := filepath.Join(dir, "victima.txt")
	if err := os.WriteFile(victim, []byte("importante"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path+".tmp"); err != nil {
		t.Skip("symlinks no soportados")
	}
	cfg := &Config{Profiles: map[string]Profile{"p": {Token: "tok"}}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "importante" {
		t.Fatal("el symlink tmp fue seguido: víctima sobrescrita")
	}
	// no quedan temps residuales del Save (el señuelo lo creó el test)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if n := e.Name(); n != "config.json" && n != "victima.txt" && n != "config.json.tmp" {
			t.Fatalf("archivo residual: %s", n)
		}
	}
}
