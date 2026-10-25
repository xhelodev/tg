# tg

CLI en Go para la [Telegram Bot API](https://core.telegram.org/bots/api) (**Bot API 10.3**).
Cliente librería (`pkg/telegram`) + CLI (`cmd/tg`). Cero dependencias externas, binario estático (`CGO_ENABLED=0`).

## Requisitos

- Go **≥ 1.22** y `git` (solo para compilar/instalar)
- Un bot de Telegram: habla con [@BotFather](https://t.me/BotFather), `/newbot`, y guarda el token (nunca lo subas a git)

## Instalación

One-liner (clona, compila e instala; si `$GOPATH/bin` no está en tu `PATH`, añade el `export` a tu rc):

```bash
curl -fsSL https://raw.githubusercontent.com/xhelodev/tg/main/install.sh | bash
```

Alternativas:

```bash
# con Go, vía module proxy
go install github.com/xhelodev/tg/cmd/tg@latest

# clonando el repo
git clone https://github.com/xhelodev/tg.git && cd tg && ./install.sh

# con make, desde un clone
make install                              # → $(go env GOPATH)/bin/tg
make install-user PREFIX=$HOME/.local/bin # o TG_INSTALL_DIR=...
```

El script es legible a propósito: revísalo antes de ejecutarlo.
Opciones: `./install.sh --help` (`--ref`, `--dir`, `--dry-run`); si reutiliza un clone
existente lo actualiza antes de compilar; el `export PATH` al rc se desactiva con `TG_NO_PATH=1`.

### Actualizar

```bash
tg update                 # go install …/cmd/tg@latest en el mismo directorio del binario
tg update --ref v0.2.0    # versión concreta (o --ref main para lo último de main)
tg update --dry-run       # muestra el comando sin ejecutarlo
```

Requiere Go en `PATH`. Reemplaza el binario en ejecución (fija `GOBIN` a su
directorio), así que también sirve si lo instalaste con `TG_INSTALL_DIR`.
Exit `3` si no hay Go, `5` si `go install` falla (red/proxy).
Sin tags publicados, `@latest` es el último commit de `main` y el module proxy
puede tardar unos minutos en verlo; `--ref main` lo fuerza.

## Configuración mínima

```bash
tg login                 # interactivo; guarda el token en ~/.config/tg/ (permisos 0600)
tg me                    # valida el token
tg send --chat <chat_id> "hola"
```

Precedencia de autenticación: `--token-file` > `--token` (desaconsejado) > `TG_BOT_TOKEN` >
`TELEGRAM_BOT_TOKEN` > perfil en `~/.config/tg/config.json`. Sin token → exit 3.

Para el `chat_id`: corre `tg whoami` y escríbele al bot desde el chat que quieras;
responde con el `chat_id` y tu `user_id` (detalle en [Cómo obtener el `chat_id`](#cómo-obtener-el-chat_id)).
Los grupos/canales usan IDs negativos; `@username` solo resuelve canales públicos.

## Comandos

| Comando | Qué hace |
|---------|----------|
| `tg login` | guarda el token del bot (valida con `getMe`) |
| `tg me` | info del bot (valida token) |
| `tg send` | envía texto/foto/audio/documento (`--photo`, `--parse-mode`, `--split`, stdin con `-`) |
| `tg edit` / `tg edit-caption` | edita texto o caption de un mensaje |
| `tg delete` | borra un mensaje |
| `tg pin` / `tg unpin` | fija / desfija un mensaje |
| `tg react` | reacciona a un mensaje (`--emoji`) |
| `tg forward` / `tg copy` | reenvía o copia un mensaje |
| `tg action` | indicador de estado (`typing`, etc.) |
| `tg chat` / `tg member` | info de un chat / de un miembro |
| `tg whoami` | escucha y responde con el `chat_id`/`user_id` de quien le escribe (`--once`, `--no-reply`) |
| `tg update` | actualiza el binario |
| `tg version` | versión del binario y Bot API objetivo |

Los flags van **después** del comando (`tg me --json`, no `tg --json me`).
Flags globales útiles: `--json`, `--quiet/-q`, `--dry-run`, `--verbose`, `--silent/-s`,
`--protect`, `--reply-to`, `--no-preview`, `--api-base`, `--timeout`, `--profile`, `--no-retry`.

Ejemplos rápidos:

```bash
tg send 589215 "hola"
echo hola | tg send 589215
tg send --to @mi_canal --parse-mode HTML "<b>alerta</b>"
tg send --photo ./foto.jpg --chat 589215 --caption "cap"
tg send --chat -100123 --thread 42 "en el tópico"
```

## Documentación

Para agentes de código: [`AGENTS.md`](AGENTS.md) (también [`llms.txt`](llms.txt)).

### Skill para agentes (workflow de este repo)

[`skills/tg/SKILL.md`](skills/tg/SKILL.md) enseña a un agente cómo trabajar aquí:
constraints duros, build/test antes de cada PR y el flujo de PR (con el revisor
automático). Instálala así:

**Proyecto** (equipo; se comparte por git):

```bash
# Claude Code / claude CLI — skill de proyecto
mkdir -p .claude/skills && cp -r skills/tg .claude/skills/

# Cursor — regla de proyecto (una sola línea de frontmatter extra)
mkdir -p .cursor/rules && { printf -- '---\ndescription: tg repo workflow — read before coding or opening PRs\nglobs:\nalwaysApply: true\n---\n\n'; cat skills/tg/SKILL.md; } > .cursor/rules/tg.mdc

# OpenCode, Codex CLI y la mayoría de agentes: ya leen AGENTS.md; no hay que instalar nada.
```

**Personal / global** (todos tus proyectos en esta máquina) — un comando para CLI + skill:

```bash
curl -fsSL https://raw.githubusercontent.com/xhelodev/tg/main/install.sh | bash -s -- --skill
# solo la skill (sin compilar nada):
curl -fsSL https://raw.githubusercontent.com/xhelodev/tg/main/install.sh | bash -s -- --skill-only
# o, desde un clone:
make install-skill
```

Detecta los agentes presentes en la máquina e instala donde existan:
**Claude Code** (`~/.claude/skills/tg/`), **OpenCode** (`~/.config/opencode/skills/tg/`),
**Hermes** (`~/.hermes/skills/tg/`), **Codex/ChatGPT** (`~/.codex/skills/tg/`);
si no encuentra ninguno, cae a `~/.claude/skills/tg/`.
**Cursor** no tiene reglas globales por fichero: se instala project-level como
`.cursor/rules/tg.mdc` si el directorio actual tiene `.cursor/rules/`.
Vuelve a ejecutar el comando para actualizar la skill.

Instalación manual alternativa:

```bash
# Claude Code
mkdir -p ~/.claude/skills && cp -r skills/tg ~/.claude/skills/

# OpenCode
mkdir -p ~/.config/opencode/skills && cp -r skills/tg ~/.config/opencode/skills/
```

Si ya tienes el repo clonado, un symlink evita duplicar el archivo
(`ln -s $(pwd)/skills/tg ~/.claude/skills/tg`) y la skill se actualiza con `git pull`.

### Autenticación (detalle)

`tg login` valida el token con `getMe`, lo guarda en el perfil activo de
`~/.config/tg/config.json` (`$XDG_CONFIG_HOME`) con permisos `0600`, e imprime
el username/id del bot (nunca el token). La entrada interactiva puede hacer eco;
usa `--token-file` si prefieres no mostrarlo.

```bash
tg login --token-file /ruta/segura/token   # no interactivo
export TG_BOT_TOKEN='123456789:AAH...'     # por entorno
```

Ejemplo de config:

```json
{
  "default_profile": "personal",
  "profiles": {
    "personal": {
      "token_env": "TG_BOT_TOKEN",
      "default_chat": "589215",
      "aliases": { "alertas": "-1001234567890:42" }
    }
  }
}
```

### Cómo obtener el `chat_id`

`tg whoami` deja el bot escuchando; escríbele desde donde quieras y responde
(en ese mismo chat) con el `chat_id`, el `thread_id` si es un tópico y el
`user_id` de quien escribió. Si le **reenvías** un mensaje de otra persona,
también te da el id de esa persona (salvo que lo oculte por privacidad).

```bash
tg whoami                    # escucha hasta Ctrl-C
tg whoami --once -q          # sale con el primer mensaje; imprime solo el destino (p.ej. -100123:42)
tg whoami --no-reply --json  # no responde en el chat; una línea JSON por mensaje en stdout
```

El bot responde algo así:

```
chat_id: -1001234567890
tipo: supergroup (Equipo)
thread_id: 42
user_id: 589215 (@marce, Marcelo A)

tg send --chat -1001234567890:42 "…"
```

- **Grupos:** con privacy mode (default de BotFather) el bot solo ve comandos
  y menciones → escribe `/whoami` o `@tu_bot`. Los supergrupos/canales usan IDs negativos tipo `-100…`.
- **Canales:** el bot tiene que ser admin para ver (y responder) los posts.
- **Mensajes viejos:** lo que llegó antes de arrancar se descarta sin responder (se lista en stderr).
- **Un solo lector:** usa `getUpdates`, así que no funciona si el bot tiene
  webhook, y si otro proceso usa el mismo token (p.ej. Hermes) se pisan → exit 4
  (conflicto 409). Usa un bot aparte o para el otro proceso un momento.
- **Sin `tg`:** mándale un mensaje al bot y lee `getUpdates` a mano:
  ```bash
  curl -s "https://api.telegram.org/bot$TG_BOT_TOKEN/getUpdates" | python3 -m json.tool
  ```
  Busca `message.chat.id`.
- **Pitfall:** `@username` **no** resuelve usuarios privados. Aunque el bot tenga un DM abierto, hace falta el ID numérico. `@canal_publico` sí funciona para canales/supergrupos públicos.

## Exit codes

| code | significado |
|------|-------------|
| 0 | éxito |
| 1 | error inesperado / bug |
| 2 | uso (flag/argumento) |
| 3 | config o token ausente/inválido |
| 4 | API `ok:false` |
| 5 | red / DNS / timeout |
| 130 | interrumpido por el usuario (Ctrl-C) |

## Límites (Bot API cloud)

| recurso | límite |
|---------|--------|
| texto | 1–4096 caracteres (tras entidades) |
| caption | 0–1024 |
| foto por URL | 5 MB |
| otros por URL | 20 MB |
| foto multipart | 10 MB |
| otros multipart | 50 MB |
| file_id | sin límite de tamaño de descarga |
| Bot API server local | hasta 2000 MB |

`sendDocument` por URL solo acepta **.PDF** y **.ZIP**; el resto, sube ruta local.

Los `file_id` son **por bot**: no se reutilizan entre bots.

## Trampas conocidas

1. `@username` falla para usuarios privados → usa ID numérico.
2. Con `parse_mode=HTML` (el default) hay que escapar `&`, `<`, `>`: el texto se envía tal cual
   (stdin incluido). Para texto plano usa `--html-escape` o `--parse-mode ""`.
3. Sin `--thread` en un foro, el mensaje cae en «General».
4. En DM el bot solo borra **sus** mensajes.
5. `deleteMessage` necesita el `message_id` exacto; no hay listado histórico.
6. `--split` numera partes `[i/N]` y evita cortar etiquetas HTML abiertas.

## Bot API server local

```bash
tg me --api-base http://localhost:8081
tg send --api-base http://localhost:8081 --video ./video-grande.mp4 --chat 589215
```

Con `--api-base` distinto de `https://api.telegram.org` los límites de subida suben a 2000 MB.

## Librería

```go
import "github.com/xhelodev/tg/pkg/telegram"

c, err := telegram.New(os.Getenv("TG_BOT_TOKEN"))
msg, err := c.SendMessage(ctx, telegram.SendMessageRequest{
    ChatID: "589215", Text: "hola", ParseMode: "HTML",
})
```

## Desarrollo

```bash
make test
make vet
make build
make cross   # linux/darwin/windows × amd64/arm64
make smoke   # 21 checks contra Telegram real; requiere TG_BOT_TOKEN y TG_TEST_CHAT
```

### CI

`.github/workflows/ci.yml` corre en cada PR y push a `main`: tests (Go 1.22 y
stable, Linux/macOS, `-race`), shellcheck + `install.sh` real, y `make smoke`
contra la Bot API real. Al final manda **un mensaje a `TG_TEST_CHAT`** con el
resumen (tests pasados, checks del smoke o cuál falló, link al run): silencioso
si todo pasó, con sonido si algo falló.

Los secrets `TG_BOT_TOKEN` y `TG_TEST_CHAT` van en **Settings → Environments →
`telegram`** (no en los del repo). Sin ninguno, smoke y aviso se saltan (p.ej. PRs
de forks); con solo uno, el job falla para que no pase desapercibido.

Fuera de alcance en v1: webhooks, `getUpdates` daemon (salvo el helper interactivo `tg whoami`), callbacks, pagos, polls, stories, inline mode.

## Créditos

Creado y mantenido por **Marcelo Aguirre ([@xhelodev](https://github.com/xhelodev))**,
con revisión automática de PRs a cargo de un agente Hermes (`glm-5.3`).

## Licencia

[MIT](LICENSE) — Copyright © 2026 Marcelo Aguirre (xhelodev).
