package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/xhelodev/tg/internal/config"
	"github.com/xhelodev/tg/pkg/telegram"
)

// TokenSource describe de dónde salió el token (para mensajes de error).
type TokenSource struct {
	Token  string
	Origin string // descripción legible
}

// ResolveToken aplica la precedencia §3.
// flagToken / flagTokenFile vienen de --token / --token-file.
func ResolveToken(flagToken, flagTokenFile, profileName string, cfg *config.Config) (TokenSource, error) {
	tried := []string{}

	// 1. --token-file
	if flagTokenFile != "" {
		tried = append(tried, "--token-file "+flagTokenFile)
		data, err := os.ReadFile(flagTokenFile)
		if err != nil {
			return TokenSource{}, fmt.Errorf("leyendo --token-file: %w", err)
		}
		tok := strings.TrimSpace(string(data))
		if err := validateTokenShape(tok); err != nil {
			return TokenSource{}, fmt.Errorf("%w (origen: --token-file); intentados: %s", err, joinTried(tried))
		}
		return TokenSource{Token: tok, Origin: "--token-file"}, nil
	}

	// --token (aviso a stderr; no recomendado)
	if flagToken != "" {
		tried = append(tried, "--token")
		fmt.Fprintln(os.Stderr, "aviso: --token deja el valor en el historial del shell; prefer --token-file o TG_BOT_TOKEN")
		if err := validateTokenShape(flagToken); err != nil {
			return TokenSource{}, fmt.Errorf("%w (origen: --token); intentados: %s", err, joinTried(tried))
		}
		return TokenSource{Token: flagToken, Origin: "--token"}, nil
	}

	// 2. TG_BOT_TOKEN
	if v := strings.TrimSpace(os.Getenv("TG_BOT_TOKEN")); v != "" {
		tried = append(tried, "TG_BOT_TOKEN")
		if err := validateTokenShape(v); err != nil {
			return TokenSource{}, fmt.Errorf("%w (origen: TG_BOT_TOKEN); intentados: %s", err, joinTried(tried))
		}
		return TokenSource{Token: v, Origin: "TG_BOT_TOKEN"}, nil
	}
	tried = append(tried, "TG_BOT_TOKEN")

	// 3. TELEGRAM_BOT_TOKEN
	if v := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); v != "" {
		tried = append(tried, "TELEGRAM_BOT_TOKEN")
		if err := validateTokenShape(v); err != nil {
			return TokenSource{}, fmt.Errorf("%w (origen: TELEGRAM_BOT_TOKEN); intentados: %s", err, joinTried(tried))
		}
		return TokenSource{Token: v, Origin: "TELEGRAM_BOT_TOKEN"}, nil
	}
	tried = append(tried, "TELEGRAM_BOT_TOKEN")

	// 4. Config profile
	if cfg != nil {
		prof, pname, err := cfg.ResolveProfile(profileName)
		if err != nil {
			return TokenSource{}, err
		}
		if pname != "" {
			if prof.TokenEnv != "" {
				tried = append(tried, "profile."+pname+".token_env="+prof.TokenEnv)
				if v := strings.TrimSpace(os.Getenv(prof.TokenEnv)); v != "" {
					if err := validateTokenShape(v); err != nil {
						return TokenSource{}, fmt.Errorf("%w (origen: %s); intentados: %s", err, prof.TokenEnv, joinTried(tried))
					}
					return TokenSource{Token: v, Origin: "profile:" + pname + " env:" + prof.TokenEnv}, nil
				}
			}
			if prof.Token != "" {
				tried = append(tried, "profile."+pname+".token")
				if err := validateTokenShape(prof.Token); err != nil {
					return TokenSource{}, fmt.Errorf("%w (origen: profile token); intentados: %s", err, joinTried(tried))
				}
				return TokenSource{Token: prof.Token, Origin: "profile:" + pname}, nil
			}
		} else {
			tried = append(tried, "config profile")
		}
	} else {
		tried = append(tried, "config")
	}

	return TokenSource{}, fmt.Errorf(
		"token ausente o inválido; orígenes intentados: %s",
		joinTried(tried),
	)
}

func validateTokenShape(tok string) error {
	// Reutilizar la validación del cliente
	_, err := telegram.New(tok, telegram.WithLimiter(nil))
	if err != nil {
		return err
	}
	return nil
}

func joinTried(s []string) string {
	return strings.Join(s, ", ")
}

// EnsureTokenNotLeaked falla el test/assert si el token aparece en s.
func EnsureTokenNotLeaked(output, token string) bool {
	if token == "" {
		return true
	}
	return !strings.Contains(output, token)
}
