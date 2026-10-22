package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unicode/utf8"

	"github.com/xhelodev/tg/internal/config"
	"github.com/xhelodev/tg/pkg/telegram"
)

func runMe(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags) int {
	if g.DryRun {
		PrintErr("dry-run GET %s", c.BaseURL()+"/bot"+c.TokenRedacted()+"/getMe")
		return ExitOK
	}
	u, err := c.GetMe(ctx)
	if err != nil {
		return fail(pr, g, err)
	}
	pr.User(u)
	return ExitOK
}

func runSend(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, pos []string, media *mediaFlag) int {
	chatID, threadID, chatConsumed, err := resolveChat(&g, prof, pos)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}

	if media != nil {
		return runSendMedia(ctx, c, pr, g, chatID, threadID, media)
	}

	text, _, err := readText(&g, pos, chatConsumed)
	if err != nil {
		PrintErr("texto: %v", err)
		return ExitUsage
	}
	text = trimRightNewline(text)
	if text == "" {
		PrintErr("texto vacío")
		return ExitUsage
	}

	// html-escape SOLO si se pide explícitamente (--html-escape): escapar
	// por defecto con stdin rompía HTML legítimo como "<b>alerta</b>".
	if g.HTMLEscape && g.ParseMode == "HTML" {
		text = escapeHTML(text)
	}

	var threadPtr *int
	if threadID > 0 {
		threadPtr = &threadID
	}

	parts := []string{text}
	if utf8.RuneCountInString(text) > telegram.MaxTextLength {
		if !g.Split {
			PrintErr("texto de %d caracteres supera %d; use --split", utf8.RuneCountInString(text), telegram.MaxTextLength)
			return ExitUsage
		}
		parts = SplitText(text, telegram.MaxTextLength)
	}

	if g.DryRun {
		var payloads []map[string]any
		for i, p := range parts {
			pl := map[string]any{
				"chat_id": chatID, "text": p, "parse_mode": g.ParseMode,
			}
			if threadPtr != nil {
				pl["message_thread_id"] = *threadPtr
			}
			payloads = append(payloads, pl)
			if !g.JSON {
				b, _ := json.MarshalIndent(pl, "", "  ")
				PrintErr("dry-run sendMessage [%d/%d] token=%s\n%s", i+1, len(parts), c.TokenRedacted(), b)
			}
		}
		if g.JSON {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"dry_run": true, "method": "sendMessage", "payload": payloads,
			})
		}
		return ExitOK
	}

	// Timeout mayor implícito no aplica a texto
	var last *telegram.Message
	for _, p := range parts {
		req := telegram.SendMessageRequest{
			ChatID:          chatID,
			Text:            p,
			ParseMode:       g.ParseMode,
			MessageThreadID: threadPtr,
		}
		if g.Silent {
			req.DisableNotification = telegram.BoolPtr(true)
		}
		if g.Protect {
			req.ProtectContent = telegram.BoolPtr(true)
		}
		if g.ReplyTo > 0 {
			req.ReplyToMessageID = telegram.IntPtr(g.ReplyTo)
		}
		if g.NoPreview {
			req.LinkPreviewOptions = &telegram.LinkPreviewOptions{IsDisabled: true}
		}
		msg, err := c.SendMessage(ctx, req)
		if err != nil {
			return fail(pr, g, err)
		}
		last = msg
		if !g.Quiet && !g.JSON && len(parts) > 1 {
			fmt.Printf("ok message_id=%d chat=%s part\n", msg.MessageID, chatID)
		}
	}
	if g.Quiet || g.JSON || len(parts) == 1 {
		pr.Message(last)
	}
	return ExitOK
}

func runSendMedia(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, chatID string, threadID int, media *mediaFlag) int {
	fileID, url, path, err := classifyMediaRef(media.Ref)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	var threadPtr *int
	if threadID > 0 {
		threadPtr = &threadID
	}
	caption := g.Caption
	capMode := g.CaptionMode
	if capMode == "" {
		capMode = g.ParseMode
	}

	req := telegram.SendMediaRequest{
		ChatID:          chatID,
		Kind:            media.Kind,
		FileID:          fileID,
		URL:             url,
		FilePath:        path,
		Caption:         caption,
		ParseMode:       capMode,
		MessageThreadID: threadPtr,
	}
	if g.Silent {
		req.DisableNotification = telegram.BoolPtr(true)
	}
	if g.Protect {
		req.ProtectContent = telegram.BoolPtr(true)
	}
	if g.ReplyTo > 0 {
		req.ReplyToMessageID = telegram.IntPtr(g.ReplyTo)
	}

	if g.DryRun {
		if g.JSON {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"dry_run": true, "method": "send" + media.Kind,
				"chat_id": chatID, "ref": media.Ref,
			})
			return ExitOK
		}
		PrintErr("dry-run send%s chat=%s ref=%s token=%s", media.Kind, chatID, media.Ref, c.TokenRedacted())
		return ExitOK
	}

	// Subida de archivo: ampliar timeout a 5m si el default es 30s
	if path != "" && g.Timeout <= 30*time.Second {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	msg, err := c.SendMedia(ctx, req)
	if err != nil {
		return fail(pr, g, err)
	}
	pr.Message(msg)
	return ExitOK
}

func runEdit(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, pos []string, messageID int) int {
	chatID, _, consumed, err := resolveChat(&g, prof, pos)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	if messageID == 0 {
		PrintErr("indique --message-id")
		return ExitUsage
	}
	text, _, err := readText(&g, pos, consumed)
	if err != nil || text == "" {
		PrintErr("texto requerido")
		return ExitUsage
	}
	if g.HTMLEscape && g.ParseMode == "HTML" {
		text = escapeHTML(text)
	}
	if g.DryRun {
		if g.JSON {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"dry_run": true, "method": "editMessageText",
				"chat_id": chatID, "message_id": messageID, "text": text,
			})
			return ExitOK
		}
		PrintErr("dry-run editMessageText chat=%s msgid=%d token=%s", chatID, messageID, c.TokenRedacted())
		return ExitOK
	}
	req := telegram.EditMessageTextRequest{
		ChatID: chatID, MessageID: messageID, Text: text, ParseMode: g.ParseMode,
	}
	if g.NoPreview {
		req.LinkPreviewOptions = &telegram.LinkPreviewOptions{IsDisabled: true}
	}
	msg, err := c.EditMessageText(ctx, req)
	if err != nil {
		return fail(pr, g, err)
	}
	pr.Message(msg)
	return ExitOK
}

func runEditCaption(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, pos []string, messageID int) int {
	chatID, _, consumed, err := resolveChat(&g, prof, pos)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	if messageID == 0 {
		PrintErr("indique --message-id")
		return ExitUsage
	}
	caption := g.Caption
	if caption == "" {
		caption, _, err = readText(&g, pos, consumed)
		if err != nil {
			PrintErr("%v", err)
			return ExitUsage
		}
	}
	mode := g.CaptionMode
	if mode == "" {
		mode = g.ParseMode
	}
	if g.DryRun {
		PrintErr("dry-run editMessageCaption chat=%s msgid=%d token=%s", chatID, messageID, c.TokenRedacted())
		return ExitOK
	}
	msg, err := c.EditMessageCaption(ctx, telegram.EditMessageCaptionRequest{
		ChatID: chatID, MessageID: messageID, Caption: caption, ParseMode: mode,
	})
	if err != nil {
		return fail(pr, g, err)
	}
	pr.Message(msg)
	return ExitOK
}

func runDelete(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, pos []string, messageID int) int {
	chatID, _, _, err := resolveChat(&g, prof, pos)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	if messageID == 0 {
		PrintErr("indique --message-id")
		return ExitUsage
	}
	if g.DryRun {
		PrintErr("dry-run deleteMessage chat=%s msgid=%d token=%s", chatID, messageID, c.TokenRedacted())
		return ExitOK
	}
	if err := c.DeleteMessage(ctx, chatID, messageID); err != nil {
		return fail(pr, g, err)
	}
	pr.OK(fmt.Sprintf("ok deleted message_id=%d", messageID))
	return ExitOK
}

func runPin(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, messageID int, pin bool) int {
	chatID, _, _, err := resolveChat(&g, prof, nil)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	if pin && messageID == 0 {
		PrintErr("indique --message-id")
		return ExitUsage
	}
	if g.DryRun {
		op := "unpinChatMessage"
		if pin {
			op = "pinChatMessage"
		}
		PrintErr("dry-run %s chat=%s msgid=%d token=%s", op, chatID, messageID, c.TokenRedacted())
		return ExitOK
	}
	if pin {
		req := telegram.PinChatMessageRequest{ChatID: chatID, MessageID: messageID}
		if g.Silent {
			req.DisableNotification = telegram.BoolPtr(true)
		}
		err = c.PinChatMessage(ctx, req)
	} else {
		err = c.UnpinChatMessage(ctx, chatID, messageID)
	}
	if err != nil {
		return fail(pr, g, err)
	}
	pr.OK("ok")
	return ExitOK
}

func runReact(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, messageID int, emoji string, big bool) int {
	chatID, _, _, err := resolveChat(&g, prof, nil)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	if messageID == 0 || emoji == "" {
		PrintErr("indique --message-id y --emoji")
		return ExitUsage
	}
	if g.DryRun {
		PrintErr("dry-run setMessageReaction chat=%s msgid=%d emoji=%s token=%s", chatID, messageID, emoji, c.TokenRedacted())
		return ExitOK
	}
	req := telegram.SetMessageReactionRequest{
		ChatID: chatID, MessageID: messageID,
		Reaction: []telegram.MessageReactionType{{Type: "emoji", Emoji: emoji}},
	}
	if big {
		req.IsBig = telegram.BoolPtr(true)
	}
	if err := c.SetMessageReaction(ctx, req); err != nil {
		return fail(pr, g, err)
	}
	pr.OK("ok")
	return ExitOK
}

func runForward(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, fromChat string, messageID int, copy bool) int {
	chatID, threadID, _, err := resolveChat(&g, prof, nil)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	if fromChat == "" || messageID == 0 {
		PrintErr("indique --from y --message-id")
		return ExitUsage
	}
	var threadPtr *int
	if threadID > 0 {
		threadPtr = &threadID
	}
	if g.DryRun {
		op := "forwardMessage"
		if copy {
			op = "copyMessage"
		}
		PrintErr("dry-run %s to=%s from=%s msgid=%d token=%s", op, chatID, fromChat, messageID, c.TokenRedacted())
		return ExitOK
	}
	if copy {
		req := telegram.CopyMessageRequest{
			ChatID: chatID, FromChatID: fromChat, MessageID: messageID,
			MessageThreadID: threadPtr, Caption: g.Caption, ParseMode: g.ParseMode,
		}
		if g.Silent {
			req.DisableNotification = telegram.BoolPtr(true)
		}
		res, err := c.CopyMessage(ctx, req)
		if err != nil {
			return fail(pr, g, err)
		}
		pr.MessageID(res.MessageID)
		return ExitOK
	}
	req := telegram.ForwardMessageRequest{
		ChatID: chatID, FromChatID: fromChat, MessageID: messageID,
		MessageThreadID: threadPtr,
	}
	if g.Silent {
		req.DisableNotification = telegram.BoolPtr(true)
	}
	msg, err := c.ForwardMessage(ctx, req)
	if err != nil {
		return fail(pr, g, err)
	}
	pr.Message(msg)
	return ExitOK
}

func runAction(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, action string) int {
	chatID, threadID, _, err := resolveChat(&g, prof, nil)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	var threadPtr *int
	if threadID > 0 {
		threadPtr = &threadID
	}
	if g.DryRun {
		PrintErr("dry-run sendChatAction chat=%s action=%s token=%s", chatID, action, c.TokenRedacted())
		return ExitOK
	}
	if err := c.SendChatAction(ctx, telegram.SendChatActionRequest{
		ChatID: chatID, Action: action, MessageThreadID: threadPtr,
	}); err != nil {
		return fail(pr, g, err)
	}
	pr.OK("ok")
	return ExitOK
}

func runChat(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, pos []string) int {
	chatID, _, _, err := resolveChat(&g, prof, pos)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	if g.DryRun {
		PrintErr("dry-run getChat chat=%s token=%s", chatID, c.TokenRedacted())
		return ExitOK
	}
	chat, err := c.GetChat(ctx, chatID)
	if err != nil {
		return fail(pr, g, err)
	}
	pr.Chat(chat)
	return ExitOK
}

func runMember(ctx context.Context, c *telegram.Client, pr Printer, g GlobalFlags, prof config.Profile, userID int64) int {
	chatID, _, _, err := resolveChat(&g, prof, nil)
	if err != nil {
		PrintErr("%v", err)
		return ExitUsage
	}
	if userID == 0 {
		if g.DryRun {
			PrintErr("dry-run getMe+getChatMember token=%s", c.TokenRedacted())
			return ExitOK
		}
		me, err := c.GetMe(ctx)
		if err != nil {
			return fail(pr, g, err)
		}
		userID = me.ID
	}
	if g.DryRun {
		PrintErr("dry-run getChatMember chat=%s user=%d token=%s", chatID, userID, c.TokenRedacted())
		return ExitOK
	}
	m, err := c.GetChatMember(ctx, chatID, userID)
	if err != nil {
		return fail(pr, g, err)
	}
	pr.Member(m)
	return ExitOK
}

func fail(pr Printer, g GlobalFlags, err error) int {
	code := mapError(err)
	msg := err.Error()
	// Asegurar que el token no se filtre si alguien lo metió en el error
	pr.ErrorJSON(code, msg)
	if !g.JSON {
		PrintErr("%v", err)
	}
	return code
}

func trimRightNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
