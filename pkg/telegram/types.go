package telegram

import "encoding/json"

// User representa un usuario o bot de Telegram.
type User struct {
	ID                      int64  `json:"id"`
	IsBot                   bool   `json:"is_bot"`
	FirstName               string `json:"first_name"`
	LastName                string `json:"last_name,omitempty"`
	Username                string `json:"username,omitempty"`
	LanguageCode            string `json:"language_code,omitempty"`
	CanJoinGroups           bool   `json:"can_join_groups,omitempty"`
	CanReadAllGroupMessages bool   `json:"can_read_all_group_messages,omitempty"`
	SupportsInlineQueries   bool   `json:"supports_inline_queries,omitempty"`
}

// Chat representa un chat (privado, grupo, supergrupo o canal).
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"` // private, group, supergroup, channel
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	IsForum   bool   `json:"is_forum,omitempty"`
}

// Message es un mensaje de Telegram (solo campos usados).
type Message struct {
	MessageID       int    `json:"message_id"`
	From            *User  `json:"from,omitempty"`
	Chat            Chat   `json:"chat"`
	Date            int64  `json:"date"`
	Text            string `json:"text,omitempty"`
	Caption         string `json:"caption,omitempty"`
	MessageThreadID int    `json:"message_thread_id,omitempty"`
	// SenderChat: canal/grupo en nombre del cual se envió (admins anónimos, canales).
	SenderChat    *Chat          `json:"sender_chat,omitempty"`
	ForwardOrigin *MessageOrigin `json:"forward_origin,omitempty"`
}

// MessageOrigin describe el origen de un mensaje reenviado.
type MessageOrigin struct {
	Type           string `json:"type"` // user, hidden_user, chat, channel
	Date           int64  `json:"date"`
	SenderUser     *User  `json:"sender_user,omitempty"`      // user
	SenderUserName string `json:"sender_user_name,omitempty"` // hidden_user
	SenderChat     *Chat  `json:"sender_chat,omitempty"`      // chat
	Chat           *Chat  `json:"chat,omitempty"`             // channel
	MessageID      int    `json:"message_id,omitempty"`       // channel
}

// ChatMember describe la membresía de un usuario en un chat.
type ChatMember struct {
	Status              string `json:"status"`
	User                User   `json:"user"`
	CanBeEdited         bool   `json:"can_be_edited,omitempty"`
	CanManageChat       bool   `json:"can_manage_chat,omitempty"`
	CanDeleteMessages   bool   `json:"can_delete_messages,omitempty"`
	CanManageVideoChats bool   `json:"can_manage_video_chats,omitempty"`
	CanRestrictMembers  bool   `json:"can_restrict_members,omitempty"`
	CanPromoteMembers   bool   `json:"can_promote_members,omitempty"`
	CanChangeInfo       bool   `json:"can_change_info,omitempty"`
	CanInviteUsers      bool   `json:"can_invite_users,omitempty"`
	CanPostMessages     bool   `json:"can_post_messages,omitempty"`
	CanEditMessages     bool   `json:"can_edit_messages,omitempty"`
	CanPinMessages      bool   `json:"can_pin_messages,omitempty"`
	CanManageTopics     bool   `json:"can_manage_topics,omitempty"`
	IsAnonymous         bool   `json:"is_anonymous,omitempty"`
}

// ResponseParameters contiene parámetros adicionales en errores de la API.
type ResponseParameters struct {
	MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
	RetryAfter      int   `json:"retry_after,omitempty"`
}

// apiResponse es el sobre estándar de la Bot API.
type apiResponse struct {
	OK          bool                `json:"ok"`
	Result      json.RawMessage     `json:"result,omitempty"`
	Description string              `json:"description,omitempty"`
	ErrorCode   int                 `json:"error_code,omitempty"`
	Parameters  *ResponseParameters `json:"parameters,omitempty"`
}

// MessageReactionType representa una reacción (emoji).
type MessageReactionType struct {
	Type  string `json:"type"` // "emoji" o "custom_emoji"
	Emoji string `json:"emoji,omitempty"`
}

// LinkPreviewOptions controla la vista previa de enlaces.
type LinkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled,omitempty"`
}
