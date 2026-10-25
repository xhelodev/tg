# AGENTS.md — guía para agentes de código

Docs humanas: [README.md](README.md). Este archivo es la referencia compacta para LLMs/agentes.
Workflow de PR (build/test, revisor automático): [skills/tg-dev/SKILL.md](skills/tg-dev/SKILL.md). Skill para *usar* el CLI: [skills/tg/SKILL.md](skills/tg/SKILL.md).

## Qué es

- **`cmd/tg`**: CLI para la [Telegram Bot API](https://core.telegram.org/bots/api) (**10.3**).
- **`pkg/telegram`**: cliente tipado reutilizable (stdlib only; sin CLI ni `os.Exit`).
- Go ≥ 1.22, **cero deps externas**, binario estático.

## Hard constraints

| Regla | Detalle |
|-------|---------|
| Zero deps | Solo stdlib en `go.mod`. No añadir módulos de terceros. |
| Nunca imprimir el token | stdout/stderr/logs/JSON de error: nunca el valor del token. |
| `CGO_ENABLED=0` | Builds (`go build` / install local) siempre estáticos. |
| Exit codes | `0` ok · `1` bug · `2` uso · `3` config/token · `4` API `ok:false` · `5` red · `130` interrumpido (Ctrl-C) |
| Librería limpia | `pkg/telegram` no llama `os.Exit` ni imprime a stdout. |

## Layout

```
cmd/tg/          # main delgado → cli.Run
pkg/telegram/    # Client, messages, media, chats, rate limit
internal/cli/    # flags, comandos, token resolve, output, exit
internal/config/ # ~/.config/tg/config.json (XDG)
```

## Auth y config

**Precedencia del token:**

1. `--token-file <ruta>`
2. `--token` (desaconsejado; deja rastro en historial)
3. `TG_BOT_TOKEN`
4. `TELEGRAM_BOT_TOKEN` (alias Hermes)
5. Perfil en config (`token_env` o `token`)

**Config:** `$XDG_CONFIG_HOME/tg/config.json` (default `~/.config/tg/config.json`), permisos preferidos `0600`.

```json
{
  "default_profile": "personal",
  "profiles": {
    "personal": {
      "token": "…",
      "default_chat": "589215",
      "aliases": { "alertas": "-1001234567890:42" }
    }
  }
}
```

Sin token → **exit 3** (lista orígenes intentados; sin revelar secretos).

## Comandos (superficie v1)

```bash
tg login                              # guarda token (getMe + config 0600)
tg login --token-file /ruta/segura/token
tg me
tg send --chat 589215 "hola"
echo hola | tg send 589215
tg send 589215 -                      # texto desde stdin
tg send --to @canal --parse-mode HTML "<b>alerta</b>"
tg send --photo ./foto.jpg --chat 589215 --caption "cap"
tg send --split --chat 589215 "…texto largo…"
tg edit --chat ID --message-id N "nuevo"
tg edit-caption --chat ID --message-id N --caption "…"
tg delete|pin|unpin --chat ID --message-id N
tg react --chat ID --message-id N --emoji "👍"
tg forward|copy --chat DEST --from SRC --message-id N
tg action --chat ID --action typing
tg chat --chat @canal
tg member --chat @canal
tg whoami [--once] [--no-reply]       # escucha (getUpdates) y responde chat_id/user_id
tg version
tg update [--ref vX.Y.Z|main] [--dry-run] # go install @latest sobre el binario actual
```

Los flags van después del comando (`tg me --json`). Flags globales útiles: `--json`, `--quiet`/`-q`, `--dry-run`, `--verbose`, `--silent`/`-s`,
`--protect`, `--reply-to`, `--no-preview`, `--api-base`, `--timeout`, `--profile`, `--no-retry`.

## Scripting

- `--quiet` / `-q`: mínimo ruido en stderr; útil en pipes.
- Texto: argumento, `-`, o stdin (`echo … | tg send CHAT`).
- `--json`: máquina-legible en stdout.
- Respeta exit codes arriba; no parses el mensaje de error para control de flujo si puedes usar el code.
- `@username` **no** resuelve DMs privados → usa chat_id numérico.

## Fuera de alcance (v1)

Webhooks, daemon `getUpdates` (salvo el helper interactivo `tg whoami`), callbacks, pagos, polls, stories, inline mode.

## Build / test

```bash
make test    # go test ./…
make vet     # go vet ./…
make build   # → bin/tg (CGO_ENABLED=0)
make install # → $(go env GOPATH)/bin/tg
make cross   # dist/ multi OS/arch
```

Actualizar: `tg update` (requiere Go; exit 3 sin Go, 5 si falla `go install`). `tg version` usa `debug.ReadBuildInfo` si no hubo `-ldflags`.

Instalación: `go install github.com/xhelodev/tg/cmd/tg@latest` (ver README § «Instalación rápida»); alternativa `./install.sh` o `make install` desde un clone.

## Notas para agentes

- Preferir cambios mínimos; no introducir deps.
- Tests con `httptest` / fixtures locales; no tokens reales en el repo.
- Al tocar auth/login: verificar que el token nunca aparece en output de tests ni en errores.
