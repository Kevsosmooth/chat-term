package bot

import (
	"regexp"
	"strconv"
	"strings"
)

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
