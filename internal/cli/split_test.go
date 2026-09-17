package cli

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xhelodev/tg/pkg/telegram"
)

func TestSplitText_Exact4096(t *testing.T) {
	text := strings.Repeat("a", 4096)
	parts := SplitText(text, telegram.MaxTextLength)
	if len(parts) != 1 {
		t.Fatalf("len=%d", len(parts))
	}
}

func TestSplitText_4097(t *testing.T) {
	text := strings.Repeat("b", 4097)
	parts := SplitText(text, telegram.MaxTextLength)
	if len(parts) < 2 {
		t.Fatalf("esperado >=2 partes, got %d", len(parts))
	}
	for i, p := range parts {
		if utf8.RuneCountInString(p) > telegram.MaxTextLength {
			t.Fatalf("parte %d demasiado larga: %d", i, utf8.RuneCountInString(p))
		}
		if !strings.HasPrefix(p, "[") {
			t.Fatalf("parte %d sin numeración: %q", i, p[:20])
		}
	}
}

func TestSplitText_HTMLOpenTag(t *testing.T) {
	// Forzar corte cerca de una etiqueta abierta
	prefix := strings.Repeat("x", 4080)
	text := prefix + "<b>hola</b>" + strings.Repeat("y", 50)
	parts := SplitText(text, telegram.MaxTextLength)
	for _, p := range parts {
		// No debe haber < sin > correspondiente de forma tosca en el corte
		if utf8.RuneCountInString(p) > telegram.MaxTextLength {
			t.Fatal("parte demasiado larga")
		}
	}
	if len(parts) < 2 {
		t.Fatal("esperado split")
	}
}

func TestEscapeHTML(t *testing.T) {
	got := escapeHTML(`a<b>&c`)
	if got != "a&lt;b&gt;&amp;c" {
		t.Fatal(got)
	}
}
