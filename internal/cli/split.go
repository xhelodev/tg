package cli

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xhelodev/tg/pkg/telegram"
)

// SplitText parte texto en chunks ≤ maxLen, preferiendo cortes en newlines.
// Evita cortar en medio de una etiqueta HTML simple (<...>).
// Cada parte se numera si hay más de una: [1/N] prefijo.
func SplitText(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = telegram.MaxTextLength
	}
	if utf8.RuneCountInString(text) <= maxLen {
		return []string{text}
	}

	var parts []string
	runes := []rune(text)
	for len(runes) > 0 {
		if len(runes) <= maxLen {
			parts = append(parts, string(runes))
			break
		}
		cut := maxLen
		// Buscar salto de línea/párrafo hacia atrás
		window := runes[:cut]
		best := -1
		for i := len(window) - 1; i >= len(window)/2; i-- {
			if window[i] == '\n' {
				best = i + 1
				break
			}
		}
		if best > 0 {
			cut = best
		}
		// No cortar dentro de <...>
		chunk := string(runes[:cut])
		if open := strings.LastIndex(chunk, "<"); open >= 0 {
			close := strings.Index(chunk[open:], ">")
			if close < 0 {
				// etiqueta abierta: cortar antes del <
				if open > 0 {
					cut = open
					chunk = string(runes[:cut])
				}
			}
		}
		if cut <= 0 {
			cut = maxLen
			chunk = string(runes[:cut])
		}
		parts = append(parts, chunk)
		runes = runes[cut:]
	}

	if len(parts) <= 1 {
		return parts
	}
	n := len(parts)
	// Reservar espacio para el prefijo [i/n] (~10 chars)
	out := make([]string, n)
	for i, p := range parts {
		prefix := fmt.Sprintf("[%d/%d] ", i+1, n)
		// Si con prefijo se pasa, recortar el cuerpo
		body := p
		limit := maxLen - utf8.RuneCountInString(prefix)
		if limit > 0 && utf8.RuneCountInString(body) > limit {
			body = string([]rune(body)[:limit])
		}
		out[i] = prefix + body
	}
	return out
}
