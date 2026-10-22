package cli

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// Cada parte debe ser HTML balanceada (etiquetas HTML válidas por sí solas),
// no exceder maxLen con el prefijo y no perder contenido: el texto sin
// etiquetas ni prefijos debe conservarse íntegro (el balanceo inserta
// "</b><b>" entre partes a propósito; los datos no).
func TestSplitText_HTMLBalance(t *testing.T) {
	text := "<b>" + strings.Repeat("a", 5000) + "</b>"
	parts := SplitText(text, 1000)
	if len(parts) < 2 {
		t.Fatalf("esperado >=2 partes, got %d", len(parts))
	}
	var content strings.Builder
	for i, p := range parts {
		if utf8.RuneCountInString(p) > 1000 {
			t.Fatalf("parte %d de %d runas excede 1000", i, utf8.RuneCountInString(p))
		}
		if strings.Count(p, "<b>") != strings.Count(p, "</b>") {
			t.Fatalf("parte %d sin cerrar <b>: %q", i, p[:30])
		}
		if !strings.HasPrefix(p, fmt.Sprintf("[%d/%d] ", i+1, len(parts))) {
			t.Fatalf("parte %d sin prefijo: %q", i, p[:12])
		}
		content.WriteString(stripTags(strings.TrimPrefix(p, fmt.Sprintf("[%d/%d] ", i+1, len(parts)))))
	}
	if content.String() != strings.Repeat("a", 5000) {
		t.Fatalf("contenido alterado: %d a's", strings.Count(content.String(), "a"))
	}
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func stripTags(s string) string { return tagRe.ReplaceAllString(s, "") }

func TestSplitText_EtiquetasAnidadas(t *testing.T) {
	text := "<b>negrita <i>itálica " + strings.Repeat("x", 4000) + "</i></b>"
	parts := SplitText(text, 800)
	for i, p := range parts {
		if utf8.RuneCountInString(p) > 800 {
			t.Fatalf("parte %d demasiado larga", i)
		}
		if strings.Count(p, "<i>") != strings.Count(p, "</i>") ||
			strings.Count(p, "<b>") != strings.Count(p, "</b>") {
			t.Fatalf("parte %d desbalanceada: %q", i, p[:40])
		}
	}
}

func TestSplitText_PrefijoNoRecorta(t *testing.T) {
	// Antes: cortaba con maxLen completo y el prefijo descartaba el final
	// en silencio. Ahora el presupuesto se reserva antes de cortar.
	text := strings.Repeat("m", 10000)
	parts := SplitText(text, 1000)
	got := 0
	for _, p := range parts {
		body := p
		if i := strings.Index(p, "] "); i >= 0 {
			body = p[i+2:]
		}
		got += utf8.RuneCountInString(body)
	}
	if got != 10000 {
		t.Fatalf("runas perdidas: got %d want 10000", got)
	}
}

func TestSplitText_Multibyte(t *testing.T) {
	text := strings.Repeat("ñó€", 3000) // 9000 runas multibyte
	parts := SplitText(text, 1000)
	total := 0
	for _, p := range parts {
		if !utf8.ValidString(p) {
			t.Fatal("parte con UTF-8 inválido")
		}
		if utf8.RuneCountInString(p) > 1000 {
			t.Fatal("parte demasiado larga")
		}
		body := p
		if i := strings.Index(p, "] "); i >= 0 {
			body = p[i+2:]
		}
		total += utf8.RuneCountInString(body)
	}
	if total != 9000 {
		t.Fatalf("runas perdidas: %d", total)
	}
}
