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

	"chat-term/internal/config"
	"chat-term/internal/screen"
	"chat-term/internal/term"
)

// Reply sends one message back to the chat that owns the bot.
type Reply = func(text string)

// Router keeps one Bot (active session, history) per chat.
type Router struct {
	cfg  config.Config
	tmux term.Tmux
	mu   sync.Mutex
	bots map[string]*Bot
}

func NewRouter(cfg config.Config, t term.Tmux) *Router {
	return &Router{cfg: cfg, tmux: t, bots: map[string]*Bot{}}
}

// Handle has the shape of chat.Handler, so any transport can feed it.
func (r *Router) Handle(chatID, text string, reply Reply) {
	r.mu.Lock()
	b, ok := r.bots[chatID]
	if !ok {
		b = New(r.cfg, r.tmux, reply)
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
	cfg     config.Config
	tmux    term.Tmux
	reply   Reply
	started time.Time

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
}

func New(cfg config.Config, t term.Tmux, reply Reply) *Bot {
	home, _ := os.UserHomeDir()
	return &Bot{cfg: cfg, tmux: t, reply: reply, started: time.Now(), dir: home}
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
	rest, isCmd := strings.CutPrefix(text, p)
	switch {
	case isCmd && strings.HasPrefix(rest, p):
		b.typeLine(rest) // "..text" types ".text"
	case isCmd && startsWithLetter(rest):
		name, arg := splitFirst(rest)
		cmd := lookup(strings.ToLower(name))
		if cmd == nil {
			b.say("Unknown command %s%s. Send %shelp for the list.", p, name, p)
			return
		}
		cmd.run(b, arg)
	default:
		b.typeLine(text)
	}
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
		b.say("No active session. %sss lists sessions, %snew starts one, %shelp shows everything.", p, p, p)
		return "", false
	}
	if !b.tmux.Exists(b.active) {
		b.say("Session %s has ended. %sss lists sessions.", b.active, p)
		b.active = ""
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

func (b *Bot) typeLine(text string) {
	name, ok := b.requireActive()
	if !ok {
		return
	}
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
	b.stopWatching()
	ctx, cancel := context.WithCancel(context.Background())
	b.stopWatch = cancel
	go b.runWatch(ctx, name, base)
}

func (b *Bot) runWatch(ctx context.Context, name string, base []string) {
	poll := time.Duration(b.cfg.PollMS) * time.Millisecond
	settle := time.Duration(b.cfg.SettleMS) * time.Millisecond
	progressEvery := time.Duration(b.cfg.ProgressEveryS) * time.Second
	watchMax := time.Duration(b.cfg.WatchMaxS) * time.Second

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	start := time.Now()
	lastChange, lastProgress := start, start
	sent, last := base, base
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
			if !screen.Equal(cur, last) {
				last, lastChange = cur, now
			}
			if ctx.Err() != nil {
				return
			}
			if now.Sub(lastChange) >= settle {
				if d := screen.Diff(sent, cur); len(d) > 0 {
					b.sendBlock(b.header(name), d)
				} else {
					b.say("%s (no change)", b.header(name))
				}
				return
			}
			if progressEvery > 0 && now.Sub(lastProgress) >= progressEvery {
				lastProgress = now
				if d := screen.Diff(sent, cur); len(d) > 0 {
					b.sendBlock(b.header(name)+" still running…", d)
					sent = cur
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
