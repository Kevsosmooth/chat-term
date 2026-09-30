package bot

import (
	"fmt"
	"strings"

	"chat-term/internal/guide"
)

// startSteps is the short answer to .help, and the top of the cheat sheet.
// %[1]s is the command prefix.
var startSteps = []string{
	"%[1]sprojects - list your project folders",
	"%[1]sopen 2 claude - open folder 2 and start claude (or codex, gemini, any command)",
	"Type your request as a normal message.",
	"A menu? Send its number, or use %[1]sup %[1]sdown %[1]senter",
	"%[1]sscreen - show the whole screen. %[1]sc - stop (Ctrl-C)",
	"%[1]sss - your sessions. %[1]ss 2 - switch to one",
}

// startHere lists startSteps and where to go next.
func startHere(p string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Anything that doesn't start with %s is typed into your terminal.\n\n", p)
	for i, s := range startSteps {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, strings.ReplaceAll(s, "%[1]s", p))
	}
	fmt.Fprintf(&sb, "\n%[1]shelp all - every command\n%[1]shelp open - details for one command\n%[1]sguide - a cheat sheet picture to keep", p)
	return sb.String()
}

// greet answers the first plain message a chat sends while nothing is open
// with the start-here steps instead of the shorter no-session hint.
func (b *Bot) greet(isCmd bool) bool {
	if b.greeted {
		return false
	}
	b.greeted = true
	if isCmd || b.active != "" {
		return false
	}
	b.send(fmt.Sprintf("*Welcome to chat-term.* Nothing is open yet. You're in %s.\n\n", displayPath(b.dir)) + startHere(b.cfg.Prefix))
	return true
}

func (b *Bot) cmdHelp(arg string) {
	p := b.cfg.Prefix
	switch name := strings.ToLower(strings.TrimPrefix(arg, p)); name {
	case "":
		b.send("*chat-term: start here*\n" + startHere(p))
	case "all":
		b.send(fullHelp(p))
	default:
		b.helpFor(name)
	}
}

// helpFor explains one command.
func (b *Bot) helpFor(name string) {
	p := b.cfg.Prefix
	c := lookup(name)
	if c == nil {
		b.say("No command %s%s. Send %shelp all for the list.", p, name, p)
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
}

// fullHelp is every command, grouped: the answer to .help all.
func fullHelp(p string) string {
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
	fmt.Fprintf(&sb, "\n%shelp <command> explains one command, with examples.\n", p)
	fmt.Fprintf(&sb, "To type text that starts with %s, begin it with %s%s.\n", p, p, p)
	sb.WriteString("In a plain shell, phone autocorrect is undone for you: Mkdir becomes mkdir, curly quotes become straight, and a long dash becomes --.")
	return sb.String()
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

// cmdGuide sends the cheat sheet picture, or the full list where pictures
// can't be sent.
func (b *Bot) cmdGuide(string) {
	full := fullHelp(b.cfg.Prefix)
	if b.sendImage == nil {
		b.send("This chat can't show pictures, so here is the text version.\n\n" + full)
		return
	}
	b.sendMu.Lock()
	err := b.sendImage(guide.PNG, "chat-term cheat sheet. Save or pin it to keep it handy.")
	b.sendMu.Unlock()
	if err != nil {
		b.send(fmt.Sprintf("Couldn't send the picture (%v), so here is the text version.\n\n", err) + full)
	}
}
