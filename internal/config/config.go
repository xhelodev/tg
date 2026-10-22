package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// WarnOnPermissions controla el aviso de permisos del config (Lo desactiva
// el CLI con --quiet/--json para no ensuciar pipelines).
var WarnOnPermissions = true

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
	if WarnOnPermissions {
		if fi, err := os.Stat(path); err == nil {
			if fi.Mode().Perm()&0o077 != 0 {
				fmt.Fprintf(os.Stderr, "aviso: %s tiene permisos %o; se recomienda 0600\n", path, fi.Mode().Perm())
			}
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

// SplitChatThread parsea "chat" o "chat:thread" con Atoi estricta: el thread
// debe ser un entero canónico positivo ("42" sí, "+5" y "007" no), de modo
// que "@canal", IDs negativos ("-100123…") y tokens con ":" no se confundan.
func SplitChatThread(s string) (chatID string, threadID int) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, ":"); i > 0 {
		right := s[i+1:]
		if n, err := strconv.Atoi(right); err == nil && n > 0 && strconv.Itoa(n) == right {
			left := s[:i]
			// Si left parece chat (@x, -100..., dígitos) úsalo
			if left != "" && !strings.Contains(left, "/") {
				return left, n
			}
		}
	}
	return s, 0
}

// HasAlias informa si name existe como alias del perfil (con o sin @).
func (p Profile) HasAlias(name string) bool {
	if p.Aliases == nil {
		return false
	}
	if _, ok := p.Aliases[name]; ok {
		return true
	}
	_, ok := p.Aliases[strings.TrimPrefix(name, "@")]
	return ok
}

// Save escribe la configuración con permisos 0600 (crea el directorio si hace falta).
// El archivo temporal se crea con os.CreateTemp en el mismo directorio: no
// sigue symlinks preexistentes tipo config.json.tmp y el rename es atómico.
func Save(path string, cfg *Config) error {
	if path == "" {
		path = DefaultPath()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creando directorio de config: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando config: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, ".tg-config-*.tmp")
	if err != nil {
		return fmt.Errorf("creando config temporal: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("escribiendo config temporal: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cerrando config temporal: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("ajustando permisos: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("renombrando config: %w", err)
	}
	return nil
}

// SetProfileToken actualiza o crea un perfil con el token y fija default_profile si falta.
func (c *Config) SetProfileToken(profileName, token string) string {
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	name := profileName
	if name == "" {
		name = c.DefaultProfile
	}
	if name == "" {
		name = "default"
	}
	p := c.Profiles[name]
	p.Token = token
	c.Profiles[name] = p
	if c.DefaultProfile == "" {
		c.DefaultProfile = name
	}
	return name
}
