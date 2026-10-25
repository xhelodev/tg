package telegram

import "context"

// Update es un update de getUpdates (solo los tipos de mensaje).
type Update struct {
	UpdateID      int64    `json:"update_id"`
	Message       *Message `json:"message,omitempty"`
	EditedMessage *Message `json:"edited_message,omitempty"`
	ChannelPost   *Message `json:"channel_post,omitempty"`
}

// AnyMessage devuelve el mensaje que trae el update, o nil.
func (u Update) AnyMessage() *Message {
	switch {
	case u.Message != nil:
		return u.Message
	case u.EditedMessage != nil:
		return u.EditedMessage
	default:
		return u.ChannelPost
	}
}

// GetUpdatesRequest parámetros de getUpdates (long polling).
type GetUpdatesRequest struct {
	Offset         int64    `json:"offset,omitempty"`
	Limit          int      `json:"limit,omitempty"`
	Timeout        int      `json:"timeout,omitempty"` // segundos; debe ser < timeout HTTP
	AllowedUpdates []string `json:"allowed_updates,omitempty"`
}

// GetUpdates llama a getUpdates. Pasar Offset = último update_id + 1
// confirma (y descarta en el servidor) los anteriores.
func (c *Client) GetUpdates(ctx context.Context, req GetUpdatesRequest) ([]Update, error) {
	var ups []Update
	if err := c.doJSON(ctx, "getUpdates", req, "", &ups); err != nil {
		return nil, err
	}
	return ups, nil
}
