package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config es el archivo ~/.config/tg/config.json.
type Config struct {
	DefaultProfile string             `json:"default_profile"`
	Profiles       map[string]Profile `json:"profiles"`
}

// Profile define token, chat por defecto y alias.
type Profile struct {
	Token       string            `json:"token,omitempty"`
	TokenEnv    string            `json:"token_env,omitempty"`
	DefaultChat string            `json:"default_chat,omitempty"`
	Aliases     map[string]string `json:"aliases,omitempty"`
}

// Load lee la configuración desde la ruta XDG estándar o path explícito.
func Load(path string) (*Config, string, error) {
	if path == "" {
		path = DefaultPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Profiles: map[string]Profile{}}, path, nil
		}
		return nil, path, err
	}
	// Aviso de permisos 0600
	if fi, err := os.Stat(path); err == nil {
		if fi.Mode().Perm()&0o077 != 0 {
			fmt.Fprintf(os.Stderr, "aviso: %s tiene permisos %o; se recomienda 0600\n", path, fi.Mode().Perm())
		}
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, path, fmt.Errorf("config JSON inválido: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return &cfg, path, nil
}

// DefaultPath respeta $XDG_CONFIG_HOME.
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "tg", "config.json")
}

// ResolveProfile elige el perfil por nombre o default.
func (c *Config) ResolveProfile(name string) (Profile, string, error) {
	if name == "" {
		name = c.DefaultProfile
	}
	if name == "" {
		// Si solo hay uno, úsalo
		if len(c.Profiles) == 1 {
			for k, v := range c.Profiles {
				return v, k, nil
			}
		}
		return Profile{}, "", nil
	}
	p, ok := c.Profiles[name]
	if !ok {
		return Profile{}, name, fmt.Errorf("perfil %q no encontrado", name)
	}
	return p, name, nil
}

// ResolveAlias resuelve un alias a chat_id (y opcionalmente thread).
// Devuelve chatID, threadID (0 si no hay), ok.
func (p Profile) ResolveAlias(name string) (chatID string, threadID int, ok bool) {
	if p.Aliases == nil {
		return "", 0, false
	}
	key := strings.TrimPrefix(name, "@")
	val, found := p.Aliases[name]
	if !found {
		val, found = p.Aliases[key]
	}
	if !found {
		return "", 0, false
	}
	chatID, threadID = SplitChatThread(val)
	return chatID, threadID, true
}

// SplitChatThread parsea "chat" o "chat:thread".
func SplitChatThread(s string) (chatID string, threadID int) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, ":"); i > 0 {
		// Evitar confundir con @channel o IDs negativos: thread es numérico al final
		right := s[i+1:]
		var n int
		if _, err := fmt.Sscanf(right, "%d", &n); err == nil && right == fmt.Sprintf("%d", n) {
			// Para IDs negativos tipo -100:42 el ":" separa thread.
			// Pero tokens tienen ":" — aquí solo chat destinations.
			left := s[:i]
			// Si left parece chat (@x, -100..., dígitos) úsalo
			if left != "" && !strings.Contains(left, "/") {
				return left, n
			}
		}
	}
	return s, 0
}
