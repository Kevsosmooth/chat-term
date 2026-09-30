// Package bot turns chat messages into tmux actions and screen replies.
package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"chat-term/internal/chat"
	"chat-term/internal/config"
	"chat-term/internal/screen"
	"chat-term/internal/term"
)

// Router keeps one Bot (active session, history) per chat.
type Router struct {
	cfg  config.Config
	tmux term.Tmux
	mu   sync.Mutex
	bots map[string]*Bot

	Images chat.ImageSender // optional; set before the transport runs
}

func NewRouter(cfg config.Config, t term.Tmux) *Router {
	return &Router{cfg: cfg, tmux: t, bots: map[string]*Bot{}}
}

// Handle has the shape of chat.Handler, so any transport can feed it.
func (r *Router) Handle(chatID, text string, reply chat.Reply) {
	r.mu.Lock()
	b, ok := r.bots[chatID]
	if !ok {
		b = New(r.cfg, r.tmux, reply)
		if r.Images != nil {
			b.sendImage = func(png []byte, caption string) error { return r.Images.SendImage(chatID, png, caption) }
		}
		r.bots[chatID] = b
	}
	r.mu.Unlock()
	b.Handle(text)
}

// Close stops every background screen watcher.
func (r *Router) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range r.bots {
		b.mu.Lock()
		b.stopWatching()
		b.mu.Unlock()
	}
}

type Bot struct {
	cfg       config.Config
	tmux      term.Tmux
	reply     chat.Reply
	sendImage func(png []byte, caption string) error // nil if the chat can't show pictures
	started   time.Time

	sendMu sync.Mutex // keeps multi-chunk replies in order

	mu           sync.Mutex
	active       string
	previous     string
	dir          string // working dir when no session is active
	lastSessions []string
	lastFiles    []string
	lastProjects []string
	pendingKill  string
	pendingUntil time.Time
	stopWatch    context.CancelFunc
	watchCtx     context.Context // done once the watcher has sent its reply
	watching     string
	seen         map[string]*screen.Seen // lines already shown, per session
	greeted      bool
}

func New(cfg config.Config, t term.Tmux, reply chat.Reply) *Bot {
	return &Bot{cfg: cfg, tmux: t, reply: reply, started: time.Now(), dir: cfg.ProjectsRoot}
}

// enterDelay separates typed text from Enter; TUIs like Claude Code treat
// instant text+Enter as a paste and may not submit it.
const enterDelay = 150 * time.Millisecond

func (b *Bot) Handle(text string) {
	text = strings.TrimRight(text, "\r\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	p := b.cfg.Prefix
	rest, isCmd := strings.CutPrefix(strings.TrimLeft(text, " "), p)
	spaced := false
	if isCmd && !strings.HasPrefix(rest, p) {
		trimmed := strings.TrimLeft(rest, " ") // ". open" is a command too
		spaced = trimmed != rest
		rest = trimmed
	}
	if b.greet(isCmd) {
		return
	}
	if b.confirmsKill(text) {
		b.cmdYes("")
		return
	}
	switch {
	case isCmd && strings.HasPrefix(rest, p):
		if startsWithLetter(rest[len(p):]) {
			b.typeLine(rest) // "..text" types ".text"
		} else {
			b.typeLine(text) // "../run.sh" and "..." are typed as sent
		}
	case isCmd && (startsWithLetter(rest) || strings.HasPrefix(rest, "?")):
		name, arg := splitFirst(rest)
		if name != "?" {
			name = strings.TrimRight(name, ".!?") // keyboards add a period
		}
		cmd := lookup(strings.ToLower(name))
		if cmd == nil && spaced && (strings.ContainsAny(name, "/.") || suggest(strings.ToLower(name)) == "") {
			b.typeLine(text) // ". venv/bin/activate" sources a file
			return
		}
		if cmd == nil {
			hint := ""
			if s := suggest(strings.ToLower(name)); s != "" {
				hint = fmt.Sprintf(" Did you mean %s%s?", p, s)
			}
			b.say("Unknown command %s%s.%s Send %shelp all for the list.", p, name, hint, p)
			return
		}
		cmd.run(b, arg)
	default:
		b.typeLine(text)
	}
}

// confirmsKill reports a bare "yes" while a kill waits for .yes; phones often
// drop the dot, and typing "yes" into a shell would print y forever.
func (b *Bot) confirmsKill(text string) bool {
	if b.pendingKill == "" || time.Now().After(b.pendingUntil) {
		return false
	}
	w := strings.ToLower(strings.Trim(text, " .!?"))
	return w == "yes" || w == "y"
}

func startsWithLetter(s string) bool {
	for _, r := range s {
		return unicode.IsLetter(r)
	}
	return false
}

// splitFirst returns the first whitespace-separated word and the trimmed rest.
func splitFirst(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i:])
}

func (b *Bot) say(format string, args ...any) {
	b.send(fmt.Sprintf(format, args...))
}

func (b *Bot) send(text string) {
	b.sendMu.Lock()
	defer b.sendMu.Unlock()
	for _, c := range screen.Chunk(text, b.cfg.ChunkChars) {
		b.reply(c)
	}
}

// sendBlock sends lines as monospace blocks, header on the first message.
func (b *Bot) sendBlock(header string, lines []string) {
	body := strings.ReplaceAll(strings.Join(lines, "\n"), "```", "'''")
	if strings.TrimSpace(body) == "" {
		body = "(blank screen)"
	}
	b.sendMu.Lock()
	defer b.sendMu.Unlock()
	for i, c := range screen.Chunk(body, b.cfg.ChunkChars) {
		msg := "```\n" + c + "\n```"
		if i == 0 && header != "" {
			msg = header + "\n" + msg
		}
		b.reply(msg)
	}
}

// header describes a session as [name · dir · running program].
func (b *Bot) header(name string) string {
	s, err := b.tmux.Info(name)
	if err != nil {
		return "[" + name + "]"
	}
	return fmt.Sprintf("[%s · %s · %s]", name, shortPath(s.Path), s.Command)
}

// shortPath keeps headers phone-sized: long paths show only their last two parts.
func shortPath(p string) string {
	d := displayPath(p)
	if len([]rune(d)) <= 40 {
		return d
	}
	parts := strings.Split(strings.TrimRight(d, "/"), "/")
	if len(parts) <= 2 {
		return d
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/")
}

// requireActive returns the active session, or explains why there is none.
func (b *Bot) requireActive() (string, bool) {
	p := b.cfg.Prefix
	if b.active == "" {
		b.say("No active session, so there is nothing to type into yet. You are in %s.\n"+
			"%snew - open a terminal here\n"+
			"%sprojects - pick a project, then %sopen <n> claude\n"+
			"%sss - see running sessions\n"+
			"%shelp - how to use chat-term", displayPath(b.dir), p, p, p, p, p)
		return "", false
	}
	if !b.tmux.Exists(b.active) {
		ended := b.active
		b.active = ""
		if names, err := b.sessionNames(); err == nil && len(names) > 0 {
			b.say("Session %s has ended. Still running: %s. %ss <name> switches.", ended, strings.Join(names, ", "), p)
		} else {
			b.say("Session %s has ended. %snew starts a new one.", ended, p)
		}
		return "", false
	}
	return b.active, true
}

func (b *Bot) snapshot(name string, history int) []string {
	raw, err := b.tmux.Capture(name, history)
	if err != nil {
		return nil
	}
	return screen.Clean(raw, b.cfg.MaxLine)
}

// typeLine types text and Enter, undoing phone autocorrect in a shell.
func (b *Bot) typeLine(text string) {
	name, ok := b.requireActive()
	if !ok {
		return
	}
	if s, err := b.tmux.Info(name); err == nil && shells[s.Command] {
		text = fixShellLine(text)
	}
	b.typeExact(name, text)
}

// typeExact types text and Enter unchanged.
func (b *Bot) typeExact(name, text string) {
	base := b.snapshot(name, 0)
	var err error
	if strings.Contains(text, "\n") {
		err = b.tmux.Paste(name, text)
	} else {
		err = b.tmux.SendText(name, text)
	}
	if err != nil {
		b.say("Could not type into %s: %v", name, err)
		return
	}
	time.Sleep(enterDelay)
	if err := b.tmux.SendKeys(name, "Enter"); err != nil {
		b.say("Could not press Enter in %s: %v", name, err)
		return
	}
	b.watch(name, base)
}

func (b *Bot) stopWatching() {
	if b.stopWatch != nil {
		b.stopWatch()
		b.stopWatch = nil
	}
}

// watch replaces any running watcher with one that reports name's screen
// changes relative to base once the screen stops changing.
func (b *Bot) watch(name string, base []string) {
	// A watcher still going for this session has not sent its output yet, so
	// base is not marked seen and that output comes with the next reply.
	interrupted := b.watchCtx != nil && b.watchCtx.Err() == nil && b.watching == name
	b.stopWatching()
	ctx, cancel := context.WithCancel(context.Background())
	b.stopWatch, b.watchCtx, b.watching = cancel, ctx, name
	if b.seen == nil {
		b.seen = map[string]*screen.Seen{}
	}
	seen := b.seen[name]
	if seen == nil {
		seen = &screen.Seen{}
		b.seen[name] = seen
	}
	if !interrupted {
		seen.Add(base)
	}
	go b.runWatch(ctx, cancel, name, base, seen)
}

// earlyLook is when a screen that never settles (top, tail -f) is first shown.
const earlyLook = 10 * time.Second

// startupMax is how long a program that has drawn nothing yet is waited for.
const startupMax = 30 * time.Second

func (b *Bot) runWatch(ctx context.Context, done context.CancelFunc, name string, base []string, seen *screen.Seen) {
	defer done()
	poll := time.Duration(b.cfg.PollMS) * time.Millisecond
	settle := time.Duration(b.cfg.SettleMS) * time.Millisecond
	progressEvery := time.Duration(b.cfg.ProgressEveryS) * time.Second
	watchMax := time.Duration(b.cfg.WatchMaxS) * time.Second

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	start := time.Now()
	lastChange, lastProgress := start, start
	shown, waited := false, false
	var last []string
	tool := ""
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			raw, err := b.tmux.Capture(name, 0)
			if err != nil {
				if ctx.Err() == nil {
					b.say("Session %s has ended.", name)
				}
				return
			}
			cur := screen.Clean(raw, b.cfg.MaxLine)
			changed := !screen.Equal(cur, last)
			if changed {
				tool = b.toolIn(name)
			}
			busy := !shells[tool] && screen.Busy(tool, raw)
			if changed {
				// A ticking clock or cpu meter in a status bar is not activity,
				// unless the tool also says it is still working.
				if !screen.OnlyDigitsChanged(cur, last) || busy {
					lastChange = now
				}
				last = cur
			}
			if ctx.Err() != nil {
				return
			}
			settled := now.Sub(lastChange) >= settle && !busy
			early := !shown && !busy && now.Sub(start) >= earlyLook
			if (settled || early) && now.Sub(start) < startupMax {
				if b.startingUp(name, cur, seen.Diff(cur)) {
					waited = true
					continue
				}
				if waited && settled {
					// The program just ended; give its last output time to land.
					waited, lastChange = false, now
					continue
				}
			}
			if settled {
				b.report(name, base, cur, seen)
				return
			}
			if progressEvery > 0 && (early || now.Sub(lastProgress) >= progressEvery) {
				lastProgress, shown = now, true
				if d := seen.Diff(cur); len(d) > 0 {
					hint := ""
					if early {
						hint = fmt.Sprintf(" (%sc stops it)", b.cfg.Prefix)
					}
					b.sendBlock(b.header(name)+" still running…"+hint, d)
					seen.Add(cur)
				}
			}
			if now.Sub(start) >= watchMax {
				b.say("Stopped watching %s after %s; it is still running. %sscreen shows it now.",
					name, watchMax, b.cfg.Prefix)
				return
			}
		}
	}
}

// report sends what changed on a settled screen.
func (b *Bot) report(name string, base, cur []string, seen *screen.Seen) {
	d := seen.Diff(cur)
	if len(d) == 0 && base != nil && !screen.Equal(cur, base) {
		// Back to an earlier screen, like a menu cursor moving back up.
		d = screen.Diff(base, cur)
	}
	if len(d) == 0 {
		b.say("%s (no change)", b.header(name))
		return
	}
	header := b.header(name)
	if len(base) > 0 && len(d) == len(cur) && len(cur) >= 15 {
		// Every line is new: the start most likely scrolled off.
		header += fmt.Sprintf(" (start scrolled off: %smore 200)", b.cfg.Prefix)
	}
	b.sendBlock(header, d)
	seen.Add(cur)
}

// toolIn names the program running in name's pane, or "" if unknown.
func (b *Bot) toolIn(name string) string {
	if s, err := b.tmux.Info(name); err == nil {
		return s.Command
	}
	return ""
}

// startingUp reports a program that has started but drawn nothing yet (a
// blank screen, or nothing below its command line), like claude or opencode
// loading, so the reply waits for its first screen.
func (b *Bot) startingUp(name string, cur, d []string) bool {
	blank := len(cur) == 0
	onlyCommand := len(d) == 1 && len(cur) > 0 && cur[len(cur)-1] == d[0]
	if !blank && !onlyCommand {
		return false
	}
	tool := b.toolIn(name)
	return tool != "" && !shells[tool]
}

func displayPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "/" {
		if p == home {
			return "~"
		}
		if rel, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
			return "~/" + rel
		}
	}
	return p
}
