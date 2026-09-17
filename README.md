# tg

CLI en Go para la [Telegram Bot API](https://core.telegram.org/bots/api) (objetivo: **Bot API 10.3**).

Dos artefactos:

1. **`pkg/telegram`** — cliente tipado, solo stdlib, sin CLI ni `os.Exit`.
2. **`cmd/tg`** — wrapper delgado sobre la librería.

Cero dependencias externas. Binario estático (`CGO_ENABLED=0`).

## Instalación

```bash
git clone https://github.com/xhelodev/tg.git
cd tg
make install          # → $(go env GOPATH)/bin/tg
# o
make build && ./bin/tg version
```

Requiere Go ≥ 1.22.

## Crear el bot (@BotFather)

1. Abre Telegram y habla con [@BotFather](https://t.me/BotFather).
2. `/newbot` → elige nombre y username.
3. Copia el token (`123456789:AAH...`). **Nunca lo subas a git ni lo pases por `--token` en scripts compartidos.**

```bash
# Recomendado: tg login (interactivo; no abre el navegador)
tg login
# o no interactivo
tg login --token-file /ruta/segura/token

# Alternativas
export TG_BOT_TOKEN='123456789:AAH...'
# o
echo '123456789:AAH...' > ~/.config/tg/token && chmod 600 ~/.config/tg/token
tg me --token-file ~/.config/tg/token
```

`tg login` valida el token con `getMe`, lo guarda en el perfil activo de
`~/.config/tg/config.json` (`$XDG_CONFIG_HOME`) con permisos `0600`, e imprime
el username/id del bot (nunca el token). La entrada interactiva puede hacer eco;
usa `--token-file` si prefieres no mostrarlo.

## Cómo obtener el `chat_id`

- **DM contigo:** mándale un mensaje al bot, luego:
  ```bash
  curl -s "https://api.telegram.org/bot$TG_BOT_TOKEN/getUpdates" | python3 -m json.tool
  ```
  Busca `message.chat.id` (número positivo).
- **Grupo/canal:** añade el bot, escribe algo (o reenvía un mensaje del grupo a [@userinfobot](https://t.me/userinfobot)), o lee `getUpdates`. Los supergrupos/canales usan IDs negativos tipo `-100…`.
- **Pitfall:** `@username` **no** resuelve usuarios privados. Aunque el bot tenga un DM abierto, hace falta el ID numérico. `@canal_publico` sí funciona para canales/supergrupos públicos.

## Autenticación (precedencia)

1. `--token-file <ruta>`
2. `TG_BOT_TOKEN`
3. `TELEGRAM_BOT_TOKEN` (alias Hermes)
4. Perfil en `~/.config/tg/config.json` (`$XDG_CONFIG_HOME`)

Sin token → **exit 3**, listando los orígenes intentados.

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

## Ejemplos

```bash
tg login
tg me
tg send --chat 589215 "hola"
echo hola | tg send 589215
tg send 589215 -
tg send --to @mi_canal --parse-mode HTML "<b>alerta</b>"
tg send --chat -100123 --thread 42 "en el tópico"
tg send --photo ./foto.jpg --chat 589215 --caption "cap"
tg send --split --chat 589215 "$(python3 -c 'print("x"*5000)')"

tg edit --chat 589215 --message-id 42 "texto nuevo"
tg edit-caption --chat 589215 --message-id 42 --caption "nuevo"
tg delete --chat 589215 --message-id 42
tg pin --chat 589215 --message-id 42
tg unpin --chat 589215 --message-id 42
tg react --chat 589215 --message-id 42 --emoji "👍"
tg forward --chat 589215 --from 589215 --message-id 42
tg copy --chat 589215 --from 589215 --message-id 42
tg action --chat 589215 --action typing
tg chat --chat @mi_canal
tg member --chat @mi_canal
tg version
```

Flags útiles: `--json`, `--quiet`/`-q`, `--dry-run`, `--verbose`, `--silent`/`-s`,
`--protect`, `--reply-to`, `--no-preview`, `--api-base`, `--timeout`, `--profile`, `--no-retry`.

## Exit codes

| code | significado |
|------|-------------|
| 0 | éxito |
| 1 | error inesperado / bug |
| 2 | uso (flag/argumento) |
| 3 | config o token ausente/inválido |
| 4 | API `ok:false` |
| 5 | red / DNS / timeout |

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
2. Con `parse_mode=HTML` hay que escapar `&`, `<`, `>` (el CLI escapa por defecto el texto de stdin).
3. Sin `--thread` en un foro, el mensaje cae en «General».
4. En DM el bot solo borra **sus** mensajes.
5. `deleteMessage` necesita el `message_id` exacto; no hay listado histórico.
6. `--split` numera partes `[i/N]` y evita cortar etiquetas HTML abiertas.

## Bot API server local

```bash
tg --api-base http://localhost:8081 me
tg --api-base http://localhost:8081 send --photo ./video-grande.mp4 --chat 589215
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
make smoke   # requiere TG_BOT_TOKEN y TG_TEST_CHAT
```

Fuera de alcance en v1: webhooks, `getUpdates` daemon, callbacks, pagos, polls, stories, inline mode.
