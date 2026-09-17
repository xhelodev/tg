package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/xhelodev/tg/internal/config"
	"github.com/xhelodev/tg/pkg/telegram"
)

const botFatherURL = "https://t.me/BotFather"

// cmdLogin guarda el token del bot en la config XDG (sin abrir navegador).
func cmdLogin(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var tokenFile, profile, apiBase string
	var timeout time.Duration
	var jsonOut, skipVerify bool
	fs.StringVar(&tokenFile, "token-file", "", "leer token desde archivo (no interactivo)")
	fs.StringVar(&profile, "profile", "", "perfil donde guardar el token")
	fs.StringVar(&apiBase, "api-base", telegram.DefaultBaseURL, "URL base de la Bot API")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "timeout HTTP para getMe")
	fs.BoolVar(&jsonOut, "json", false, "salida JSON")
	fs.BoolVar(&skipVerify, "skip-verify", false, "no llamar getMe antes de guardar")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			printLoginUsage()
			return ExitOK
		}
		PrintErr("flags: %v", err)
		printLoginUsage()
		return ExitUsage
	}
	if fs.NArg() > 0 {
		PrintErr("argumentos posicionales no admitidos; use --token-file o el modo interactivo")
		printLoginUsage()
		return ExitUsage
	}

	cfg, cfgPath, err := config.Load("")
	if err != nil {
		PrintErr("config: %v", err)
		return ExitConfig
	}

	token, err := obtainLoginToken(tokenFile)
	if err != nil {
		PrintErr("%v", err)
		return ExitConfig
	}
	if err := validateTokenShape(token); err != nil {
		PrintErr("token inválido: %v", err)
		return ExitConfig
	}

	var user *telegram.User
	if !skipVerify {
		client, err := telegram.New(token,
			telegram.WithBaseURL(apiBase),
			telegram.WithTimeout(timeout),
		)
		if err != nil {
			PrintErr("%v", err)
			return ExitConfig
		}
		user, err = client.GetMe(ctx)
		if err != nil {
			// Fallo de verificación: no guardar; exit 4 según spec.
			if jsonOut {
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
					"ok": false, "error": err.Error(), "exit_code": ExitAPI,
				})
			} else {
				PrintErr("verificación getMe falló (no se guardó): %v", err)
			}
			return ExitAPI
		}
	}

	pname := cfg.SetProfileToken(profile, token)
	if err := config.Save(cfgPath, cfg); err != nil {
		PrintErr("guardando config: %v", err)
		return ExitConfig
	}

	// Nunca reimprimir el token.
	if user != nil {
		if jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"ok": true, "profile": pname, "path": cfgPath,
				"id": user.ID, "username": user.Username, "first_name": user.FirstName,
			})
		} else {
			fmt.Printf("ok guardado perfil=%s path=%s id=%d username=@%s\n",
				pname, cfgPath, user.ID, user.Username)
		}
		return ExitOK
	}
	if jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"ok": true, "profile": pname, "path": cfgPath,
		})
	} else {
		fmt.Printf("ok guardado perfil=%s path=%s (sin verificar)\n", pname, cfgPath)
	}
	return ExitOK
}

func printLoginUsage() {
	fmt.Fprintf(os.Stderr, `Uso:
  tg login [--token-file <ruta>] [--profile <nombre>] [--api-base <url>]

Guarda el token del bot en ~/.config/tg/config.json (o $XDG_CONFIG_HOME/tg/config.json).
No abre el navegador. Obtén el token en BotFather: %s

Modo interactivo: pide el token por terminal (la entrada puede verse en eco;
para entrada sin eco usa --token-file).
`, botFatherURL)
}

// obtainLoginToken lee el token desde --token-file o de forma interactiva.
func obtainLoginToken(tokenFile string) (string, error) {
	if tokenFile != "" {
		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("leyendo --token-file: %w", err)
		}
		tok := strings.TrimSpace(string(data))
		if tok == "" {
			return "", fmt.Errorf("--token-file vacío")
		}
		return tok, nil
	}
	return readTokenInteractive()
}

// readTokenInteractive pide el token por stderr y lo lee de /dev/tty o stdin.
// Stdlib only: la entrada puede hacer eco; documentado en el prompt.
func readTokenInteractive() (string, error) {
	fmt.Fprintf(os.Stderr, `tg login — autentica el bot (sin abrir navegador)

Obtén el token en BotFather: %s
  1. Abre el enlace o busca @BotFather en Telegram
  2. /newbot o /token → copia el token

Pega el token y pulsa Enter.
Nota: la entrada puede verse en pantalla (eco). Para evitarlo usa:
  tg login --token-file <ruta>

Token: `, botFatherURL)

	in, closer, err := openLoginInput()
	if err != nil {
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("leyendo token: %w", err)
	}
	tok := strings.TrimSpace(line)
	if tok == "" {
		return "", fmt.Errorf("token vacío")
	}
	return tok, nil
}

// openLoginInput prefiere /dev/tty para que stdin redirigido no bloquee el prompt.
func openLoginInput() (io.Reader, io.Closer, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err == nil {
		return tty, tty, nil
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stdin: %w", err)
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return nil, nil, fmt.Errorf("stdin no es un terminal; use --token-file o ejecute en una TTY")
	}
	return os.Stdin, nil, nil
}
