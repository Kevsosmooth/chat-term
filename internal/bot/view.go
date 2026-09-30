package bot

import (
	"strconv"
)

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
