// Package term drives tmux sessions: list, create, type into, and read screens.
package term

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Tmux talks to one tmux server. An empty Socket means the user's default server.
type Tmux struct {
	Socket string
}

type Session struct {
	Name     string
	Attached bool
	Path     string
	Command  string
}

const infoFormat = "#{session_name}\t#{session_attached}\t#{pane_current_path}\t#{pane_current_command}"

func (t Tmux) cmd(args ...string) *exec.Cmd {
	if t.Socket != "" {
		args = append([]string{"-L", t.Socket}, args...)
	}
	return exec.Command("tmux", args...)
}

func (t Tmux) run(stdin string, args ...string) (string, error) {
	var stderr bytes.Buffer
	c := t.cmd(args...)
	c.Stderr = &stderr
	if stdin != "" {
		c.Stdin = strings.NewReader(stdin)
	}
	out, err := c.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("tmux %s: %s", args[0], msg)
	}
	return string(out), nil
}

// target addresses the active pane of an exactly-named session.
func target(name string) string { return "=" + name + ":" }

func (t Tmux) List() ([]Session, error) {
	out, err := t.run("", "list-sessions", "-F", infoFormat)
	if err != nil {
		if strings.Contains(err.Error(), "no server running") || strings.Contains(err.Error(), "error connecting") {
			return nil, nil
		}
		return nil, err
	}
	var sessions []Session
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if s, ok := parseSession(line); ok {
			sessions = append(sessions, s)
		}
	}
	return sessions, nil
}

func parseSession(line string) (Session, bool) {
	f := strings.Split(line, "\t")
	if len(f) != 4 || f[0] == "" {
		return Session{}, false
	}
	return Session{Name: f[0], Attached: f[1] != "0", Path: f[2], Command: f[3]}, true
}

func (t Tmux) Info(name string) (Session, error) {
	out, err := t.run("", "display-message", "-p", "-t", target(name), infoFormat)
	if err != nil {
		return Session{}, err
	}
	s, ok := parseSession(strings.TrimRight(out, "\n"))
	if !ok {
		return Session{}, fmt.Errorf("tmux: unexpected session info %q", out)
	}
	return s, nil
}

func (t Tmux) Exists(name string) bool {
	return t.cmd("has-session", "-t", "="+name).Run() == nil
}

func (t Tmux) New(name, dir string, width, height int) error {
	_, err := t.run("", "new-session", "-d", "-s", name, "-c", dir,
		"-x", strconv.Itoa(width), "-y", strconv.Itoa(height))
	return err
}

// SendText types a single line literally, without pressing Enter.
func (t Tmux) SendText(name, text string) error {
	_, err := t.run("", "send-keys", "-t", target(name), "-l", "--", text)
	return err
}

// Paste inserts multi-line text as a bracketed paste, so apps that support it
// (bash, Claude Code, Codex) treat the lines as one input instead of submitting each.
func (t Tmux) Paste(name, text string) error {
	if _, err := t.run(text, "load-buffer", "-b", "chat-term", "-"); err != nil {
		return err
	}
	_, err := t.run("", "paste-buffer", "-p", "-d", "-b", "chat-term", "-t", target(name))
	return err
}

// SendKeys sends tmux key names such as Enter, Escape, C-c, Up.
func (t Tmux) SendKeys(name string, keys ...string) error {
	_, err := t.run("", append([]string{"send-keys", "-t", target(name)}, keys...)...)
	return err
}

// Capture returns the visible screen plus up to history lines of scrollback.
func (t Tmux) Capture(name string, history int) (string, error) {
	args := []string{"capture-pane", "-p", "-J", "-t", target(name)}
	if history > 0 {
		args = append(args, "-S", "-"+strconv.Itoa(history))
	}
	return t.run("", args...)
}

func (t Tmux) Kill(name string) error {
	_, err := t.run("", "kill-session", "-t", "="+name)
	return err
}

func (t Tmux) Rename(oldName, newName string) error {
	_, err := t.run("", "rename-session", "-t", "="+oldName, newName)
	return err
}
