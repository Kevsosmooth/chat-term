package bot

import (
	"os"
	"runtime"
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
	register(&command{name: "open", aliases: []string{"o"}, group: "Directories", usage: "<n|folder> [cmd]", summary: "new session in a folder",
		detail: "Starts a session in a folder and optionally runs a tool.\nA number is from %[1]sprojects; a name is a folder where you are now, like cd.\nExamples:\n  %[1]sopen 3 claude\n  %[1]sopen my-app teamclaude run\n  %[1]sopen ~/notes",
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

	register(&command{name: "help", aliases: []string{"h", "?"}, group: "Other", usage: "[all|command]", summary: "how to get started (add all for every command)",
		run: (*Bot).cmdHelp})
	register(&command{name: "guide", group: "Other", summary: "cheat sheet picture to keep on your phone",
		run: (*Bot).cmdGuide})
	register(&command{name: "status", group: "Other", summary: "bridge uptime, memory, active session",
		run: (*Bot).cmdStatus})
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

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
