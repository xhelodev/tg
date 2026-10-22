package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xhelodev/tg/internal/config"
	"github.com/xhelodev/tg/pkg/telegram"
)

// VersionInfo se inyecta desde main vía ldflags.
type VersionInfo struct {
	Version string
	Commit  string
}

// GlobalFlags flags comunes.
type GlobalFlags struct {
	Token       string
	TokenFile   string
	Profile     string
	APIBase     string
	Timeout     time.Duration
	JSON        bool
	Quiet       bool
	DryRun      bool
	NoRetry     bool
	Verbose     bool
	Silent      bool
	Protect     bool
	ReplyTo     int
	NoPreview   bool
	Chat        string
	Thread      int
	ParseMode   string
	Split       bool
	HTMLEscape  bool
	TextFile    string
	Caption     string
	CaptionMode string
}

// Run es el punto de entrada del CLI. Devuelve exit code.
func Run(args []string, ver VersionInfo) int {
	if len(args) == 0 {
		printUsage()
		return ExitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help", "-h", "--help":
		printUsage()
		return ExitOK
	case "version":
		return cmdVersion(rest, ver)
	case "login":
		return cmdLogin(ctx, rest)
	case "me", "send", "edit", "edit-caption", "delete", "pin", "unpin",
		"react", "forward", "copy", "action", "chat", "member":
		return dispatch(ctx, cmd, rest, ver)
	default:
		PrintErr("comando desconocido: %s", cmd)
		printUsage()
		return ExitUsage
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `tg — CLI para la Telegram Bot API

Uso:
  tg <comando> [flags] [args]

Comandos:
  login           guarda el token del bot en la config (BotFather)
  me              getMe — valida token y muestra bot
  send            sendMessage / sendPhoto / … 
  edit            editMessageText
  edit-caption    editMessageCaption
  delete          deleteMessage
  pin / unpin     pinChatMessage / unpinChatMessage
  react           setMessageReaction
  forward         forwardMessage
  copy            copyMessage
  action          sendChatAction
  chat            getChat
  member          getChatMember
  version         versión del binario y Bot API objetivo

Flags globales:
  --token-file, --token, --profile, --api-base, --timeout
  --json, --quiet/-q, --dry-run, --no-retry, --verbose
  --silent/-s, --protect, --reply-to, --no-preview
  --chat/--to, --thread/--topic, --parse-mode, --split
  --html-escape, --text-file, --caption, --caption-mode

Auth (precedencia): --token-file > --token > TG_BOT_TOKEN > TELEGRAM_BOT_TOKEN > config
Texto: sin --html-escape el texto se envía tal cual (stdin incluido);
con --parse-mode HTML y texto plano use --html-escape para escapar &<>.

Exit codes: 0 ok | 1 bug | 2 uso | 3 config/token | 4 API | 5 red | 130 interrumpido (Ctrl-C)
`)
}

func parseGlobal(fs *flag.FlagSet, g *GlobalFlags) {
	fs.StringVar(&g.Token, "token", "", "token del bot (no recomendado; queda en historial)")
	fs.StringVar(&g.TokenFile, "token-file", "", "archivo con el token")
	fs.StringVar(&g.Profile, "profile", "", "perfil de configuración")
	fs.StringVar(&g.APIBase, "api-base", telegram.DefaultBaseURL, "URL base de la Bot API")
	fs.DurationVar(&g.Timeout, "timeout", 30*time.Second, "timeout HTTP")
	fs.BoolVar(&g.JSON, "json", false, "salida JSON cruda")
	fs.BoolVar(&g.Quiet, "quiet", false, "solo message_id")
	fs.BoolVar(&g.Quiet, "q", false, "alias de --quiet")
	fs.BoolVar(&g.DryRun, "dry-run", false, "mostrar payload sin enviar")
	fs.BoolVar(&g.NoRetry, "no-retry", false, "desactivar reintentos 429")
	fs.BoolVar(&g.Verbose, "verbose", false, "log de petición (token redactado)")
	fs.BoolVar(&g.Silent, "silent", false, "disable_notification")
	fs.BoolVar(&g.Silent, "s", false, "alias de --silent")
	fs.BoolVar(&g.Protect, "protect", false, "protect_content")
	fs.IntVar(&g.ReplyTo, "reply-to", 0, "reply_to_message_id")
	fs.BoolVar(&g.NoPreview, "no-preview", false, "desactivar vista previa de enlaces")
	fs.StringVar(&g.Chat, "chat", "", "chat_id, @username o alias")
	fs.StringVar(&g.Chat, "to", "", "alias de --chat")
	fs.IntVar(&g.Thread, "thread", 0, "message_thread_id")
	fs.IntVar(&g.Thread, "topic", 0, "alias de --thread")
	fs.StringVar(&g.ParseMode, "parse-mode", "HTML", "HTML|MarkdownV2|Markdown")
	fs.BoolVar(&g.Split, "split", false, "partir textos >4096")
	fs.BoolVar(&g.HTMLEscape, "html-escape", false, "escapar &<> del texto (nunca se aplica por defecto, ni con stdin)")
	fs.StringVar(&g.TextFile, "text-file", "", "leer texto desde archivo")
	fs.StringVar(&g.Caption, "caption", "", "caption para media")
	fs.StringVar(&g.CaptionMode, "caption-mode", "", "parse_mode del caption")
}

func dispatch(ctx context.Context, cmd string, args []string, ver VersionInfo) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var g GlobalFlags
	parseGlobal(fs, &g)

	// Flags específicos de send
	var photo, video, audio, voice, animation, document, sticker string
	if cmd == "send" {
		fs.StringVar(&photo, "photo", "", "ruta/URL/file_id de foto")
		fs.StringVar(&video, "video", "", "video")
		fs.StringVar(&audio, "audio", "", "audio")
		fs.StringVar(&voice, "voice", "", "voice")
		fs.StringVar(&animation, "animation", "", "animation")
		fs.StringVar(&document, "document", "", "document")
		fs.StringVar(&sticker, "sticker", "", "sticker")
	}
	var emoji string
	var big bool
	if cmd == "react" {
		fs.StringVar(&emoji, "emoji", "", "emoji de reacción")
		fs.BoolVar(&big, "big", false, "reacción grande")
	}
	var fromChat string
	var messageID int
	if cmd == "forward" || cmd == "copy" || cmd == "edit" || cmd == "edit-caption" ||
		cmd == "delete" || cmd == "pin" || cmd == "unpin" || cmd == "react" {
		fs.StringVar(&fromChat, "from", "", "from_chat_id (forward/copy)")
		fs.IntVar(&messageID, "message-id", 0, "message_id")
		fs.IntVar(&messageID, "msgid", 0, "alias de --message-id")
	}
	var action string
	if cmd == "action" {
		fs.StringVar(&action, "action", "typing", "typing|upload_photo|…")
	}
	var userID int64
	if cmd == "member" {
		fs.Int64Var(&userID, "user-id", 0, "user_id (default: el bot)")
	}

	if err := fs.Parse(args); err != nil {
		PrintErr("flags: %v", err)
		return ExitUsage
	}
	pos := fs.Args()

	// El aviso de permisos del config es ruido en pipelines silenciosas
	config.WarnOnPermissions = !(g.Quiet || g.JSON)

	cfg, _, err := config.Load("")
	if err != nil {
		PrintErr("config: %v", err)
		return ExitConfig
	}

	ts, err := ResolveToken(g.Token, g.TokenFile, g.Profile, cfg)
	if err != nil {
		pr := Printer{JSON: g.JSON}
		pr.ErrorJSON(ExitConfig, err.Error())
		if !g.JSON {
			PrintErr("%v", err)
		}
		return ExitConfig
	}

	// Nunca filtrar el token a logs
	opts := []telegram.Option{
		telegram.WithBaseURL(g.APIBase),
		telegram.WithTimeout(g.Timeout),
	}
	if g.NoRetry {
		opts = append(opts, telegram.WithNoRetry())
	}
	if g.Verbose {
		opts = append(opts, telegram.WithVerbose(func(method, url string, status int, d time.Duration) {
			PrintErr("verbose %s %s status=%d dur=%s", method, url, status, d)
		}))
	}
	// Retry 429 aviso a stderr
	opts = append(opts, telegram.WithRetrySleep(func(ctx context.Context, d time.Duration) error {
		PrintErr("aviso: 429 retry_after, esperando %s", d.Round(time.Millisecond))
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}))

	client, err := telegram.New(ts.Token, opts...)
	if err != nil {
		PrintErr("%v", err)
		return ExitConfig
	}

	pr := Printer{JSON: g.JSON, Quiet: g.Quiet}
	prof, _, _ := cfg.ResolveProfile(g.Profile)

	switch cmd {
	case "me":
		return runMe(ctx, client, pr, g)
	case "send":
		media := mediaFromFlags(photo, video, audio, voice, animation, document, sticker)
		return runSend(ctx, client, pr, g, prof, pos, media)
	case "edit":
		return runEdit(ctx, client, pr, g, prof, pos, messageID)
	case "edit-caption":
		return runEditCaption(ctx, client, pr, g, prof, pos, messageID)
	case "delete":
		return runDelete(ctx, client, pr, g, prof, pos, messageID)
	case "pin":
		return runPin(ctx, client, pr, g, prof, messageID, true)
	case "unpin":
		return runPin(ctx, client, pr, g, prof, messageID, false)
	case "react":
		return runReact(ctx, client, pr, g, prof, messageID, emoji, big)
	case "forward":
		return runForward(ctx, client, pr, g, prof, fromChat, messageID, false)
	case "copy":
		return runForward(ctx, client, pr, g, prof, fromChat, messageID, true)
	case "action":
		return runAction(ctx, client, pr, g, prof, action)
	case "chat":
		return runChat(ctx, client, pr, g, prof, pos)
	case "member":
		return runMember(ctx, client, pr, g, prof, userID)
	default:
		return ExitUsage
	}
}

func cmdVersion(args []string, ver VersionInfo) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "")
	_ = fs.Parse(args)
	v := ver.Version
	if v == "" {
		v = "dev"
	}
	c := ver.Commit
	if c == "" {
		c = "unknown"
	}
	if jsonOut {
		fmt.Printf(`{"version":%q,"commit":%q,"bot_api":%q}`+"\n", v, c, telegram.BotAPIVersion)
		return ExitOK
	}
	fmt.Printf("tg %s (commit %s) bot_api=%s\n", v, c, telegram.BotAPIVersion)
	return ExitOK
}

func mapError(err error) int {
	if err == nil {
		return ExitOK
	}
	var ve *telegram.ValidationError
	if errors.As(err, &ve) {
		return ExitUsage
	}
	var ae *telegram.APIError
	if errors.As(err, &ae) {
		return ExitAPI
	}
	var ne *telegram.NetworkError
	if errors.As(err, &ne) {
		return ExitNetwork
	}
	if errors.Is(err, context.Canceled) {
		// Ctrl-C del usuario: no es un fallo de red.
		return ExitInterrupted
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ExitNetwork
	}
	return ExitBug
}

func resolveChat(g *GlobalFlags, prof config.Profile, pos []string) (chatID string, threadID int, consumed bool, err error) {
	raw := g.Chat
	if raw == "" && len(pos) > 0 {
		// El primer posicional es el chat solo si parece ID/@username o es
		// un alias conocido; una palabra suelta es texto, no destino (bug:
		// antes cualquier palabra <64 chars se tragaba como chat).
		cand := pos[0]
		if looksLikeChat(cand) || prof.HasAlias(cand) {
			raw = cand
			consumed = true
		}
	}
	if raw == "" && prof.DefaultChat != "" {
		raw = prof.DefaultChat
	}
	if raw == "" {
		return "", 0, false, fmt.Errorf("indique --chat / --to o un destino posicional")
	}

	// Alias
	if aChat, aTh, ok := prof.ResolveAlias(raw); ok {
		chatID, threadID = aChat, aTh
	} else {
		chatID, threadID = config.SplitChatThread(raw)
	}
	if g.Thread > 0 {
		threadID = g.Thread
	}
	return chatID, threadID, consumed, nil
}

// looksLikeChat solo acepta formas inequívocas de destino: @username,
// ID numérico (opcionalmente negativo) e "id:thread" — sin espacios en
// ningún caso ("2026: balance" es texto, no chat:thread). Las palabras
// sueltas NO son chats: sin alias ni default_chat se devuelve error de uso.
func looksLikeChat(s string) bool {
	if s == "" || s == "-" || strings.ContainsFunc(s, func(r rune) bool {
		return r == ' ' || r == '	' || r == '\n'
	}) {
		return false
	}
	if strings.HasPrefix(s, "@") {
		return true
	}
	body := s
	if i := strings.LastIndex(s, ":"); i > 0 {
		body = s[:i]
	}
	if _, err := strconv.ParseInt(body, 10, 64); err == nil {
		return true
	}
	return false
}

type mediaFlag struct {
	Kind telegram.MediaKind
	Ref  string
}

func mediaFromFlags(photo, video, audio, voice, animation, document, sticker string) *mediaFlag {
	pairs := []struct {
		k telegram.MediaKind
		v string
	}{
		{telegram.MediaPhoto, photo},
		{telegram.MediaVideo, video},
		{telegram.MediaAudio, audio},
		{telegram.MediaVoice, voice},
		{telegram.MediaAnimation, animation},
		{telegram.MediaDocument, document},
		{telegram.MediaSticker, sticker},
	}
	for _, p := range pairs {
		if p.v != "" {
			return &mediaFlag{Kind: p.k, Ref: p.v}
		}
	}
	return nil
}

// Limitaciones conocidas: rutas Windows absolutas ("C:\\foto.jpg") y
// "~usuario/x" no se detectan como ruta y acabarían tratadas como file_id.
func classifyMediaRef(ref string) (fileID, url, path string, err error) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return "", ref, "", nil
	}
	// Referencias con forma de ruta (./x, /abs, ~/x, cualquier "/") que no
	// existen: error local claro en vez de enviarlas a Telegram como file_id
	// y recibir un 400 críptico.
	pathLike := strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") ||
		strings.HasPrefix(ref, "~") || strings.Contains(ref, "/")
	if pathLike {
		p := ref
		if strings.HasPrefix(p, "~") {
			if home, e := os.UserHomeDir(); e == nil {
				p = filepath.Join(home, strings.TrimPrefix(p, "~"))
			}
		}
		if _, e := os.Stat(p); e != nil {
			return "", "", "", fmt.Errorf("archivo no encontrado: %s", ref)
		}
		return "", "", p, nil
	}
	if _, e := os.Stat(ref); e == nil {
		return "", "", ref, nil
	}
	// file_id de Telegram
	return ref, "", "", nil
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func readText(g *GlobalFlags, pos []string, chatConsumed bool) (text string, fromStdin bool, err error) {
	if g.TextFile != "" {
		b, err := os.ReadFile(g.TextFile)
		if err != nil {
			return "", false, err
		}
		return string(b), false, nil
	}
	args := pos
	// Si el chat consumió el primer posicional, el resto es texto.
	// No se re-verifica con looksLikeChat: resolveChat ya lo decidió
	// (incluye alias, que no parecen chat a ojos de la heurística).
	if chatConsumed && len(args) > 0 {
		args = args[1:]
	}
	if len(args) == 0 || (len(args) == 1 && args[0] == "-") {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", true, err
		}
		return string(b), true, nil
	}
	// Soporte -- ya consumido por flag; unir resto
	return strings.Join(args, " "), false, nil
}
