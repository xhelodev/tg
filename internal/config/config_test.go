package config

import "testing"

func TestSplitChatThread(t *testing.T) {
	cases := []struct {
		in    string
		chat  string
		thread int
	}{
		{"589215", "589215", 0},
		{"-100123:42", "-100123", 42},
		{"@canal", "@canal", 0},
		{"@canal:7", "@canal", 7},
	}
	for _, tc := range cases {
		c, th := SplitChatThread(tc.in)
		if c != tc.chat || th != tc.thread {
			t.Errorf("%q -> %q/%d want %q/%d", tc.in, c, th, tc.chat, tc.thread)
		}
	}
}

func TestResolveAlias(t *testing.T) {
	p := Profile{Aliases: map[string]string{"alertas": "-100123:42"}}
	chat, th, ok := p.ResolveAlias("alertas")
	if !ok || chat != "-100123" || th != 42 {
		t.Fatalf("%s %d %v", chat, th, ok)
	}
}
