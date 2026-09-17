package telegram

import (
	"context"
	"fmt"
)

// SendMessageRequest parámetros de sendMessage.
type SendMessageRequest struct {
	ChatID              string              `json:"chat_id"`
	Text                string              `json:"text"`
	ParseMode           string              `json:"parse_mode,omitempty"`
	MessageThreadID     *int                `json:"message_thread_id,omitempty"`
	DisableNotification *bool               `json:"disable_notification,omitempty"`
	ProtectContent      *bool               `json:"protect_content,omitempty"`
	ReplyToMessageID    *int                `json:"reply_to_message_id,omitempty"`
	LinkPreviewOptions  *LinkPreviewOptions `json:"link_preview_options,omitempty"`
}

// EditMessageTextRequest parámetros de editMessageText.
type EditMessageTextRequest struct {
	ChatID             string              `json:"chat_id"`
	MessageID          int                 `json:"message_id"`
	Text               string              `json:"text"`
	ParseMode          string              `json:"parse_mode,omitempty"`
	LinkPreviewOptions *LinkPreviewOptions `json:"link_preview_options,omitempty"`
}

// EditMessageCaptionRequest parámetros de editMessageCaption.
type EditMessageCaptionRequest struct {
	ChatID    string `json:"chat_id"`
	MessageID int    `json:"message_id"`
	Caption   string `json:"caption"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// ForwardMessageRequest parámetros de forwardMessage.
type ForwardMessageRequest struct {
	ChatID              string `json:"chat_id"`
	FromChatID          string `json:"from_chat_id"`
	MessageID           int    `json:"message_id"`
	MessageThreadID     *int   `json:"message_thread_id,omitempty"`
	DisableNotification *bool  `json:"disable_notification,omitempty"`
	ProtectContent      *bool  `json:"protect_content,omitempty"`
}

// CopyMessageRequest parámetros de copyMessage.
type CopyMessageRequest struct {
	ChatID              string `json:"chat_id"`
	FromChatID          string `json:"from_chat_id"`
	MessageID           int    `json:"message_id"`
	MessageThreadID     *int   `json:"message_thread_id,omitempty"`
	Caption             string `json:"caption,omitempty"`
	ParseMode           string `json:"parse_mode,omitempty"`
	DisableNotification *bool  `json:"disable_notification,omitempty"`
	ProtectContent      *bool  `json:"protect_content,omitempty"`
	ReplyToMessageID    *int   `json:"reply_to_message_id,omitempty"`
}

// MessageIDResult es el resultado de copyMessage.
type MessageIDResult struct {
	MessageID int `json:"message_id"`
}

// SendChatActionRequest parámetros de sendChatAction.
type SendChatActionRequest struct {
	ChatID          string `json:"chat_id"`
	Action          string `json:"action"`
	MessageThreadID *int   `json:"message_thread_id,omitempty"`
}

// PinChatMessageRequest parámetros de pinChatMessage.
type PinChatMessageRequest struct {
	ChatID              string `json:"chat_id"`
	MessageID           int    `json:"message_id"`
	DisableNotification *bool  `json:"disable_notification,omitempty"`
}

// SetMessageReactionRequest parámetros de setMessageReaction.
type SetMessageReactionRequest struct {
	ChatID    string                `json:"chat_id"`
	MessageID int                   `json:"message_id"`
	Reaction  []MessageReactionType `json:"reaction,omitempty"`
	IsBig     *bool                 `json:"is_big,omitempty"`
}

// SendMessage envía un mensaje de texto.
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (*Message, error) {
	if req.Text == "" {
		return nil, &ValidationError{Message: "text vacío"}
	}
	if len([]rune(req.Text)) > MaxTextLength {
		return nil, &ValidationError{
			Message: fmt.Sprintf("texto supera %d caracteres (use --split)", MaxTextLength),
		}
	}
	var msg Message
	if err := c.doJSON(ctx, "sendMessage", req, req.ChatID, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// EditMessageText edita el texto de un mensaje.
func (c *Client) EditMessageText(ctx context.Context, req EditMessageTextRequest) (*Message, error) {
	var msg Message
	if err := c.doJSON(ctx, "editMessageText", req, req.ChatID, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// EditMessageCaption edita el caption de un mensaje con media.
func (c *Client) EditMessageCaption(ctx context.Context, req EditMessageCaptionRequest) (*Message, error) {
	var msg Message
	if err := c.doJSON(ctx, "editMessageCaption", req, req.ChatID, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// DeleteMessage borra un mensaje.
func (c *Client) DeleteMessage(ctx context.Context, chatID string, messageID int) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	return c.doJSON(ctx, "deleteMessage", payload, chatID, nil)
}

// PinChatMessage fija un mensaje.
func (c *Client) PinChatMessage(ctx context.Context, req PinChatMessageRequest) error {
	return c.doJSON(ctx, "pinChatMessage", req, req.ChatID, nil)
}

// UnpinChatMessage desfija un mensaje (messageID=0 desfija el más reciente fijado).
func (c *Client) UnpinChatMessage(ctx context.Context, chatID string, messageID int) error {
	payload := map[string]any{"chat_id": chatID}
	if messageID > 0 {
		payload["message_id"] = messageID
	}
	return c.doJSON(ctx, "unpinChatMessage", payload, chatID, nil)
}

// SetMessageReaction establece reacciones a un mensaje.
func (c *Client) SetMessageReaction(ctx context.Context, req SetMessageReactionRequest) error {
	return c.doJSON(ctx, "setMessageReaction", req, req.ChatID, nil)
}

// ForwardMessage reenvía un mensaje.
func (c *Client) ForwardMessage(ctx context.Context, req ForwardMessageRequest) (*Message, error) {
	var msg Message
	if err := c.doJSON(ctx, "forwardMessage", req, req.ChatID, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// CopyMessage copia un mensaje sin "Reenviado de".
func (c *Client) CopyMessage(ctx context.Context, req CopyMessageRequest) (*MessageIDResult, error) {
	var res MessageIDResult
	if err := c.doJSON(ctx, "copyMessage", req, req.ChatID, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// SendChatAction envía una acción de chat (typing, upload_photo, etc.).
func (c *Client) SendChatAction(ctx context.Context, req SendChatActionRequest) error {
	var ok bool
	return c.doJSON(ctx, "sendChatAction", req, req.ChatID, &ok)
}
