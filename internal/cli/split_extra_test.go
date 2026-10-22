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

// Regresión: en el último tramo, si el encogimiento redujo el corte debía
// conservarse la reapertura; antes el </b> original quedaba huérfano
// (parte 3 = "[3/3] aaaa</b>") y Telegram devolvía 400.
func TestSplitText_UltimoTramoEncogido_ConservaReapertura(t *testing.T) {
	text := "<b>" + strings.Repeat("a", 1970) + "</b>"
	parts := SplitText(text, 1000)
	for i, p := range parts {
		if strings.Count(p, "<b>") != strings.Count(p, "</b>") {
			t.Fatalf("parte %d desbalanceada: %q", i, p)
		}
	}
}

// Regresión: "2026: balance" es texto con dos puntos, no chat:thread.
func TestLooksLikeChat_Espacios(t *testing.T) {
	for _, s := range []string{"2026: balance del año", "1: hola", "589215 mundo"} {
		if looksLikeChat(s) {
			t.Errorf("%q no debe parecer chat", s)
		}
	}
	if !looksLikeChat("-100123:42") || !looksLikeChat("589215") {
		t.Fatal("IDs legítimos rechazados")
	}
}

// Comentarios HTML no se balancean y los '>' dentro de atributos entre
// comillas no cierran la etiqueta.
func TestSplitText_ComentariosYAtributosConComillas(t *testing.T) {
	text := "<!-- nota: hola --><a href=\"x>y\">enlace " + strings.Repeat("z", 3000) + "</a>"
	parts := SplitText(text, 500)
	for i, p := range parts {
		if strings.Count(p, "<a ") != strings.Count(p, "</a>") {
			t.Fatalf("parte %d desbalanceada: %q", i, p[:60])
		}
		if strings.Contains(p, "</>") {
			t.Fatalf("parte %d con cierre basura de comentario: %q", i, p[:60])
		}
	}
}

// Con maxLen chico se parte sin prefijos y sin pérdida.
func TestSplitText_MaxLenChico_SinNumeracion(t *testing.T) {
	text := strings.Repeat("w", 200)
	parts := SplitText(text, 20)
	got := 0
	for _, p := range parts {
		if utf8.RuneCountInString(p) > 20 {
			t.Fatalf("parte de %d runas > 20", utf8.RuneCountInString(p))
		}
		if strings.HasPrefix(p, "[") {
			t.Fatalf("no debe haber prefijo con maxLen chico: %q", p)
		}
		got += utf8.RuneCountInString(p)
	}
	if got != 200 {
		t.Fatalf("runas perdidas: %d", got)
	}
}

// Muchas partes: la reserva del prefijo se ajusta a punto fijo; el contenido
// no se recorta (garantía "sin pérdida" también con prefijos anchos).
func TestSplitText_MuchasPartes_ReservaPuntoFijo(t *testing.T) {
	text := strings.Repeat("q", 4000)
	parts := SplitText(text, 40)
	got := 0
	for _, p := range parts {
		if utf8.RuneCountInString(p) > 40 {
			t.Fatalf("parte de %d runas > 40", utf8.RuneCountInString(p))
		}
		body := p
		if i := strings.Index(p, "] "); i >= 0 {
			body = p[i+2:]
		}
		got += utf8.RuneCountInString(body)
	}
	if got != 4000 {
		t.Fatalf("runas perdidas: %d", got)
	}
}
