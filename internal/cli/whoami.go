package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xhelodev/tg/pkg/telegram"
)

// Inyectables en tests.
var (
	whoamiBackoff     = time.Second
	whoamiMaxNetFails = 5
)

var whoamiAllowed = []string{"message", "edited_message", "channel_post"}

// whoamiInfo es lo que se imprime/responde por cada mensaje recibido.
type whoamiInfo struct {
	ChatID      int64  `json:"chat_id"`
	ChatType    string `json:"chat_type"`
	ChatTitle   string `json:"chat_title,omitempty"`
	ThreadID    int    `json:"thread_id,omitempty"`
	UserID      int64  `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	Name        string `json:"name,omitempty"`
	SenderChat  int64  `json:"sender_chat_id,omitempty"`
	FwdUserID   int64  `json:"forward_user_id,omitempty"`
	FwdUsername string `json:"forward_username,omitempty"`
	FwdName     string `json:"forward_name,omitempty"`
	FwdChatID   int64  `json:"forward_chat_id,omitempty"`
	FwdChat     string `json:"forward_chat_title,omitempty"`
}

// Target es el valor a usar en --chat (id o id:thread).
func (w whoamiInfo) Target() string {
	if w.ThreadID > 0 {
		return fmt.Sprintf("%d:%d", w.ChatID, w.ThreadID)
	}
	return strconv.FormatInt(w.ChatID, 10)
}

func newWhoamiInfo(m *telegram.Message) whoamiInfo {
	w := whoamiInfo{
		ChatID:    m.Chat.ID,
		ChatType:  m.Chat.Type,
		ChatTitle: chatTitle(&m.Chat),
		ThreadID:  m.MessageThreadID,
	}
	if m.From != nil {
		w.UserID, w.Username, w.Name = m.From.ID, m.From.Username, fullName(m.From)
	}
	if m.SenderChat != nil {
		w.SenderChat = m.SenderChat.ID
	}
	if o := m.ForwardOrigin; o != nil {
		switch {
		case o.SenderUser != nil:
			w.FwdUserID, w.FwdUsername, w.FwdName = o.SenderUser.ID, o.SenderUser.Username, fullName(o.SenderUser)
		case o.SenderUserName != "":
			w.FwdName = o.SenderUserName
		case o.SenderChat != nil:
			w.FwdChatID, w.FwdChat = o.SenderChat.ID, chatTitle(o.SenderChat)
		case o.Chat != nil:
			w.FwdChatID, w.FwdChat = o.Chat.ID, chatTitle(o.Chat)
		}
	}
	return w
}

func fullName(u *telegram.User) string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

func chatTitle(c *telegram.Chat) string {
	if c.Title != "" {
		return c.Title
	}
	return strings.TrimSpace(c.FirstName + " " + c.LastName)
}

func userLabel(id int64, username, name string) string {
	s := strconv.FormatInt(id, 10)
	var extra []string
	if username != "" {
		extra = append(extra, "@"+username)
	}
	if name != "" {
		extra = append(extra, name)
	}
	if len(extra) > 0 {
		s += " (" + strings.Join(extra, ", ") + ")"
	}
	return s
}

// ReplyText es la respuesta en texto plano que el bot envía al chat.
func (w whoamiInfo) ReplyText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "chat_id: %d\n", w.ChatID)
	if w.ChatTitle != "" {
		fmt.Fprintf(&b, "tipo: %s (%s)\n", w.ChatType, w.ChatTitle)
	} else {
		fmt.Fprintf(&b, "tipo: %s\n", w.ChatType)
	}
	if w.ThreadID > 0 {
		fmt.Fprintf(&b, "thread_id: %d\n", w.ThreadID)
	}
	if w.UserID != 0 {
		fmt.Fprintf(&b, "user_id: %s\n", userLabel(w.UserID, w.Username, w.Name))
	}
	if w.SenderChat != 0 && w.SenderChat != w.ChatID {
		fmt.Fprintf(&b, "sender_chat_id: %d\n", w.SenderChat)
	}
	switch {
	case w.FwdUserID != 0:
		fmt.Fprintf(&b, "reenviado de user_id: %s\n", userLabel(w.FwdUserID, w.FwdUsername, w.FwdName))
	case w.FwdChatID != 0:
		fmt.Fprintf(&b, "reenviado de chat_id: %d (%s)\n", w.FwdChatID, w.FwdChat)
	case w.FwdName != "":
		fmt.Fprintf(&b, "reenviado de: %s (id oculto por su privacidad)\n", w.FwdName)
	}
	fmt.Fprintf(&b, "\ntg send --chat %s \"…\"", w.Target())
	return b.String()
}

// Line es la línea resumen para stdout.
func (w whoamiInfo) Line() string {
	s := fmt.Sprintf("chat_id=%d type=%s", w.ChatID, w.ChatType)
	if w.ThreadID > 0 {
		s += fmt.Sprintf(" thread=%d", w.ThreadID)
	}
	if w.ChatTitle != "" {
		s += fmt.Sprintf(" title=%q", w.ChatTitle)
	}
	if w.UserID != 0 {
		s += fmt.Sprintf(" user_id=%d", w.UserID)
		if w.Username != "" {
			s += " @" + w.Username
		}
	}
	if w.FwdUserID != 0 {
		s += fmt.Sprintf(" fwd_user_id=%d", w.FwdUserID)
	} else if w.FwdChatID != 0 {
		s += fmt.Sprintf(" fwd_chat_id=%d", w.FwdChatID)
	}
	return s
}

// runWhoami escucha con getUpdates y responde a cada mensaje con los IDs
// del chat y del remitente. Los mensajes pendientes de antes de arrancar
// se descartan (se avisa en stderr) para no responder a mensajes viejos.
func runWhoami(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, once, noReply bool) int {
	if g.DryRun {
		PrintErr("dry-run POST %s allowed_updates=%s", c.BaseURL()+"/bot"+c.TokenRedacted()+"/getUpdates", strings.Join(whoamiAllowed, ","))
		return ExitOK
	}
	chatty := !g.Quiet && !g.JSON

	me, err := c.GetMe(ctx)
	if err != nil {
		return fail(pr, g, err)
	}

	// Vaciar pendientes (timeout 0): todo lo que llegue aquí es anterior al arranque.
	offset, code := whoamiDrain(ctx, c, pr, g, chatty)
	if code != ExitOK {
		return code
	}

	if chatty {
		PrintErr("escuchando como @%s — escríbele al bot (en grupos: /whoami o mencionándolo). Ctrl-C para salir.", me.Username)
	}

	// Long polling por debajo del timeout HTTP.
	poll := int((g.Timeout - 5*time.Second) / time.Second)
	if poll < 0 {
		poll = 0
	}
	netFails := 0
	for {
		ups, err := c.GetUpdates(ctx, telegram.GetUpdatesRequest{
			Offset: offset, Timeout: poll, AllowedUpdates: whoamiAllowed,
		})
		if err != nil {
			if ctx.Err() != nil {
				return ExitInterrupted
			}
			var ne *telegram.NetworkError
			if errors.As(err, &ne) && netFails < whoamiMaxNetFails {
				netFails++
				wait := whoamiBackoff << (netFails - 1)
				PrintErr("aviso: %v; reintentando en %s", err, wait)
				if sleepCtx(ctx, wait) != nil {
					return ExitInterrupted
				}
				continue
			}
			return whoamiFail(pr, g, err)
		}
		netFails = 0
		if len(ups) == 0 && poll == 0 {
			if sleepCtx(ctx, time.Second) != nil {
				return ExitInterrupted
			}
		}
		for _, u := range ups {
			offset = u.UpdateID + 1
			m := u.AnyMessage()
			if m == nil {
				continue
			}
			info := newWhoamiInfo(m)
			switch {
			case g.JSON:
				_ = json.NewEncoder(os.Stdout).Encode(info)
			case g.Quiet:
				fmt.Println(info.Target())
			default:
				fmt.Println(info.Line())
			}
			if !noReply {
				_, err := c.SendMessage(ctx, telegram.SendMessageRequest{
					ChatID:           strconv.FormatInt(m.Chat.ID, 10),
					Text:             info.ReplyText(),
					ReplyToMessageID: telegram.IntPtr(m.MessageID),
				})
				if err != nil && ctx.Err() == nil {
					PrintErr("aviso: no se pudo responder en %d: %v", m.Chat.ID, err)
				}
			}
			if once {
				// Confirmar el update antes de salir para no re-procesarlo.
				_, _ = c.GetUpdates(ctx, telegram.GetUpdatesRequest{Offset: offset, AllowedUpdates: whoamiAllowed})
				return ExitOK
			}
		}
	}
}

func whoamiDrain(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, chatty bool) (int64, int) {
	var offset int64
	skipped := 0
	for {
		ups, err := c.GetUpdates(ctx, telegram.GetUpdatesRequest{Offset: offset, AllowedUpdates: whoamiAllowed})
		if err != nil {
			return 0, whoamiFail(pr, g, err)
		}
		if len(ups) == 0 {
			break
		}
		for _, u := range ups {
			offset = u.UpdateID + 1
			if m := u.AnyMessage(); m != nil {
				skipped++
				if chatty {
					PrintErr("pendiente (sin responder): %s", newWhoamiInfo(m).Line())
				}
			}
		}
	}
	if skipped > 0 && chatty {
		PrintErr("%d mensaje(s) pendiente(s) descartado(s)", skipped)
	}
	return offset, ExitOK
}

func whoamiFail(pr Printer, g GlobalFlags, err error) int {
	var ae *telegram.APIError
	if errors.As(err, &ae) && ae.Code == 409 {
		err = fmt.Errorf("%w; whoami usa getUpdates: no funciona si el bot tiene webhook "+
			"o si otro proceso (p.ej. otro bot con el mismo token) está leyendo updates", err)
	}
	return fail(pr, g, err)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
