package tui

import "testing"

func TestWrapLinePreservesText(t *testing.T) {
	input := "EasyCodeAI terminal interface"
	parts := wrapLine(input, 7)
	joined := ""
	for _, part := range parts {
		if len([]rune(part)) > 7 {
			t.Fatalf("wrapped segment %q exceeds width 7", part)
		}
		joined += part
	}
	if joined != input {
		t.Fatalf("wrapped text = %q, want %q", joined, input)
	}
}

func TestWrapLineKeepsShortLine(t *testing.T) {
	input := "short"
	got := wrapLine(input, 20)
	if len(got) != 1 || got[0] != input {
		t.Fatalf("wrapLine(%q) = %#v, want one unchanged line", input, got)
	}
}
