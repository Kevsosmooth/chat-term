package bot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kevsosmooth/chat-term/internal/config"
	"github.com/Kevsosmooth/chat-term/internal/term"
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

	h.expect("echo hi", "Welcome to chat-term", ".help all")
	h.expect("echo hi", "No active session")
	h.expect(".help", "start here", ".open 2 claude", ".guide")
	h.expect(".help all", "*Directories*", ".open <n|folder> [cmd] - new session", ".esc", ".help <command>")
	h.expect(".help open", ".open 3 claude", "Short form: .o")
	h.expect(".nope", "Unknown command .nope")
	h.expect(".opne 1", "Did you mean .open?")
	h.expect(". open nope", "No folder \"nope\"", ".projects - list your projects")

	h.expect(".   projects", "1 alpha", "2 beta")
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

	h.expect("echo waiting; sleep 30", "· sleep]", "waiting")
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

func TestPhoneInput(t *testing.T) {
	h := newHarness(t)
	if err := os.MkdirAll(filepath.Join(h.b.dir, "alpha", "my notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.b.dir, "alpha", "my notes", "a b.txt"), []byte("spaced file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.expect(".open alpha", "Started alpha")
	h.drain()
	h.expect(".?", "start here")
	h.expect(".Pwd.", "/alpha")
	h.expect(".cat my notes/a b.txt", "spaced file")
	h.expect(`.cat "my notes/a b.txt" 1-`, "spaced file")
	h.expect(".ls my notes", "a b.txt")

	// Output from a command still running when the next message arrives is kept.
	h.b.Handle("echo one-$((1)); sleep 0.25; echo done-$((2))")
	time.Sleep(150 * time.Millisecond)
	h.expect("echo two-$((3))", "one-1", "done-2", "two-3")
	h.drain()

	// Coming back to an earlier screen is a change, not "(no change)".
	h.expect("echo same-$((4))", "same-4")
	h.drain()
	h.b.Handle(".l") // clears the screen
	h.drain()
	h.expect("echo same-$((4))", "same-4")
	h.drain()

	// A program that is slow to draw its first screen is waited for.
	h.expect("sleep 1; echo ready-$((5))", "ready-5")
	h.drain()

	h.expect(".new My Project", "Started My-Project")
	h.drain()
	h.expect(".new my notes", "Started my-notes", "/alpha/my notes")
	h.drain()
	h.expect(".s alpha", "[alpha · ")
	h.expect(". ./missing.sh", "missing.sh")
	h.drain()
	h.expect(".kill", ".yes")
	h.expect("Yes.", "Ended alpha")
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
	if sanitizeName("café ☕") != "cafe" {
		t.Errorf("sanitizeName = %q", sanitizeName("café ☕"))
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

func TestFixShellLine(t *testing.T) {
	for in, want := range map[string]string{
		"Mkdir test-wa-env":            "mkdir test-wa-env",
		"  Cd ..":                      "  cd ..",
		"Echo “hi” ‘x’":                `echo "hi" 'x'`,
		"teamclaude run — —permission": "teamclaude run -- --permission",
		"TC_ACCT=a teamclaude run":     "TC_ACCT=a teamclaude run",
		"Hello there":                  "Hello there",
		"ls\nMkdir a":                  "ls\nMkdir a",
		"Kill stale\nsessions":         "Kill stale\nsessions",
		"sed 's/—/-/g' f":              "sed 's/—/-/g' f",
		"Ls.":                          "ls",
		"cd ..":                        "cd ..",
		"Help":                         "help",
	} {
		if got := fixShellLine(in); got != want {
			t.Errorf("fixShellLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSuggest(t *testing.T) {
	for in, want := range map[string]string{"opne": "open", "proj": "projects", "sttus": "status", "xyzzy": ""} {
		if got := suggest(in); got != want {
			t.Errorf("suggest(%q) = %q, want %q", in, got, want)
		}
	}
}
