package screen

import (
	"regexp"
	"strings"
)

// Busy signals per AI CLI, adapted from Agent Deck's tested patterns
// (github.com/asheshgoplani/agent-deck, MIT, Copyright (c) 2025 Ashesh Goplani).
// Loose single words like "Running" are left out: a false "busy" holds the
// reply back until the next progress update.

type signals struct {
	text []string // matched case-insensitively
	re   []*regexp.Regexp
}

// genericBusy is shown by most AI CLIs while they work.
var genericBusy = []string{"esc to interrupt", "ctrl+c to interrupt", "esc to cancel"}

var brailleSpinner = regexp.MustCompile(`(?m)^\s*[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]\s`)

var toolSignals = map[string]signals{
	// Spinner plus ellipsis, like "✻ Churning… (5s)"; the done line has no "…".
	"claude": {re: []*regexp.Regexp{regexp.MustCompile(`(?m)^[✳✽✶✻✢·]\s*.+…`)}},
	"opencode": {text: []string{"esc interrupt", "esc to exit", "thinking...", "generating...",
		"building tool call...", "waiting for tool response..."}},
	"copilot": {re: []*regexp.Regexp{brailleSpinner}},
	"pi": {re: []*regexp.Regexp{brailleSpinner,
		regexp.MustCompile(`(?m)^\[(subagent|running)\]`)}},
	"codewhale": {text: []string{"waiting for deepseek", "working (", "idle timeout"}},
	"openclaw":  {text: []string{"[processing]", "[connecting]", "[reconnecting]"}},
}

// toolAliases maps launcher names to the tool whose screen they show.
var toolAliases = map[string]string{"teamclaude": "claude"}

// busyWindow is how many non-blank bottom lines are checked, so an old
// "esc to interrupt" in scrolled-up output does not count.
const busyWindow = 15

// Busy reports whether tool's screen says it is still working. raw is the
// uncleaned capture, since cleaning strips braille spinners.
func Busy(tool, raw string) bool {
	text := strings.Join(bottomLines(raw, busyWindow), "\n")
	lower := strings.ToLower(text)
	for _, m := range genericBusy {
		if strings.Contains(lower, m) {
			return true
		}
	}
	tool = strings.ToLower(tool)
	if t, ok := toolAliases[tool]; ok {
		tool = t
	}
	s := toolSignals[tool]
	for _, m := range s.text {
		if strings.Contains(lower, m) {
			return true
		}
	}
	for _, re := range s.re {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

func bottomLines(raw string, n int) []string {
	lines := strings.Split(raw, "\n")
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			out = append(out, lines[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
