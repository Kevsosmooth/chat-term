package bot

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"chat-term/internal/config"
	"chat-term/internal/term"
)

var update = flag.Bool("update", false, "rewrite internal/guide/cheatsheet.html")

func TestStartHereNamesRealCommands(t *testing.T) {
	for _, m := range regexp.MustCompile(`\.([a-z]+)`).FindAllStringSubmatch(startHere("."), -1) {
		if lookup(m[1]) == nil && !isKeyCommand(m[1]) {
			t.Errorf("start-here mentions .%s, which is not a command", m[1])
		}
	}
}

func TestFullHelpListsEveryCommand(t *testing.T) {
	full := fullHelp(".")
	for _, c := range commands {
		if !strings.Contains(full, "."+c.name) {
			t.Errorf(".help all is missing .%s", c.name)
		}
	}
}

func TestCheatsheet(t *testing.T) {
	path := filepath.Join("..", "guide", "cheatsheet.html")
	got := cheatsheetHTML(".")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("updated; now render the PNG and PDF (see internal/guide/guide.go)")
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatal("cheatsheet.html is out of date: run go test ./internal/bot -run TestCheatsheet -update, then render it (see internal/guide/guide.go)")
	}
	for _, c := range commands {
		if !strings.Contains(got, "."+c.name) {
			t.Errorf("cheat sheet is missing .%s", c.name)
		}
	}
}

func newQuietBot(out *[]string) *Bot {
	return New(config.Default(), term.Tmux{Socket: "unused"}, func(s string) { *out = append(*out, s) })
}

func TestWelcomeOnlyForFirstPlainMessage(t *testing.T) {
	var out []string
	b := newQuietBot(&out)
	if !b.greet(false) || !strings.Contains(out[0], "Welcome") {
		t.Fatalf("first plain message not welcomed: %q", out)
	}
	if b.greet(false) {
		t.Error("welcomed twice")
	}
	c := newQuietBot(&out)
	if c.greet(true) || c.greet(false) {
		t.Error("a chat that starts with a command was welcomed later")
	}
}

func TestGuideSendsPictureOrText(t *testing.T) {
	var out []string
	b := newQuietBot(&out)
	b.cmdGuide("")
	if len(out) == 0 || !strings.Contains(out[0], "text version") {
		t.Fatalf("no text fallback without pictures: %q", out)
	}

	out = nil
	var caption string
	b.sendImage = func(png []byte, c string) error { caption = c; return nil }
	b.cmdGuide("")
	if caption == "" || len(out) != 0 {
		t.Fatalf("picture not sent alone: caption %q, text %q", caption, out)
	}

	b.sendImage = func([]byte, string) error { return errors.New("offline") }
	b.cmdGuide("")
	if len(out) == 0 || !strings.Contains(out[0], "offline") {
		t.Fatalf("failed picture gave no text: %q", out)
	}
}
