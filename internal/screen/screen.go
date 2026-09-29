// Package screen turns tmux captures into phone-readable text.
package screen

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// isBox reports box-drawing and block characters that TUIs use for borders.
func isBox(r rune) bool { return r >= 0x2500 && r <= 0x259F }

func isBoxOrSpace(r rune) bool { return isBox(r) || unicode.IsSpace(r) }

// Clean strips TUI borders and trailing space, collapses blank runs, and caps
// line length at maxLine runes (0 = no cap). Indentation of normal text is kept.
func Clean(raw string, maxLine int) []string {
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
	seen := make(map[string]int, len(prev))
	for _, l := range prev {
		if l != "" {
			seen[l]++
		}
	}
	for i, l := range cur {
		if l == "" {
			continue
		}
		if seen[l] > 0 {
			seen[l]--
			continue
		}
		return cur[i:]
	}
	return nil
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
