package bot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chat-term/internal/config"
	"chat-term/internal/term"
)

// harness runs a bot against a private tmux server, never the user's.
type harness struct {
	t   *testing.T
	b   *Bot
	out chan string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	socket := fmt.Sprintf("chat-term-test-%d", os.Getpid())
	tm := term.Tmux{Socket: socket}
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })

	root := t.TempDir()
	for _, d := range []string{"alpha", "beta/src"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "alpha", "notes.txt"), []byte("line one\nline two\nline three\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.ProjectsRoot = root
	cfg.SettleMS, cfg.PollMS = 400, 100
	h := &harness{t: t, out: make(chan string, 100)}
	h.b = New(cfg, tm, func(s string) { h.out <- s })
	h.b.dir = root
	t.Cleanup(func() { h.b.mu.Lock(); h.b.stopWatching(); h.b.mu.Unlock() })
	return h
}

// expect sends msg and waits for a reply containing every want string.
func (h *harness) expect(msg string, want ...string) string {
	h.t.Helper()
	h.b.Handle(msg)
	deadline := time.After(8 * time.Second)
	var seen []string
	for {
		select {
		case r := <-h.out:
			seen = append(seen, r)
			all := strings.Join(seen, "\n")
			ok := true
			for _, w := range want {
				if !strings.Contains(all, w) {
					ok = false
				}
			}
			if ok {
				return all
			}
		case <-deadline:
			h.t.Fatalf("%q: no reply containing %q; got:\n%s", msg, want, strings.Join(seen, "\n---\n"))
		}
	}
}

func (h *harness) drain() {
	for {
		select {
		case <-h.out:
		case <-time.After(700 * time.Millisecond):
			return
		}
	}
}

func TestEndToEnd(t *testing.T) {
	h := newHarness(t)

	h.expect("echo hi", "No active session")
	h.expect(".help", "*Directories*", ".open <n|name|path> [cmd] - new session", ".esc", ".help <command>")
	h.expect(".help open", ".open 3 claude", "Short form: .o")
	h.expect(".nope", "Unknown command .nope")

	h.expect(".projects", "1 alpha", "2 beta")
	h.expect(".open 1", "Started alpha", "[alpha · ")
	h.drain()
	h.expect(".pwd", "/alpha")
	h.expect(".ls", "1 notes.txt")
	h.expect(".cat 1", "   2 line two", "lines 1-3 of 3")
	h.expect(".cat notes.txt 2-3", "   3 line three", "lines 2-3 of 3")

	h.expect("echo wa-$((40+2))", "wa-42")
	h.expect("..not-a-command 2>/dev/null; echo typed-dot", "typed-dot")

	h.expect(".cd ../beta", "cd --")
	h.drain()
	h.expect(".pwd", "/beta")
	h.expect(".tree", "src/")

	h.expect(".new side", "Started side")
	h.drain()
	h.expect(".ss", "alpha", ">2 side")
	h.expect(".s alp", "[alpha · ")
	h.expect(".back", "[side · ")

	h.expect("sleep 30", "· sleep]", "# sleep 30")
	h.expect(".enter", "no change")
	h.expect(".c", "^C")
	h.drain()

	h.expect(".rename renamed", "Renamed side to renamed")
	h.expect(".kill", "End session renamed", ".yes")
	h.expect(".yes", "Ended renamed")
	h.expect(".screen", "No active session")
	h.expect(".s 99", "no session number 99")
	h.expect(".detach", "Not attached")
	h.expect(".status", "memory", "tmux server: chat-term-test-")
}

func TestCdRefusesWhileToolRuns(t *testing.T) {
	h := newHarness(t)
	h.expect(".open alpha sleep 30", "Started alpha")
	h.drain()
	h.expect(".cd ..", "sleep is running in alpha", ".open")
}

func TestParse(t *testing.T) {
	for _, tc := range []struct{ in, first, rest string }{
		{"open 3 claude --resume", "open", "3 claude --resume"},
		{"  ls  ", "ls", ""},
		{"", "", ""},
	} {
		f, r := splitFirst(tc.in)
		if f != tc.first || r != tc.rest {
			t.Errorf("splitFirst(%q) = %q, %q", tc.in, f, r)
		}
	}
	if sanitizeName("my proj.v2!") != "my-proj-v2" {
		t.Errorf("sanitizeName = %q", sanitizeName("my proj.v2!"))
	}
}

func TestShortPath(t *testing.T) {
	if got := shortPath("/volume1/playground/some/really/long/nested/folder/demo"); got != "…/folder/demo" {
		t.Errorf("shortPath = %q", got)
	}
	if got := shortPath("/volume1/playground"); got != "/volume1/playground" {
		t.Errorf("shortPath = %q", got)
	}
}
