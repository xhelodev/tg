package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Límites documentados (ajustables; no hardcodear en call sites).
const (
	MaxTextLength       = 4096
	MaxCaptionLength    = 1024
	MaxPhotoURLBytes    = 5 << 20  // 5 MB
	MaxOtherURLBytes    = 20 << 20 // 20 MB
	MaxPhotoMultipart   = 10 << 20 // 10 MB
	MaxOtherMultipart   = 50 << 20 // 50 MB
	MaxLocalAPIBytes    = 2000 << 20
)

// MediaKind identifica el tipo de media a enviar.
type MediaKind string

const (
	MediaPhoto     MediaKind = "photo"
	MediaVideo     MediaKind = "video"
	MediaAudio     MediaKind = "audio"
	MediaVoice     MediaKind = "voice"
	MediaAnimation MediaKind = "animation"
	MediaDocument  MediaKind = "document"
	MediaSticker   MediaKind = "sticker"
)

// SendMediaRequest describe un envío de media (file_id, URL o ruta local).
type SendMediaRequest struct {
	ChatID              string
	Kind                MediaKind
	// Exactamente uno de FileID, URL, FilePath debe estar set.
	FileID   string
	URL      string
	FilePath string

	Caption             string
	ParseMode           string
	MessageThreadID     *int
	DisableNotification *bool
	ProtectContent      *bool
	ReplyToMessageID    *int
}

// MaxUploadBytes devuelve el límite aplicable según kind, fuente y si es API local.
func (c *Client) MaxUploadBytes(kind MediaKind, localFile bool) int64 {
	if c.IsLocalAPI() {
		return MaxLocalAPIBytes
	}
	isPhoto := kind == MediaPhoto
	if localFile {
		if isPhoto {
			return MaxPhotoMultipart
		}
		return MaxOtherMultipart
	}
	// URL
	if isPhoto {
		return MaxPhotoURLBytes
	}
	return MaxOtherURLBytes
}

// SendMedia envía foto/video/audio/etc. Usa JSON si file_id/URL; multipart si ruta local.
func (c *Client) SendMedia(ctx context.Context, req SendMediaRequest) (*Message, error) {
	if req.Kind == "" {
		return nil, &ValidationError{Message: "media kind vacío"}
	}
	if req.Caption != "" && len([]rune(req.Caption)) > MaxCaptionLength {
		return nil, &ValidationError{
			Message: fmt.Sprintf("caption supera %d caracteres", MaxCaptionLength),
		}
	}

	method := "send" + strings.ToUpper(string(req.Kind[:1])) + string(req.Kind[1:])

	switch {
	case req.FilePath != "":
		return c.sendMediaMultipart(ctx, method, req)
	case req.FileID != "" || req.URL != "":
		return c.sendMediaJSON(ctx, method, req)
	default:
		return nil, &ValidationError{Message: "indique file_id, URL o ruta local"}
	}
}

func (c *Client) sendMediaJSON(ctx context.Context, method string, req SendMediaRequest) (*Message, error) {
	field := string(req.Kind)
	value := req.FileID
	if value == "" {
		value = req.URL
		// sendDocument por URL solo acepta .PDF y .ZIP
		if req.Kind == MediaDocument {
			lower := strings.ToLower(value)
			if !strings.HasSuffix(lower, ".pdf") && !strings.HasSuffix(lower, ".zip") {
				return nil, &ValidationError{
					Message: "sendDocument por URL solo acepta .PDF y .ZIP; suba el archivo como ruta local",
				}
			}
		}
	}
	payload := map[string]any{
		"chat_id": req.ChatID,
		field:     value,
	}
	if req.Caption != "" {
		payload["caption"] = req.Caption
	}
	if req.ParseMode != "" {
		payload["parse_mode"] = req.ParseMode
	}
	if req.MessageThreadID != nil {
		payload["message_thread_id"] = *req.MessageThreadID
	}
	if req.DisableNotification != nil {
		payload["disable_notification"] = *req.DisableNotification
	}
	if req.ProtectContent != nil {
		payload["protect_content"] = *req.ProtectContent
	}
	if req.ReplyToMessageID != nil {
		payload["reply_to_message_id"] = *req.ReplyToMessageID
	}
	var msg Message
	if err := c.doJSON(ctx, method, payload, req.ChatID, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (c *Client) sendMediaMultipart(ctx context.Context, method string, req SendMediaRequest) (*Message, error) {
	fi, err := os.Stat(req.FilePath)
	if err != nil {
		return nil, &ValidationError{Message: "archivo: " + err.Error()}
	}
	limit := c.MaxUploadBytes(req.Kind, true)
	if fi.Size() > limit {
		return nil, &ValidationError{
			Message: fmt.Sprintf(
				"archivo de %d bytes supera el límite de %d bytes para %s (multipart)",
				fi.Size(), limit, req.Kind,
			),
		}
	}

	f, err := os.Open(req.FilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("chat_id", req.ChatID)
	if req.Caption != "" {
		_ = w.WriteField("caption", req.Caption)
	}
	if req.ParseMode != "" {
		_ = w.WriteField("parse_mode", req.ParseMode)
	}
	if req.MessageThreadID != nil {
		_ = w.WriteField("message_thread_id", strconv.Itoa(*req.MessageThreadID))
	}
	if req.DisableNotification != nil {
		_ = w.WriteField("disable_notification", strconv.FormatBool(*req.DisableNotification))
	}
	if req.ProtectContent != nil {
		_ = w.WriteField("protect_content", strconv.FormatBool(*req.ProtectContent))
	}
	if req.ReplyToMessageID != nil {
		_ = w.WriteField("reply_to_message_id", strconv.Itoa(*req.ReplyToMessageID))
	}

	field := string(req.Kind)
	filename := filepath.Base(req.FilePath)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
	h.Set("Content-Type", "application/octet-stream")
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	var msg Message
	if err := c.doMultipart(ctx, method, buf.Bytes(), w.FormDataContentType(), req.ChatID, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
