package bot

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// confirmWindow is how long a question stays open for .yes.
const confirmWindow = 60 * time.Second

// pending is an action waiting for .yes: ending a session, or making a
// folder and opening it.
type pending struct {
	kill  string // session to end
	mkdir string // folder to create, then open
	cmd   string // tool to start in the new folder
	until time.Time
}

// confirmsPending reports a bare "yes" while an action waits for .yes; phones
// often drop the dot, and typing "yes" into a shell would print y forever.
func (b *Bot) confirmsPending(text string) bool {
	if b.pending == (pending{}) || time.Now().After(b.pending.until) {
		return false
	}
	w := strings.ToLower(strings.Trim(text, " .!?"))
	return w == "yes" || w == "y"
}

func (b *Bot) cmdYes(string) {
	p := b.pending
	b.pending = pending{}
	switch {
	case p.kill == "" && p.mkdir == "":
		b.say("Nothing to confirm.")
	case time.Now().After(p.until) && p.kill != "":
		b.say("That request expired. Send %skill again.", b.cfg.Prefix)
	case time.Now().After(p.until):
		b.say("That request expired. Send %sopen again.", b.cfg.Prefix)
	case p.kill != "":
		b.endSession(p.kill)
	default:
		b.createAndOpen(p.mkdir, p.cmd)
	}
}

// offerFolder asks before .open makes a missing folder, so a typo doesn't
// leave a stray folder behind. It reports whether it asked.
func (b *Bot) offerFolder(target, cmd string) bool {
	p, err := b.resolvePath(target)
	if err != nil {
		return false
	}
	parent := filepath.Dir(p)
	if st, err := os.Stat(parent); err != nil || !st.IsDir() {
		return false
	}
	if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	b.pending = pending{mkdir: p, cmd: cmd, until: time.Now().Add(confirmWindow)}
	then := "open a terminal there"
	if cmd != "" {
		then = "start " + cmd + " there"
	}
	b.say("There's no folder %s in %s yet. Create it and %s? Send %syes within 60 seconds.",
		filepath.Base(p), displayPath(parent), then, b.cfg.Prefix)
	return true
}

func (b *Bot) createAndOpen(dir, cmd string) {
	if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		b.say("Could not create %s: %v", displayPath(dir), err)
		return
	}
	b.startSession(b.uniqueName(sanitizeName(filepath.Base(dir))), dir, cmd)
}
