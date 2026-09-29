package screen

import (
	"strings"
	"testing"
)

func TestCleanStripsBordersKeepsIndent(t *testing.T) {
	raw := "\n\n╭──────────────╮\n│ > hello      │\n╰──────────────╯\nfunc main() {\n    fmt.Println(1)   \n\n\n\n}\n\n"
	got := Clean(raw, 0)
	want := []string{"> hello", "", "func main() {", "    fmt.Println(1)", "", "}"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Clean =\n%q\nwant\n%q", got, want)
	}
}

func TestCleanCapsLongLines(t *testing.T) {
	got := Clean(strings.Repeat("a", 10), 5)
	if len(got) != 1 || got[0] != "aaaa…" {
		t.Fatalf("got %q", got)
	}
}

func TestDiffShowsFromFirstNewLine(t *testing.T) {
	prev := []string{"$ ls", "a b", "$"}
	cur := []string{"$ ls", "a b", "$ echo hi", "hi", "$"}
	got := Diff(prev, cur)
	if strings.Join(got, "|") != "$ echo hi|hi|$" {
		t.Fatalf("got %q", got)
	}
}

func TestDiffCountsRepeatedLines(t *testing.T) {
	prev := []string{"ok"}
	cur := []string{"ok", "ok"}
	if got := Diff(prev, cur); len(got) != 1 || got[0] != "ok" {
		t.Fatalf("got %q", got)
	}
	if got := Diff(cur, cur); got != nil {
		t.Fatalf("unchanged screen should diff to nil, got %q", got)
	}
}

func TestChunkSplitsOnLines(t *testing.T) {
	text := "aaaa\nbbbb\ncccc"
	got := Chunk(text, 9)
	if strings.Join(got, "|") != "aaaa\nbbbb|cccc" {
		t.Fatalf("got %q", got)
	}
	long := Chunk(strings.Repeat("é", 10), 7) // 2-byte runes never split
	for _, c := range long {
		if !strings.HasPrefix(c, "é") || len(c) > 7 {
			t.Fatalf("bad rune split: %q", long)
		}
	}
}
