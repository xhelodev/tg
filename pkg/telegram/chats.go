package telegram

import "context"

// GetMe llama a getMe y valida el token.
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	var u User
	if err := c.doJSON(ctx, "getMe", nil, "", &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetChat llama a getChat.
func (c *Client) GetChat(ctx context.Context, chatID string) (*Chat, error) {
	payload := map[string]any{"chat_id": chatID}
	var chat Chat
	if err := c.doJSON(ctx, "getChat", payload, chatID, &chat); err != nil {
		return nil, err
	}
	return &chat, nil
}

// GetChatMember llama a getChatMember.
func (c *Client) GetChatMember(ctx context.Context, chatID string, userID int64) (*ChatMember, error) {
	payload := map[string]any{
		"chat_id": chatID,
		"user_id": userID,
	}
	var m ChatMember
	if err := c.doJSON(ctx, "getChatMember", payload, chatID, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
