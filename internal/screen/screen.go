// Package screen turns tmux captures into phone-readable text.
package screen

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// isBox reports box-drawing and block characters that TUIs use for borders,
// and braille dots used for logos and spinners.
func isBox(r rune) bool { return (r >= 0x2500 && r <= 0x259F) || (r >= 0x2800 && r <= 0x28FF) }

func isBoxOrSpace(r rune) bool { return isBox(r) || unicode.IsSpace(r) }

// Clean strips TUI borders and trailing space, collapses blank runs, and caps
// line length at maxLine runes (0 = no cap). Indentation of normal text is kept.
func Clean(raw string, maxLine int) []string {
	raw = strings.ReplaceAll(raw, "\uFFFD", "") // glyphs tmux could not show
	raw = strings.ReplaceAll(raw, " ", " ")
	var out []string
	blank := true // drops leading blank lines
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if r, _ := utf8.DecodeRuneInString(trimmed); isBox(r) {
			line = strings.TrimFunc(line, isBoxOrSpace)
		} else {
			line = strings.TrimRightFunc(line, isBoxOrSpace)
		}
		if maxLine > 0 && utf8.RuneCountInString(line) > maxLine {
			line = string([]rune(line)[:maxLine-1]) + "…"
		}
		if line == "" {
			if !blank {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// Diff returns cur from its first line that was not already on prev, so a
// reply shows new output (plus whatever sits below it) instead of the whole screen.
func Diff(prev, cur []string) []string {
	var s Seen
	s.Add(prev)
	return s.Diff(cur)
}

// Seen remembers the lines already shown for one session, keeping each line's
// highest count on any single screen. Diffing against it skips older lines
// that come back when a menu closes or a TUI redraws, while the second copy of
// a repeated command's identical output still counts as new.
type Seen struct {
	mu     sync.Mutex
	counts map[string]int
}

// maxSeen bounds memory; past it, only the latest screen is remembered.
const maxSeen = 5000

func (s *Seen) Add(lines []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	screen := countLines(lines)
	if s.counts == nil || len(s.counts)+len(screen) > maxSeen {
		s.counts = map[string]int{}
	}
	for l, n := range screen {
		s.counts[l] = max(s.counts[l], n)
	}
}

// Diff returns cur from its first line not accounted for by what was seen.
func (s *Seen) Diff(cur []string) []string {
	s.mu.Lock()
	left := make(map[string]int, len(s.counts))
	for l, n := range s.counts {
		left[l] = n
	}
	s.mu.Unlock()
	first := true
	for i, l := range cur {
		if l == "" {
			continue
		}
		if left[l] > 0 || (first && tailOfAny(l, left)) {
			left[l]--
			first = false
			continue
		}
		return cur[i:]
	}
	return nil
}

// tailOfAny reports whether l ends a longer seen line. After scrolling, the top
// row can be the tail of a wrapped line whose start is off screen.
func tailOfAny(l string, seen map[string]int) bool {
	for k := range seen {
		if len(k) > len(l) && strings.HasSuffix(k, l) {
			return true
		}
	}
	return false
}

func countLines(lines []string) map[string]int {
	c := make(map[string]int, len(lines))
	for _, l := range lines {
		if l != "" {
			c[l]++
		}
	}
	return c
}

// statusLines is how many bottom lines may hold a tool's status bar.
const statusLines = 3

// progress matches lines like "12% (120/1000)", "3.1 MB/s" or "eta 0:02".
var progress = regexp.MustCompile(`(?i)\d+/\d+|/s\b|\beta\b`)

// OnlyDigitsChanged reports whether a and b differ only in digits in the
// bottom status lines, like a status bar's clock or countdown ticking. A
// progress line counting up is real activity.
func OnlyDigitsChanged(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == b[i] {
			continue
		}
		if i < len(a)-statusLines || maskDigits(a[i]) != maskDigits(b[i]) || progress.MatchString(a[i]) {
			return false
		}
	}
	return true
}

func maskDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return '0'
		}
		return r
	}, s)
}

func Equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Chunk splits text on line boundaries into pieces of at most max bytes.
func Chunk(text string, max int) []string {
	if max <= 0 || len(text) <= max {
		return []string{text}
	}
	var chunks []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			chunks = append(chunks, b.String())
			b.Reset()
		}
	}
	for _, line := range strings.Split(text, "\n") {
		for len(line) > max {
			cut := max
			for cut > 0 && !utf8.RuneStart(line[cut]) {
				cut--
			}
			flush()
			chunks = append(chunks, line[:cut])
			line = line[cut:]
		}
		if b.Len() > 0 && b.Len()+1+len(line) > max {
			flush()
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	flush()
	return chunks
}
