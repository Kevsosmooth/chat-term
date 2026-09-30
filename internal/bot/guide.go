package bot

import (
	"fmt"
	"strings"

	"github.com/Kevsosmooth/chat-term/internal/guide"
)

// guideSteps walks through starting a new project. %[1]s is the command prefix.
const guideSteps = `*Start a new project from your phone*

1. %[1]sprojects
   See the project folders you already have.
2. %[1]sopen my-app claude
   Pick a new name (or use codex, gemini...). chat-term shows where it will make the folder and asks first: send yes.
   If Claude asks whether you trust the folder, send 1.
3. Say what to build, as a normal message:
   build a landing page for my barbershop
4. Claude asks to run or change something? Send the number of your answer (1 is usually yes).
5. Taking a while? %[1]sscreen shows where it is. %[1]sc stops it.
6. Done for now? %[1]sdetach leaves it running. %[1]skill closes it.

Already have the folder? %[1]sopen 3 claude opens number 3 from %[1]sprojects.
Next time: %[1]sss lists your sessions and %[1]ss 1 jumps back in.

%[1]sguide pic - cheat sheet picture
%[1]shelp all - every command`

// cmdGuide sends the new-project walkthrough, or with "pic" the cheat sheet
// picture.
func (b *Bot) cmdGuide(arg string) {
	p := b.cfg.Prefix
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "":
		b.send(fmt.Sprintf(guideSteps, p))
	case "pic", "picture", "image", "photo":
		b.sendCheatSheet()
	default:
		b.say("%sguide - start a new project, step by step\n%sguide pic - cheat sheet picture", p, p)
	}
}

// sendCheatSheet sends the cheat sheet picture, or the full list where
// pictures can't be sent.
func (b *Bot) sendCheatSheet() {
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
