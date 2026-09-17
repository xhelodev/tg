package telegram

import (
	"fmt"
	"strings"
)

// APIError representa un fallo de la Bot API (ok:false o HTTP de error).
type APIError struct {
	Code            int
	Description     string
	RetryAfter      int
	MigrateToChatID int64
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "telegram API error %d: %s", e.Code, e.Description)
	if e.RetryAfter > 0 {
		fmt.Fprintf(&b, " (retry_after=%d)", e.RetryAfter)
	}
	if e.MigrateToChatID != 0 {
		fmt.Fprintf(&b, " (migrate_to_chat_id=%d)", e.MigrateToChatID)
	}
	return b.String()
}

// ValidationError indica un token o parámetro inválido antes de la petición.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// NetworkError envuelve fallos de red, DNS o timeout.
type NetworkError struct {
	Err error
}

func (e *NetworkError) Error() string { return "network error: " + e.Err.Error() }
func (e *NetworkError) Unwrap() error { return e.Err }
