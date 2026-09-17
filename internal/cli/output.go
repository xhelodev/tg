package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/xhelodev/tg/pkg/telegram"
)

// Printer controla el formato de salida.
type Printer struct {
	JSON  bool
	Quiet bool
}

func (p Printer) Message(msg *telegram.Message) {
	if p.JSON {
		enc := json.NewEncoder(os.Stdout)
		_ = enc.Encode(msg)
		return
	}
	if p.Quiet {
		fmt.Println(msg.MessageID)
		return
	}
	chat := fmt.Sprintf("%d", msg.Chat.ID)
	if msg.Chat.Username != "" {
		chat = "@" + msg.Chat.Username
	}
	fmt.Printf("ok message_id=%d chat=%s\n", msg.MessageID, chat)
}

func (p Printer) MessageID(id int) {
	if p.JSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"message_id": id, "ok": true})
		return
	}
	if p.Quiet {
		fmt.Println(id)
		return
	}
	fmt.Printf("ok message_id=%d\n", id)
}

func (p Printer) OK(msg string) {
	if p.JSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "message": msg})
		return
	}
	if p.Quiet {
		return
	}
	fmt.Println(msg)
}

func (p Printer) User(u *telegram.User) {
	if p.JSON {
		_ = json.NewEncoder(os.Stdout).Encode(u)
		return
	}
	fmt.Printf("ok id=%d username=@%s name=%s\n", u.ID, u.Username, u.FirstName)
}

func (p Printer) Chat(c *telegram.Chat) {
	if p.JSON {
		_ = json.NewEncoder(os.Stdout).Encode(c)
		return
	}
	title := c.Title
	if title == "" {
		title = c.FirstName
	}
	uname := ""
	if c.Username != "" {
		uname = " @" + c.Username
	}
	fmt.Printf("ok id=%d type=%s title=%s%s\n", c.ID, c.Type, title, uname)
}

func (p Printer) Member(m *telegram.ChatMember) {
	if p.JSON {
		_ = json.NewEncoder(os.Stdout).Encode(m)
		return
	}
	canPost := m.CanPostMessages || m.Status == "creator" || m.Status == "administrator"
	fmt.Printf("ok status=%s user=%d can_post=%v\n", m.Status, m.User.ID, canPost)
}

func (p Printer) ErrorJSON(code int, msg string) {
	if p.JSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"ok": false, "error": msg, "exit_code": code,
		})
	}
}

// PrintErr escribe a stderr (nunca el token).
func PrintErr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
