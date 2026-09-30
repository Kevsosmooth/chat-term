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

func TestCleanDropsBrailleArt(t *testing.T) {
	got := Clean("  >_ OpenAI Codex\n      ⢀⣠⣤⣶⣶⣦⣤⣄⡀\n    ⣴⣿⣿⡿ ⣿⣿⡇\n⠋ Installing\n› Ask", 0)
	want := []string{"  >_ OpenAI Codex", "", "Installing", "› Ask"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Clean = %q, want %q", got, want)
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

func TestOnlyDigitsChanged(t *testing.T) {
	a := []string{"❯", "  cpu 29% · 0h01m"}
	if !OnlyDigitsChanged(a, []string{"❯", "  cpu 31% · 0h02m"}) {
		t.Error("clock tick counted as a real change")
	}
	if OnlyDigitsChanged(a, []string{"✻ Churning…", "  cpu 31% · 0h02m"}) {
		t.Error("new text counted as a digit tick")
	}
}

func TestBusyPerTool(t *testing.T) {
	for _, tc := range []struct {
		tool, raw string
		want      bool
	}{
		{"claude", "✻ Churning… (5s · esc to interrupt)\n\n❯", true},
		{"teamclaude", "✶ Pondering…\n❯\n  Opus · high", true},
		{"claude", "● done\n✻ Brewed for 2s · done 9:14 AM\n❯", false},
		{"codex", "• Working (3s • esc to interrupt)\n›", true},
		{"codex", "• codex test ok\n  Worked for 3s • 9:15 AM\n› Ask Codex", false},
		{"gemini", "⠼ Thinking (esc to cancel, 2s)\n>", true},
		{"opencode", "  Generating...\n  Ask anything", true},
		{"opencode", "  Ask anything\n  press enter to send", false},
		{"copilot", "⠙ Reading files\n>", true},
		{"pi", "[subagent] review\npi>", true},
		{"bash", "✻ Churning…", false}, // claude's spinner means nothing to other tools
		{"claude", "esc to interrupt\n" + strings.Repeat("line\n", 20) + "❯", false},
	} {
		if got := Busy(tc.tool, tc.raw); got != tc.want {
			t.Errorf("Busy(%q, %q) = %v, want %v", tc.tool, tc.raw, got, tc.want)
		}
	}
}

func TestCleanDropsReplacementChars(t *testing.T) {
	got := Clean("ctx ��� 3%", 0)
	if len(got) != 1 || got[0] != "ctx  3%" {
		t.Errorf("got %q", got)
	}
}

func TestSeenSkipsLinesBackAfterMenu(t *testing.T) {
	var s Seen
	s.Add([]string{"❯ /effort medium", "done", "❯"})
	s.Add([]string{"Do you want to proceed?", "❯ 1. Yes"}) // menu hid the history
	got := s.Diff([]string{"❯ /effort medium", "done", "● 1 file", "❯"})
	if len(got) != 2 || got[0] != "● 1 file" {
		t.Errorf("got %q, want the new answer onward", got)
	}
}

func TestSeenKeepsRepeatedOutput(t *testing.T) {
	var s Seen
	s.Add([]string{"$ ls", "a.txt", "$"})
	s.Add([]string{"$ ls", "a.txt", "$"})
	got := s.Diff([]string{"$ ls", "a.txt", "$ ls", "a.txt", "$"})
	if len(got) != 3 || got[0] != "$ ls" {
		t.Errorf("got %q, want the second ls onward", got)
	}
}

func TestOnlyDigitsChangedSpotsProgress(t *testing.T) {
	a := []string{"$ git clone x", "Receiving objects:  12% (120/1000)"}
	if OnlyDigitsChanged(a, []string{"$ git clone x", "Receiving objects:  13% (130/1000)"}) {
		t.Error("a progress counter counted as idle")
	}
	top := []string{"count 1", "a", "b", "c", "d"}
	if OnlyDigitsChanged(top, []string{"count 2", "a", "b", "c", "d"}) {
		t.Error("a digit change above the status lines counted as idle")
	}
}

func TestSeenSkipsWrappedTail(t *testing.T) {
	var s Seen
	s.Add([]string{"$ echo " + strings.Repeat("x", 70), "$"})
	got := s.Diff([]string{strings.Repeat("x", 10), "$", "$ ls", "a.txt"})
	if len(got) != 2 || got[0] != "$ ls" {
		t.Errorf("got %q, want the new command onward", got)
	}
}
