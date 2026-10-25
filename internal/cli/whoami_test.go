package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBotAPI simula getMe/getUpdates/sendMessage. backlog se devuelve en
// la primera llamada (drain); live, una tanda por cada long poll.
type fakeBotAPI struct {
	mu       sync.Mutex
	backlog  []map[string]any
	live     [][]map[string]any
	fail502  int // long polls que devuelven 502 antes de live
	conflict bool
	offsets  []float64
	sent     []map[string]any
}

func (f *fakeBotAPI) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		ok := func(res any) { _ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": res}) }
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			ok(map[string]any{"id": 99, "is_bot": true, "first_name": "Bot", "username": "mybot"})
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			if f.conflict {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": 409,
					"description": "Conflict: can't use getUpdates method while webhook is active"})
				return
			}
			off, _ := body["offset"].(float64)
			f.offsets = append(f.offsets, off)
			timeout, _ := body["timeout"].(float64)
			if timeout == 0 {
				// drain o confirmación final
				if f.backlog != nil {
					ok(f.backlog)
					f.backlog = nil
					return
				}
				ok([]any{})
				return
			}
			if f.fail502 > 0 {
				f.fail502--
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("bad gateway"))
				return
			}
			if len(f.live) == 0 {
				t.Error("long poll sin más updates (el test no terminó)")
				ok([]any{})
				return
			}
			batch := f.live[0]
			f.live = f.live[1:]
			ok(batch)
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			f.sent = append(f.sent, body)
			ok(map[string]any{"message_id": 1000, "date": 1, "chat": map[string]any{"id": 1, "type": "private"}})
		default:
			t.Errorf("path inesperado %s", r.URL.Path)
		}
	}
}

func upd(id int, msg map[string]any) map[string]any {
	return map[string]any{"update_id": id, "message": msg}
}

var (
	oldDM = map[string]any{
		"message_id": 1, "date": 1, "text": "viejo",
		"chat": map[string]any{"id": 111, "type": "private", "first_name": "Old"},
		"from": map[string]any{"id": 111, "is_bot": false, "first_name": "Old"},
	}
	groupFwd = map[string]any{
		"message_id": 7, "date": 2, "text": "hola", "message_thread_id": 42,
		"chat": map[string]any{"id": -1001234567890, "type": "supergroup", "title": "Equipo"},
		"from": map[string]any{"id": 589215, "is_bot": false, "first_name": "Marcelo", "last_name": "A", "username": "marce"},
		"forward_origin": map[string]any{"type": "user", "date": 1,
			"sender_user": map[string]any{"id": 777, "is_bot": false, "first_name": "Ana", "username": "ana"}},
	}
	hiddenFwd = map[string]any{
		"message_id": 8, "date": 3, "text": "x",
		"chat":           map[string]any{"id": 589215, "type": "private", "first_name": "Marcelo"},
		"from":           map[string]any{"id": 589215, "is_bot": false, "first_name": "Marcelo"},
		"forward_origin": map[string]any{"type": "hidden_user", "date": 1, "sender_user_name": "Fulano"},
	}
)

func startFake(t *testing.T, f *fakeBotAPI) string {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("TG_BOT_TOKEN", testToken)
	return srv.URL
}

func TestWhoami_OnceRepliesWithIDs(t *testing.T) {
	f := &fakeBotAPI{
		backlog: []map[string]any{upd(1, oldDM)},
		live:    [][]map[string]any{{upd(2, groupFwd)}},
	}
	url := startFake(t, f)
	code, out, errOut := captureStd(t, func() int {
		return Run([]string{"whoami", "--api-base", url, "--once"}, VersionInfo{})
	})
	if code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	if !strings.Contains(out, "chat_id=-1001234567890 type=supergroup thread=42") ||
		!strings.Contains(out, "user_id=589215 @marce") || !strings.Contains(out, "fwd_user_id=777") {
		t.Fatalf("stdout=%q", out)
	}
	if strings.Contains(out, "111") {
		t.Fatalf("el backlog no debe ir a stdout: %q", out)
	}
	if !strings.Contains(errOut, "pendiente (sin responder): chat_id=111") || !strings.Contains(errOut, "@mybot") {
		t.Fatalf("stderr=%q", errOut)
	}
	if strings.Contains(errOut, testToken) || strings.Contains(out, testToken) {
		t.Fatal("token filtrado")
	}
	if len(f.sent) != 1 {
		t.Fatalf("esperada 1 respuesta (no al backlog), got %d", len(f.sent))
	}
	s := f.sent[0]
	if s["chat_id"] != "-1001234567890" || s["reply_to_message_id"] != float64(7) {
		t.Fatalf("sendMessage=%v", s)
	}
	text, _ := s["text"].(string)
	for _, want := range []string{
		"chat_id: -1001234567890", "tipo: supergroup (Equipo)", "thread_id: 42",
		"user_id: 589215 (@marce, Marcelo A)", "reenviado de user_id: 777 (@ana, Ana)",
		`tg send --chat -1001234567890:42`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("falta %q en respuesta:\n%s", want, text)
		}
	}
	if _, has := s["parse_mode"]; has {
		t.Fatalf("la respuesta debe ir en texto plano: %v", s)
	}
	// drain(0) → drain(2) → poll(2) → confirmación final(3)
	if got := f.offsets; len(got) != 4 || got[0] != 0 || got[1] != 2 || got[2] != 2 || got[3] != 3 {
		t.Fatalf("offsets=%v", got)
	}
}

func TestWhoami_QuietJSONNoReply(t *testing.T) {
	f := &fakeBotAPI{live: [][]map[string]any{{upd(5, groupFwd)}}}
	url := startFake(t, f)
	code, out, errOut := captureStd(t, func() int {
		return Run([]string{"whoami", "--api-base", url, "--once", "-q", "--no-reply"}, VersionInfo{})
	})
	if code != ExitOK || strings.TrimSpace(out) != "-1001234567890:42" || errOut != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if len(f.sent) != 0 {
		t.Fatal("--no-reply no debe enviar")
	}

	f = &fakeBotAPI{live: [][]map[string]any{{upd(6, hiddenFwd)}}}
	url = startFake(t, f)
	code, out, _ = captureStd(t, func() int {
		return Run([]string{"whoami", "--api-base", url, "--once", "--json"}, VersionInfo{})
	})
	var got map[string]any
	if code != ExitOK || json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if got["chat_id"] != float64(589215) || got["user_id"] != float64(589215) || got["forward_name"] != "Fulano" {
		t.Fatalf("json=%v", got)
	}
	if text, _ := f.sent[0]["text"].(string); !strings.Contains(text, "reenviado de: Fulano (id oculto") {
		t.Fatalf("respuesta=%q", text)
	}
}

func TestWhoami_RetriesNetworkErrors(t *testing.T) {
	old := whoamiBackoff
	whoamiBackoff = time.Millisecond
	t.Cleanup(func() { whoamiBackoff = old })
	f := &fakeBotAPI{fail502: 2, live: [][]map[string]any{{upd(1, groupFwd)}}}
	url := startFake(t, f)
	code, _, errOut := captureStd(t, func() int {
		return Run([]string{"whoami", "--api-base", url, "--once", "--no-retry"}, VersionInfo{})
	})
	if code != ExitOK || strings.Count(errOut, "reintentando") != 2 {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestWhoami_GivesUpAfterMaxNetworkErrors(t *testing.T) {
	old := whoamiBackoff
	whoamiBackoff = time.Millisecond
	t.Cleanup(func() { whoamiBackoff = old })
	f := &fakeBotAPI{fail502: whoamiMaxNetFails + 1}
	url := startFake(t, f)
	code, _, _ := captureStd(t, func() int {
		return Run([]string{"whoami", "--api-base", url, "--no-retry"}, VersionInfo{})
	})
	if code != ExitNetwork {
		t.Fatalf("code=%d", code)
	}
}

func TestWhoami_WebhookConflict(t *testing.T) {
	url := startFake(t, &fakeBotAPI{conflict: true})
	code, _, errOut := captureStd(t, func() int {
		return Run([]string{"whoami", "--api-base", url}, VersionInfo{})
	})
	if code != ExitAPI || !strings.Contains(errOut, "webhook") || !strings.Contains(errOut, "otro proceso") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestWhoami_DryRun(t *testing.T) {
	t.Setenv("TG_BOT_TOKEN", testToken)
	code, _, errOut := captureStd(t, func() int {
		return Run([]string{"whoami", "--dry-run"}, VersionInfo{})
	})
	if code != ExitOK || !strings.Contains(errOut, "getUpdates") || strings.Contains(errOut, testToken) {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}
