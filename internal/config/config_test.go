package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitChatThread(t *testing.T) {
	cases := []struct {
		in     string
		chat   string
		thread int
	}{
		{"589215", "589215", 0},
		{"-100123:42", "-100123", 42},
		{"@canal", "@canal", 0},
		{"@canal:7", "@canal", 7},
	}
	for _, tc := range cases {
		c, th := SplitChatThread(tc.in)
		if c != tc.chat || th != tc.thread {
			t.Errorf("%q -> %q/%d want %q/%d", tc.in, c, th, tc.chat, tc.thread)
		}
	}
}

func TestResolveAlias(t *testing.T) {
	p := Profile{Aliases: map[string]string{"alertas": "-100123:42"}}
	chat, th, ok := p.ResolveAlias("alertas")
	if !ok || chat != "-100123" || th != 42 {
		t.Fatalf("%s %d %v", chat, th, ok)
	}
}

func TestSave_PermissionsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tg", "config.json")
	cfg := &Config{
		DefaultProfile: "default",
		Profiles: map[string]Profile{
			"default": {Token: "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"},
		},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm=%o", fi.Mode().Perm())
	}
	loaded, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Profiles["default"].Token != cfg.Profiles["default"].Token {
		t.Fatal("token mismatch")
	}
}

func TestSetProfileToken(t *testing.T) {
	cfg := &Config{}
	name := cfg.SetProfileToken("", "tok")
	if name != "default" || cfg.DefaultProfile != "default" {
		t.Fatalf("%q %+v", name, cfg)
	}
	if cfg.Profiles["default"].Token != "tok" {
		t.Fatal(cfg.Profiles)
	}
}
