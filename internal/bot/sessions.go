package bot

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func (b *Bot) sessionNames() ([]string, error) {
	list, err := b.tmux.List()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(list))
	for i, s := range list {
		names[i] = s.Name
	}
	sort.Strings(names)
	return names, nil
}

// resolveSession accepts a number from the last .ss list, an exact name,
// or a unique case-insensitive part of a name.
func (b *Bot) resolveSession(arg string) (string, error) {
	names, err := b.sessionNames()
	if err != nil {
		return "", err
	}
	if n, err := strconv.Atoi(arg); err == nil {
		list := b.lastSessions
		if len(list) == 0 {
			list = names
		}
		if n < 1 || n > len(list) {
			return "", fmt.Errorf("no session number %d; %sss lists them", n, b.cfg.Prefix)
		}
		if !b.tmux.Exists(list[n-1]) {
			return "", fmt.Errorf("session %s has ended; %sss for a fresh list", list[n-1], b.cfg.Prefix)
		}
		return list[n-1], nil
	}
	return matchName(arg, names, "session")
}

// matchName finds an exact, then unique case-insensitive substring match.
func matchName(arg string, names []string, kind string) (string, error) {
	var hits []string
	for _, n := range names {
		if n == arg {
			return n, nil
		}
		if strings.Contains(strings.ToLower(n), strings.ToLower(arg)) {
			hits = append(hits, n)
		}
	}
	switch len(hits) {
	case 0:
		return "", fmt.Errorf("no %s matches %q", kind, arg)
	case 1:
		return hits[0], nil
	default:
		return "", fmt.Errorf("%q matches several: %s", arg, strings.Join(hits, ", "))
	}
}

func (b *Bot) cmdSessions(string) {
	list, err := b.tmux.List()
	if err != nil {
		b.say("Could not list sessions: %v", err)
		return
	}
	if len(list) == 0 {
		b.say("No tmux sessions. %snew starts one.", b.cfg.Prefix)
		return
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	b.lastSessions = b.lastSessions[:0]
	lines := make([]string, 0, len(list))
	for i, s := range list {
		b.lastSessions = append(b.lastSessions, s.Name)
		mark := " "
		if s.Name == b.active {
			mark = ">"
		}
		extra := ""
		if s.Attached {
			extra = " (open on a screen)"
		}
		lines = append(lines, fmt.Sprintf("%s%d %s · %s · %s%s", mark, i+1, s.Name, displayPath(s.Path), s.Command, extra))
	}
	b.sendBlock(fmt.Sprintf("%d %s · %ss <n> to switch", len(list), plural(len(list), "session", "sessions"), b.cfg.Prefix), lines)
}

func (b *Bot) activate(name string) {
	if b.active != name {
		b.previous = b.active
		b.active = name
	}
	b.stopWatching()
}

func (b *Bot) showScreen(name string, maxLines int) {
	lines := b.snapshot(name, 0)
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	b.sendBlock(b.header(name), lines)
}

func (b *Bot) cmdSwitch(arg string) {
	if arg == "" {
		b.say("Which session? %ss <n|name>. %sss lists them.", b.cfg.Prefix, b.cfg.Prefix)
		return
	}
	name, err := b.resolveSession(arg)
	if err != nil {
		b.say("%v", err)
		return
	}
	b.activate(name)
	b.showScreen(name, 40)
}

func (b *Bot) cmdBack(string) {
	if b.previous == "" || !b.tmux.Exists(b.previous) {
		b.say("No previous session. %sss lists sessions.", b.cfg.Prefix)
		return
	}
	b.activate(b.previous)
	b.showScreen(b.active, 40)
}

func (b *Bot) cmdDetach(string) {
	if b.active == "" {
		b.say("Not attached to a session.")
		return
	}
	b.stopWatching()
	name := b.active
	b.previous, b.active = name, ""
	b.say("Detached from %s. It keeps running; %ss %s to come back.", name, b.cfg.Prefix, name)
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func sanitizeName(s string) string {
	s = strings.Trim(unsafeName.ReplaceAllString(foldAccents(s), "-"), "-")
	if len(s) > 32 {
		s = s[:32]
	}
	if s == "" {
		s = "wa"
	}
	return s
}

// foldAccents turns "café" into "cafe" so a name keeps its letters.
func foldAccents(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// uniqueName appends -2, -3... until the name is free.
func (b *Bot) uniqueName(base string) string {
	name := base
	for i := 2; b.tmux.Exists(name); i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name
}

// startSession creates a session in dir, optionally runs cmd, makes it active,
// and reports its screen once it settles.
func (b *Bot) startSession(name, dir, cmd string) {
	if err := b.tmux.New(name, dir, b.cfg.Width, b.cfg.Height); err != nil {
		b.say("Could not start session: %v", err)
		return
	}
	if cmd != "" {
		if err := b.tmux.SendText(name, fixShellLine(cmd)); err == nil {
			time.Sleep(enterDelay)
			_ = b.tmux.SendKeys(name, "Enter")
		}
	}
	delete(b.seen, name) // a session that once had this name is gone
	b.activate(name)
	b.say("Started %s in %s.", name, displayPath(dir))
	b.watch(name, nil)
}

func looksLikePath(s string) bool {
	return strings.ContainsRune(s, '/') || strings.HasPrefix(s, "~") || strings.HasPrefix(s, ".")
}

func (b *Bot) cmdNew(arg string) {
	first, rest := splitFirst(arg)
	name, dir := "", b.cwd()
	if first != "" && !looksLikePath(first) {
		folder, after := b.splitPath(arg)
		next, _ := splitFirst(after)
		if d, err := b.resolveDir(folder); err == nil && !looksLikePath(next) {
			// ".new api [cmd]" when api is a folder here: start inside it.
			b.startSession(b.uniqueName(sanitizeName(filepath.Base(d))), d, after)
			return
		}
		name, arg = first, rest
	}
	dirArg, cmd := b.splitPath(arg)
	if dirArg != "" {
		d, err := b.resolveDir(dirArg)
		switch {
		case err == nil:
			dir = d
		case looksLikePath(dirArg):
			b.say("%v", err)
			return
		case isCommand(strings.ToLower(dirArg)):
			cmd = arg // ".new api claude": no folder given, run claude here
		default:
			name, cmd = name+" "+arg, "" // ".new My Project": all of it is the name
		}
	}
	if name == "" {
		name = "wa"
	}
	b.startSession(b.uniqueName(sanitizeName(name)), dir, cmd)
}

func (b *Bot) cmdKill(arg string) {
	name := b.active
	if arg != "" {
		n, err := b.resolveSession(arg)
		if err != nil {
			b.say("%v", err)
			return
		}
		name = n
	}
	if name == "" {
		b.say("Which session? %skill <n|name>.", b.cfg.Prefix)
		return
	}
	b.pending = pending{kill: name, until: time.Now().Add(confirmWindow)}
	note := ""
	if strings.HasPrefix(name, "agentdeck_") {
		note = "\nThis is an Agent Deck session; Agent Deck will show it as stopped."
	}
	b.say("End session %s and everything running in it? Send %syes within 60 seconds.%s", name, b.cfg.Prefix, note)
}

// endSession kills a session once .kill has been confirmed.
func (b *Bot) endSession(name string) {
	if err := b.tmux.Kill(name); err != nil {
		b.say("Could not end %s: %v", name, err)
		return
	}
	delete(b.seen, name)
	if b.active == name {
		b.stopWatching()
		b.active = ""
	}
	if b.previous == name {
		b.previous = ""
	}
	b.say("Ended %s.", name)
}

func (b *Bot) cmdRename(arg string) {
	name, ok := b.requireActive()
	if !ok {
		return
	}
	if arg == "" {
		b.say("New name? %srename <name>.", b.cfg.Prefix)
		return
	}
	newName := sanitizeName(arg)
	if b.tmux.Exists(newName) {
		b.say("A session named %s already exists.", newName)
		return
	}
	if err := b.tmux.Rename(name, newName); err != nil {
		b.say("Could not rename: %v", err)
		return
	}
	b.active = newName
	if s, ok := b.seen[name]; ok {
		b.seen[newName] = s
		delete(b.seen, name)
	}
	b.say("Renamed %s to %s.", name, newName)
}
