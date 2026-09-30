package bot

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"chat-term/internal/config"
)

const (
	maxListEntries = 80
	maxTreeLines   = 100
	maxCatBytes    = 5 << 20
	catDefault     = 150
	catMaxRange    = 400
	catMaxBytes    = 20000 // about six chat messages per .cat
	catLineDefault = 500   // line cap when max_line is 0
)

var shells = map[string]bool{
	"bash": true, "zsh": true, "sh": true, "fish": true, "dash": true,
	"ksh": true, "tcsh": true, "csh": true, "nu": true,
}

var skipDirs = map[string]bool{"node_modules": true, ".git": true, "vendor": true, "__pycache__": true}

// cwd is the active session's folder, or the bot's own folder with no session.
func (b *Bot) cwd() string {
	if b.active != "" {
		if s, err := b.tmux.Info(b.active); err == nil && s.Path != "" {
			return s.Path
		}
	}
	return b.dir
}

// resolvePath accepts a number from the last .ls, ~, or a path relative to cwd.
func (b *Bot) resolvePath(arg string) (string, error) {
	if n, err := strconv.Atoi(arg); err == nil {
		if len(b.lastFiles) == 0 {
			return "", fmt.Errorf("no numbered list yet; %sls shows one", b.cfg.Prefix)
		}
		if n < 1 || n > len(b.lastFiles) {
			return "", fmt.Errorf("no item %d in the last %sls list", n, b.cfg.Prefix)
		}
		return b.lastFiles[n-1], nil
	}
	p := config.ExpandHome(arg)
	if !filepath.IsAbs(p) {
		p = filepath.Join(b.cwd(), p)
	}
	return filepath.Clean(p), nil
}

// splitPath separates a path that may contain spaces from the words after it.
// Quotes group a path ("my dir" 3); otherwise the longest run of leading words
// that exists wins, falling back to the first word.
func (b *Bot) splitPath(arg string) (string, string) {
	arg = strings.TrimSpace(arg)
	for _, q := range [][2]string{{`"`, `"`}, {"'", "'"}, {"“", "”"}, {"‘", "’"}} {
		if rest, ok := strings.CutPrefix(arg, q[0]); ok {
			if p, after, ok := strings.Cut(rest, q[1]); ok {
				return p, strings.TrimSpace(after)
			}
		}
	}
	words := strings.Fields(arg)
	for n := len(words); n > 1; n-- {
		p := strings.Join(words[:n], " ")
		if full, err := b.resolvePath(p); err == nil {
			if _, err := os.Stat(full); err == nil {
				return p, strings.Join(words[n:], " ")
			}
		}
	}
	return splitFirst(arg)
}

func (b *Bot) resolveDir(arg string) (string, error) {
	p, err := b.resolvePath(arg)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(p); err != nil {
		if m := matchFold(filepath.Dir(p), filepath.Base(p)); m != "" {
			return m, nil // "Src" when the folder is "src"
		}
		pp := b.cfg.Prefix
		return "", fmt.Errorf("No folder %q in %s.\n%sls - see what's here\n%scd <folder> - move somewhere else\n%sprojects - list your projects",
			filepath.Base(p), displayPath(filepath.Dir(p)), pp, pp, pp)
	} else if !st.IsDir() {
		return "", fmt.Errorf("%s is a file, not a folder", displayPath(p))
	}
	return p, nil
}

func (b *Bot) cmdPwd(string) {
	if b.active == "" {
		b.say("%s (no active session)", displayPath(b.cwd()))
		return
	}
	b.say("%s\n%s", b.header(b.active), b.cwd())
}

func (b *Bot) cmdLs(arg string) {
	all := false
	var rest []string
	for _, f := range strings.Fields(arg) {
		if f == "-a" {
			all = true
		} else {
			rest = append(rest, f)
		}
	}
	target, _ := b.splitPath(strings.Join(rest, " "))
	dir := b.cwd()
	if target != "" {
		d, err := b.resolveDir(target)
		if err != nil {
			b.say("%v", err)
			return
		}
		dir = d
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		b.say("Could not read %s: %v", displayPath(dir), err)
		return
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].IsDir() && !entries[j].IsDir() })

	var files, lines []string
	hidden := 0
	for _, e := range entries {
		if !all && strings.HasPrefix(e.Name(), ".") {
			hidden++
			continue
		}
		if len(files) == maxListEntries {
			lines = append(lines, fmt.Sprintf("… and %d more", len(entries)-hidden-maxListEntries))
			break
		}
		files = append(files, filepath.Join(dir, e.Name()))
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		lines = append(lines, fmt.Sprintf("%3d %s", len(files), name))
	}
	b.lastFiles = files
	if len(files) == 0 {
		lines = append(lines, "(empty)")
	}
	note := ""
	if hidden > 0 {
		note = fmt.Sprintf(" · %d hidden (%sls -a)", hidden, b.cfg.Prefix)
	}
	b.sendBlock(displayPath(dir)+note, lines)
}

func (b *Bot) cmdTree(arg string) {
	target, depthArg := b.splitPath(arg)
	dir, depth := b.cwd(), 2
	if target != "" {
		d, err := b.resolveDir(target)
		if err != nil {
			b.say("%v", err)
			return
		}
		dir = d
	}
	if depthArg != "" {
		n, err := strconv.Atoi(depthArg)
		if err != nil || n < 1 || n > 6 {
			b.say("Depth must be 1-6. Example: %stree . 3", b.cfg.Prefix)
			return
		}
		depth = n
	}
	var lines []string
	truncated := false
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		level := strings.Count(rel, string(filepath.Separator))
		if strings.HasPrefix(d.Name(), ".") || skipDirs[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if len(lines) == maxTreeLines {
			truncated = true
			return filepath.SkipAll
		}
		name := d.Name()
		if d.IsDir() {
			name += "/"
		}
		lines = append(lines, strings.Repeat("  ", level)+name)
		if d.IsDir() && level+1 >= depth {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		b.say("Could not read %s: %v", displayPath(dir), err)
		return
	}
	if truncated {
		lines = append(lines, "… (cut at 100 lines; pick a subfolder)")
	}
	b.sendBlock(displayPath(dir), lines)
}

func (b *Bot) cmdCd(arg string) {
	if arg == "" {
		arg = "~"
	}
	dir, err := b.resolveDir(arg)
	if err != nil {
		b.say("%v", err)
		return
	}
	if b.active == "" {
		b.dir = dir
		b.say("No active session; %snew and %sls will use %s.", b.cfg.Prefix, b.cfg.Prefix, displayPath(dir))
		return
	}
	name, ok := b.requireActive()
	if !ok {
		return
	}
	s, err := b.tmux.Info(name)
	if err != nil {
		b.say("%v", err)
		return
	}
	if !shells[s.Command] {
		b.say("%s is running in %s, so it can't change folders.\nStart a new session there instead: %sopen %s <tool>",
			s.Command, name, b.cfg.Prefix, displayPath(dir))
		return
	}
	b.typeExact(name, "cd -- "+shellQuote(dir)) // autocorrect undo would break a ’ in the name
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (b *Bot) cmdCat(arg string) {
	fileArg, rangeArg := b.splitPath(arg)
	if fileArg == "" {
		b.say("Which file? %scat <file|n> [from-to]", b.cfg.Prefix)
		return
	}
	p, err := b.resolvePath(fileArg)
	if err != nil {
		b.say("%v", err)
		return
	}
	st, err := os.Stat(p)
	switch {
	case err != nil:
		b.say("%s: no such file", displayPath(p))
		return
	case st.IsDir():
		b.say("%s is a folder; use %sls.", displayPath(p), b.cfg.Prefix)
		return
	case st.Size() > maxCatBytes:
		b.say("%s is %d MB; too big to show.", displayPath(p), st.Size()>>20)
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		b.say("Could not read %s: %v", displayPath(p), err)
		return
	}
	if bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
		b.say("%s looks like a binary file.", displayPath(p))
		return
	}
	all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	from, to := 1, min(len(all), catDefault)
	if rangeArg != "" {
		a, z, ok := strings.Cut(rangeArg, "-")
		f, err1 := strconv.Atoi(a)
		t, err2 := len(all), error(nil)
		if z != "" {
			t, err2 = strconv.Atoi(z)
		}
		if !ok || err1 != nil || err2 != nil || f < 1 || t < f {
			b.say("Range looks like 40-90, or 40- for the rest.")
			return
		}
		from, to = f, min(t, len(all), f+catMaxRange-1)
	}
	if from > len(all) {
		b.say("%s has only %d lines.", displayPath(p), len(all))
		return
	}
	maxLen := b.cfg.MaxLine
	if maxLen <= 0 {
		maxLen = catLineDefault
	}
	lines := make([]string, 0, to-from+1)
	size := 0
	for i := from; i <= to; i++ {
		line := all[i-1]
		if utf8.RuneCountInString(line) > maxLen {
			line = string([]rune(line)[:maxLen-1]) + "…"
		}
		if size += len(line) + 6; size > catMaxBytes && i > from {
			to = i - 1 // the header tells where to continue
			break
		}
		lines = append(lines, fmt.Sprintf("%4d %s", i, line))
	}
	header := fmt.Sprintf("%s · lines %d-%d of %d", displayPath(p), from, to, len(all))
	b.sendBlock(header, lines)
}

func (b *Bot) projectDirs() ([]string, error) {
	entries, err := os.ReadDir(b.cfg.ProjectsRoot)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

func (b *Bot) cmdProjects(string) {
	dirs, err := b.projectDirs()
	if err != nil {
		b.say("Could not read projects_root %s: %v", b.cfg.ProjectsRoot, err)
		return
	}
	b.lastProjects = dirs
	lines := make([]string, len(dirs))
	for i, d := range dirs {
		lines[i] = fmt.Sprintf("%3d %s", i+1, d)
	}
	if len(lines) == 0 {
		lines = []string{"(no folders)"}
	}
	b.sendBlock(fmt.Sprintf("%s · %sopen <n> [tool]", displayPath(b.cfg.ProjectsRoot), b.cfg.Prefix), lines)
}

func (b *Bot) cmdOpen(arg string) {
	target, cmd := b.splitPath(arg)
	if target == "" {
		b.say("Open what? %sopen <n|name|path> [tool]. %sprojects lists projects.", b.cfg.Prefix, b.cfg.Prefix)
		return
	}
	var dir string
	if n, convErr := strconv.Atoi(target); convErr == nil {
		// A number is from the .projects list, which says ".open <n>".
		list := b.lastProjects
		if len(list) == 0 {
			dirs, err := b.projectDirs()
			if err != nil {
				b.say("Could not read projects_root: %v", err)
				return
			}
			list = dirs
		}
		if n < 1 || n > len(list) {
			b.say("No project number %d; %sprojects lists them.", n, b.cfg.Prefix)
			return
		}
		dir = filepath.Join(b.cfg.ProjectsRoot, list[n-1])
	} else {
		// Anything else is a folder, relative to where you are, like a terminal.
		d, err := b.resolveDir(target)
		if err != nil {
			b.say("%v", err)
			return
		}
		dir = d
	}
	b.startSession(b.uniqueName(sanitizeName(filepath.Base(dir))), dir, cmd)
}

// matchFold finds the one folder in dir named name ignoring case, or "".
func matchFold(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	found := ""
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), name) {
			if found != "" {
				return ""
			}
			found = filepath.Join(dir, e.Name())
		}
	}
	return found
}
