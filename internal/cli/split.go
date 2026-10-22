package cli

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xhelodev/tg/pkg/telegram"
)

// prefixReserveBase reserva runas para el prefijo "[i/n] " en la primera
// pasada ("[8888/8888] " = 12). La reserva definitiva se calcula a punto
// fijo según el número de partes (ver SplitText).
const prefixReserveBase = 12

// Sin numeración no hay garantía que cumplir; con maxLen muy chico se parte
// sin prefijos antes que descartar texto.
const minNumberedMaxLen = 32

// voidTags no llevan cierre en HTML (no balancear).
var voidTags = map[string]bool{"br": true, "hr": true, "img": true}

// SplitText parte texto en chunks ≤ maxLen, preferiendo cortes en newlines.
// Evita cortar en medio de una etiqueta HTML y balancea las etiquetas entre
// partes: cierra las abiertas al final de cada parte y las reabre al inicio
// de la siguiente, de modo que cada parte es HTML válido por sí sola.
// El espacio del prefijo "[i/n] " se reserva antes de cortar (sin pérdida).
func SplitText(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = telegram.MaxTextLength
	}
	if utf8.RuneCountInString(text) <= maxLen {
		return []string{text}
	}
	if maxLen < minNumberedMaxLen {
		// Sin prefijos: la garantía es sin pérdida, no numeración.
		return splitBodies(text, maxLen, 0)
	}
	// Punto fijo: la reserva depende del nº de partes y viceversa.
	// needed crece log(n), así que converge en 2-3 iteraciones.
	reserve := prefixReserveBase
	for i := 0; i < 8; i++ {
		parts := splitBodies(text, maxLen, reserve)
		n := len(parts)
		if n <= 1 {
			return parts
		}
		needed := utf8.RuneCountInString(fmt.Sprintf("[%d/%d] ", n, n))
		if needed <= reserve {
			return numberParts(parts, maxLen)
		}
		reserve = needed
	}
	// Texto desmesurado (no converge): reserva holgada fija; cubre
	// prefijos de hasta 8 dígitos por lado.
	return numberParts(splitBodies(text, maxLen, 20), maxLen)
}

// splitBodies parte el texto en cuerpos ≤ maxLen-reserve con HTML
// balanceado; SIN numerar (los prefijos los añade numberParts).
func splitBodies(text string, maxLen, reserve int) []string {
	budget := maxLen - reserve
	if budget < 1 {
		budget = 1
	}

	var parts []string
	runes := []rune(text)

	for len(runes) > 0 {
		isLast := len(runes) <= maxLen
		cut := budget
		if isLast {
			cut = len(runes)
		} else {
			cut = chooseCut(runes, budget)
		}
		chunk := string(runes[:cut])
		closers, reopen := closeOpenTags(chunk)
		// Cap único para TODAS las partes (incluida la última): cuerpo +
		// cierres ≤ budget. Así prefijo + cuerpo + cierres ≤ maxLen y el
		// recorte de numeración nunca descarta contenido.
		for cut > 1 && utf8.RuneCountInString(chunk)+utf8.RuneCountInString(closers) > budget {
			if !isLast && cut-1 <= utf8.RuneCountInString(reopen) {
				// Rebalanceo imposible sin perder progreso: cortar sin cerrar.
				closers, reopen = "", ""
				break
			}
			cut--
			chunk = string(runes[:cut])
			closers, reopen = closeOpenTags(chunk)
		}
		if cut == len(runes) {
			// Último tramo real (nada queda detrás): no hay parte que
			// reabrir. OJO: si el encogimiento redujo cut, quedan runas
			// y reopen DEBE conservarse o el </b> original queda huérfano.
			reopen = ""
		}
		parts = append(parts, chunk+closers)

		rest := []rune(string(runes[cut:]))
		if reopen != "" {
			rest = append([]rune(reopen), rest...)
		}
		runes = rest
	}

	return parts
}

// numberParts añade prefijos "[i/n] ". Por construcción del punto fijo el
// prefijo cabe en la reserva, así que el recorte de seguridad no debería
// dispararse jamás; queda como cinturón y tirantes.
func numberParts(parts []string, maxLen int) []string {
	if len(parts) <= 1 {
		return parts
	}
	n := len(parts)
	out := make([]string, n)
	for i, p := range parts {
		prefix := fmt.Sprintf("[%d/%d] ", i+1, n)
		body := p
		limit := maxLen - utf8.RuneCountInString(prefix)
		if limit > 0 && utf8.RuneCountInString(body) > limit {
			body = string([]rune(body)[:limit])
		}
		out[i] = prefix + body
	}
	return out
}

// chooseCut elige el punto de corte dentro del presupuesto: prefiere un
// newline hacia atrás y evita dejar una etiqueta "<..." sin cerrar.
func chooseCut(runes []rune, budget int) int {
	cut := budget
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
	for cut > 1 {
		chunk := string(runes[:cut])
		// '<' sin '>' que lo cierre (respetando comillas): cortar antes.
		if open := strings.LastIndex(chunk, "<"); open >= 0 && tagEnd(chunk, open) < 0 {
			cut = open
			continue
		}
		break
	}
	if cut <= 0 {
		cut = 1
	}
	return cut
}

// tagEnd devuelve el índice del '>' que cierra la etiqueta que abre en
// start ('<'), respetando comillas dentro de atributos (`<a href="x>y">`);
// -1 si la etiqueta queda sin cerrar en s.
func tagEnd(s string, start int) int {
	inQuote := byte(0)
	for i := start + 1; i < len(s); i++ {
		c := s[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '>':
			return i
		}
	}
	return -1
}

// closeOpenTags analiza chunk (que arranca con las etiquetas de la parte
// anterior ya reabiertas, así que ninguna pila se hereda) y devuelve los
// cierres a añadir al final para dejarlo balanceado y la reapertura a
// anteponer a la siguiente parte. Función pura: no mantiene estado.
//
// Límites conocidos (documentados): los comentarios `<!-- … -->` se
// omiten (no se balancean), y un comentario/etiqueta sin cierre dentro
// del chunk hace cortar antes de su '<'.
func closeOpenTags(chunk string) (closers, reopen string) {
	var stack []string
	for i := 0; i < len(chunk); {
		if chunk[i] != '<' {
			i++
			continue
		}
		// Comentarios HTML: región ignorada, no se balancea.
		if strings.HasPrefix(chunk[i:], "<!--") {
			end := strings.Index(chunk[i:], "-->")
			if end < 0 {
				break
			}
			i += end + 3
			continue
		}
		end := tagEnd(chunk, i)
		if end < 0 {
			break
		}
		tag := chunk[i+1 : end]
		switch {
		case strings.HasPrefix(tag, "!"):
			// declaraciones <!…>: no balancean
		case strings.HasPrefix(tag, "/"):
			name := tagName(tag[1:])
			for j := len(stack) - 1; j >= 0; j-- {
				if tagName(stack[j]) == name {
					stack = append(stack[:j], stack[j+1:]...)
					break
				}
			}
		case !strings.HasSuffix(tag, "/") && !voidTags[tagName(tag)]:
			stack = append(stack, tag)
		}
		i = end + 1
	}
	for j := len(stack) - 1; j >= 0; j-- {
		closers += "</" + tagName(stack[j]) + ">"
	}
	for _, t := range stack {
		reopen += "<" + t + ">"
	}
	return closers, reopen
}

// tagName extrae el nombre de etiqueta ("a" de `a href="…"`, "b" de "b").
func tagName(tag string) string {
	tag = strings.TrimSpace(tag)
	var b strings.Builder
	for _, r := range tag {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
			continue
		}
		break
	}
	return strings.ToLower(b.String())
}
