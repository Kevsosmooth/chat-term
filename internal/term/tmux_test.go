package term

import (
	"strings"
	"testing"
)

func TestCmdIgnoresLaunchingTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-0/bridge,1,0")
	for _, kv := range (Tmux{}).cmd("ls").Env {
		if strings.HasPrefix(kv, "TMUX=") {
			t.Fatalf("tmux command inherited %q; it would drive the launching server", kv)
		}
	}
}
