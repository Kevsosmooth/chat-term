package bot

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type command struct {
	name    string
	aliases []string
	group   string
	usage   string // arguments, shown after the name
	summary string
	detail  string // extra help for ".help <name>"; may use %[1]s for the prefix
	run     func(b *Bot, arg string)
}

var groups = []string{"Directories", "Sessions", "Keys", "View", "Other"}

var (
	commands []*command
	byName   = map[string]*command{}
)

func register(c *command) {
	commands = append(commands, c)
	for _, n := range append([]string{c.name}, c.aliases...) {
		byName[n] = c
	}
}

func lookup(name string) *command { return byName[name] }

// Key commands send one tmux key, optionally repeated: ".down 3".
var keyCommands = []struct{ name, key, summary string }{
	{"enter", "Enter", "press Enter"},
	{"esc", "Escape", "press Escape (interrupts Claude/Codex)"},
	{"tab", "Tab", "press Tab"},
	{"stab", "BTab", "press Shift+Tab (Claude: cycle modes)"},
	{"up", "Up", "arrow up"},
	{"down", "Down", "arrow down"},
	{"left", "Left", "arrow left"},
	{"right", "Right", "arrow right"},
	{"bs", "BSpace", "backspace"},
	{"space", "Space", "press Space"},
	{"c", "C-c", "Ctrl-C (stop a program)"},
	{"d", "C-d", "Ctrl-D (end input / exit a shell)"},
	{"z", "C-z", "Ctrl-Z (suspend)"},
	{"l", "C-l", "Ctrl-L (clear screen)"},
}

func init() {
	register(&command{name: "pwd", group: "Directories", summary: "show the current folder",
		run: (*Bot).cmdPwd})
	register(&command{name: "ls", group: "Directories", usage: "[path|n] [-a]", summary: "list files, numbered",
		detail: "Numbers from the list work in %[1]scd, %[1]scat, %[1]sls, %[1]stree.\n-a includes hidden files.\nExample: %[1]sls src",
		run:    (*Bot).cmdLs})
	register(&command{name: "tree", group: "Directories", usage: "[path|n] [depth]", summary: "folder tree (default depth 2)",
		run: (*Bot).cmdTree})
	register(&command{name: "cd", group: "Directories", usage: "<path|n>", summary: "change folder",
		detail: "In a shell this types cd for you. If an AI tool is running it can't change folders,\nso use %[1]sopen <path> <tool> to start a new session there.",
		run:    (*Bot).cmdCd})
	register(&command{name: "cat", group: "Directories", usage: "<file|n> [from-to]", summary: "show a file",
		detail: "Shows the first 150 lines, or a range.\nExample: %[1]scat main.go 40-90",
		run:    (*Bot).cmdCat})
	register(&command{name: "projects", aliases: []string{"p"}, group: "Directories", summary: "list project folders, numbered",
		detail: "Lists folders under projects_root from the config file.",
		run:    (*Bot).cmdProjects})
	register(&command{name: "open", aliases: []string{"o"}, group: "Directories", usage: "<n|name|path> [cmd]", summary: "new session in a project",
		detail: "Starts a session in that folder and optionally runs a tool.\nExamples:\n  %[1]sopen 3 claude\n  %[1]sopen insurance codex\n  %[1]sopen ~/notes",
		run:    (*Bot).cmdOpen})

	register(&command{name: "ss", aliases: []string{"sessions"}, group: "Sessions", summary: "list tmux sessions, numbered",
		detail: "Shows every tmux session, including Agent Deck's. > marks the active one.",
		run:    (*Bot).cmdSessions})
	register(&command{name: "s", aliases: []string{"switch"}, group: "Sessions", usage: "<n|name>", summary: "switch to a session",
		detail: "Part of a name is enough if it is unique.\nThe session you leave keeps running.\nExample: %[1]ss insurance",
		run:    (*Bot).cmdSwitch})
	register(&command{name: "new", group: "Sessions", usage: "[name] [dir] [cmd]", summary: "start a new shell session",
		detail: "Examples:\n  %[1]snew\n  %[1]snew api ~/proj/api\n  %[1]snew api ~/proj/api claude\n  %[1]snew ~/proj/api",
		run:    (*Bot).cmdNew})
	register(&command{name: "back", aliases: []string{"b"}, group: "Sessions", summary: "switch to the previous session",
		run: (*Bot).cmdBack})
	register(&command{name: "detach", group: "Sessions", summary: "stop viewing; session keeps running",
		run: (*Bot).cmdDetach})
	register(&command{name: "kill", group: "Sessions", usage: "[n|name]", summary: "end a session (asks first)",
		detail: "Ends the active session, or the one named. Confirm with %[1]syes within 60 seconds.",
		run:    (*Bot).cmdKill})
	register(&command{name: "yes", group: "Sessions", summary: "confirm a pending kill",
		run: (*Bot).cmdYes})
	register(&command{name: "rename", group: "Sessions", usage: "<new-name>", summary: "rename the active session",
		run: (*Bot).cmdRename})

	for _, k := range keyCommands {
		key := k.key
		register(&command{name: k.name, group: "Keys", usage: "[times]", summary: k.summary,
			run: func(b *Bot, arg string) { b.cmdKeyRepeat(key, arg) }})
	}
	register(&command{name: "key", group: "Keys", usage: "<keys...>", summary: "any tmux key names",
		detail: "Examples: %[1]skey C-r   %[1]skey M-Enter   %[1]skey PageUp\nNames: https://man.openbsd.org/tmux#KEY_BINDINGS",
		run:    (*Bot).cmdKey})
	register(&command{name: "raw", group: "Keys", usage: "<text>", summary: "type text without pressing Enter",
		run: (*Bot).cmdRaw})

	register(&command{name: "screen", aliases: []string{"sc"}, group: "View", summary: "show the whole current screen",
		run: (*Bot).cmdScreen})
	register(&command{name: "more", group: "View", usage: "[lines]", summary: "screen plus scrollback (default 60)",
		run: (*Bot).cmdMore})

	register(&command{name: "help", aliases: []string{"h", "?"}, group: "Other", usage: "[command]", summary: "this list, or details for one command",
		run: (*Bot).cmdHelp})
	register(&command{name: "status", group: "Other", summary: "bridge uptime, memory, active session",
		run: (*Bot).cmdStatus})
}

func (b *Bot) cmdHelp(arg string) {
	p := b.cfg.Prefix
	if arg != "" {
		name := strings.ToLower(strings.TrimPrefix(arg, p))
		c := lookup(name)
		if c == nil {
			b.say("No command %s%s. Send %shelp for the list.", p, name, p)
			return
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "%s\n%s", usageLine(p, c), c.summary)
		if len(c.aliases) > 0 {
			fmt.Fprintf(&sb, "\nShort form: %s%s", p, strings.Join(c.aliases, ", "+p))
		}
		if c.detail != "" {
			sb.WriteString("\n\n" + strings.ReplaceAll(c.detail, "%[1]s", p))
		}
		b.send(strings.TrimSpace(sb.String()))
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*chat-term* - anything without %s is typed into the active session, then Enter.\n", p)
	for _, g := range groups {
		fmt.Fprintf(&sb, "\n*%s*\n", g)
		if g == "Keys" {
			var names []string
			for _, k := range keyCommands {
				names = append(names, p+k.name)
			}
			fmt.Fprintf(&sb, "%s  (add a number to repeat: %sdown 3)\n", strings.Join(names, " "), p)
		}
		for _, c := range commands {
			if c.group != g || (g == "Keys" && isKeyCommand(c.name)) {
				continue
			}
			fmt.Fprintf(&sb, "%s - %s\n", usageLine(p, c), c.summary)
		}
	}
	fmt.Fprintf(&sb, "\n%shelp <command> for details and examples.\n", p)
	fmt.Fprintf(&sb, "Start a message with %s%s to type text beginning with %s.\n", p, p, p)
	sb.WriteString("Tip: turn off auto-capitalize, or the shell sees \"Ls\" instead of \"ls\".")
	b.send(sb.String())
}

func usageLine(prefix string, c *command) string {
	return strings.TrimSpace(prefix + c.name + " " + c.usage)
}

func isKeyCommand(name string) bool {
	for _, k := range keyCommands {
		if k.name == name {
			return true
		}
	}
	return false
}

func (b *Bot) cmdStatus(string) {
	active := b.active
	if active == "" {
		active = "none"
	}
	socket := b.tmux.Socket
	if socket == "" {
		socket = "default"
	}
	b.say("Up %s · memory %s · Go routines %d\nActive session: %s · tmux server: %s",
		time.Since(b.started).Round(time.Second), rss(), runtime.NumGoroutine(), active, socket)
}

// rss reads this process's resident memory from /proc.
func rss() string {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return "unknown"
}

// --- sessions ---

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
	b.sendBlock(fmt.Sprintf("%d sessions · %ss <n> to switch", len(list), b.cfg.Prefix), lines)
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
	s = strings.Trim(unsafeName.ReplaceAllString(s, "-"), "-")
	if len(s) > 32 {
		s = s[:32]
	}
	if s == "" {
		s = "wa"
	}
	return s
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
		if err := b.tmux.SendText(name, cmd); err == nil {
			time.Sleep(enterDelay)
			_ = b.tmux.SendKeys(name, "Enter")
		}
	}
	b.activate(name)
	b.say("Started %s in %s.", name, displayPath(dir))
	b.watch(name, nil)
}

func looksLikePath(s string) bool {
	return strings.ContainsRune(s, '/') || strings.HasPrefix(s, "~") || strings.HasPrefix(s, ".")
}

func (b *Bot) cmdNew(arg string) {
	first, rest := splitFirst(arg)
	name := ""
	if first != "" && !looksLikePath(first) {
		name, arg = sanitizeName(first), rest
	}
	dirArg, cmd := splitFirst(arg)
	dir := b.cwd()
	if dirArg != "" {
		d, err := b.resolveDir(dirArg)
		if err != nil {
			b.say("%v", err)
			return
		}
		dir = d
	}
	if name == "" {
		name = "wa"
	}
	b.startSession(b.uniqueName(name), dir, cmd)
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
	b.pendingKill, b.pendingUntil = name, time.Now().Add(60*time.Second)
	note := ""
	if strings.HasPrefix(name, "agentdeck_") {
		note = "\nThis is an Agent Deck session; Agent Deck will show it as stopped."
	}
	b.say("End session %s and everything running in it? Send %syes within 60 seconds.%s", name, b.cfg.Prefix, note)
}

func (b *Bot) cmdYes(string) {
	name := b.pendingKill
	b.pendingKill = ""
	if name == "" || time.Now().After(b.pendingUntil) {
		b.say("Nothing to confirm.")
		return
	}
	if err := b.tmux.Kill(name); err != nil {
		b.say("Could not end %s: %v", name, err)
		return
	}
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
	b.say("Renamed %s to %s.", name, newName)
}

// --- keys ---

func (b *Bot) sendKeys(keys ...string) {
	name, ok := b.requireActive()
	if !ok {
		return
	}
	base := b.snapshot(name, 0)
	if err := b.tmux.SendKeys(name, keys...); err != nil {
		b.say("Could not send keys: %v", err)
		return
	}
	b.watch(name, base)
}

func (b *Bot) cmdKeyRepeat(key, arg string) {
	times := 1
	if arg != "" {
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > 50 {
			b.say("Repeat count must be 1-50.")
			return
		}
		times = n
	}
	keys := make([]string, times)
	for i := range keys {
		keys[i] = key
	}
	b.sendKeys(keys...)
}

var keyName = regexp.MustCompile(`^[A-Za-z0-9-]{1,20}$`)

func (b *Bot) cmdKey(arg string) {
	keys := strings.Fields(arg)
	if len(keys) == 0 {
		b.say("Which keys? Example: %skey C-r", b.cfg.Prefix)
		return
	}
	for _, k := range keys {
		if !keyName.MatchString(k) {
			b.say("%q is not a tmux key name. %shelp key for examples.", k, b.cfg.Prefix)
			return
		}
	}
	b.sendKeys(keys...)
}

func (b *Bot) cmdRaw(arg string) {
	if arg == "" {
		b.say("What should I type? %sraw <text>", b.cfg.Prefix)
		return
	}
	name, ok := b.requireActive()
	if !ok {
		return
	}
	base := b.snapshot(name, 0)
	if err := b.tmux.SendText(name, arg); err != nil {
		b.say("Could not type: %v", err)
		return
	}
	b.watch(name, base)
}

// --- view ---

func (b *Bot) cmdScreen(string) {
	if name, ok := b.requireActive(); ok {
		b.showScreen(name, 0)
	}
}

func (b *Bot) cmdMore(arg string) {
	name, ok := b.requireActive()
	if !ok {
		return
	}
	n := 60
	if arg != "" {
		v, err := strconv.Atoi(arg)
		if err != nil || v < 1 || v > 2000 {
			b.say("Lines must be 1-2000.")
			return
		}
		n = v
	}
	b.sendBlock(b.header(name), b.snapshot(name, n))
}
