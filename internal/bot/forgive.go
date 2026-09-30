package bot

import (
	"os/exec"
	"sort"
	"strings"
	"unicode"
)

// Phones "improve" what you type: curly quotes, "--" turned into a dash,
// "..." into an ellipsis, a capital first letter. In a shell each of those
// breaks the command, so they are undone before typing into one.
var smartPunct = strings.NewReplacer(
	"‘", "'", "’", "'", "“", `"`, "”", `"`, "…", "...",
)

// fixDashes turns a dash that starts a word into "--", where the phone
// replaced a typed "--" ("—verbose", a lone "—"). Dashes elsewhere may be
// meant, as in sed 's/—/-/'.
func fixDashes(s string) string {
	r := []rune(s)
	var b strings.Builder
	for i, c := range r {
		if (c == '—' || c == '–') && (i == 0 || unicode.IsSpace(r[i-1])) &&
			(i == len(r)-1 || unicode.IsSpace(r[i+1]) || unicode.IsLetter(r[i+1])) {
			b.WriteString("--")
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

var shellBuiltins = map[string]bool{
	"cd": true, "export": true, "source": true, "alias": true, "unset": true,
	"exit": true, "history": true, "type": true, "echo": true, "pwd": true,
	"set": true, "eval": true, "exec": true, "jobs": true, "fg": true, "bg": true,
	"kill": true, "read": true, "test": true, "umask": true, "wait": true,
	"help": true, "for": true, "if": true, "while": true, "until": true, "case": true,
	"time": true, "ulimit": true, "declare": true, "local": true, "printf": true,
	"command": true, "hash": true, "pushd": true, "popd": true, "dirs": true,
	"disown": true, "trap": true, "true": true, "false": true, "logout": true,
}

// fixShellLine undoes phone autocorrect in text typed into a shell. Case is
// only fixed in one-line messages: a pasted block (heredoc, commit message)
// may start lines with capitals on purpose.
func fixShellLine(text string) string {
	text = fixDashes(smartPunct.Replace(text))
	if strings.Contains(text, "\n") {
		return text
	}
	return lowerFirstCommand(dropKeyboardPeriod(text))
}

// dropKeyboardPeriod turns "Ls." into "Ls" when the message is one word
// naming a command, since keyboards add a period after a sentence.
func dropKeyboardPeriod(line string) string {
	word := strings.TrimSpace(line)
	cut, ok := strings.CutSuffix(word, ".")
	if !ok || strings.ContainsAny(word, " \t") || strings.HasSuffix(cut, ".") || !isCommand(strings.ToLower(cut)) {
		return line
	}
	return cut
}

// lowerFirstCommand turns "Mkdir x" into "mkdir x", but only when "mkdir" is a
// command and "Mkdir" is not, so real capitalized names are left alone.
func lowerFirstCommand(line string) string {
	body := strings.TrimLeft(line, " \t")
	lead := line[:len(line)-len(body)]
	word, _, _ := strings.Cut(body, " ")
	lower := strings.ToLower(word)
	if word == lower || !isCommand(lower) || isCommand(word) {
		return line
	}
	return lead + lower + body[len(word):]
}

func isCommand(name string) bool {
	if shellBuiltins[name] {
		return true
	}
	if name == "" || strings.ContainsAny(name, "/=") {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

// suggest returns the command a mistyped name most likely meant, or "".
func suggest(name string) string {
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	best, bestDist := "", 3
	for _, n := range names {
		if len(name) >= 2 && strings.HasPrefix(n, name) {
			return byName[n].name
		}
		limit := 2
		if len(name) <= 4 {
			limit = 1
		}
		if d := editDistance(name, n); d <= limit && d < bestDist {
			best, bestDist = byName[n].name, d
		}
	}
	return best
}

// editDistance counts single-letter edits, a swap of neighbors ("opne") as one.
func editDistance(a, b string) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}
