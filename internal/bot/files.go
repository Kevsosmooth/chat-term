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

	"chat-term/internal/config"
)

const (
	maxListEntries = 80
	maxTreeLines   = 100
	maxCatBytes    = 5 << 20
	catDefault     = 150
	catMaxRange    = 400
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

func (b *Bot) resolveDir(arg string) (string, error) {
	p, err := b.resolvePath(arg)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("%s: no such folder", displayPath(p))
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
	var target string
	for _, f := range strings.Fields(arg) {
		if f == "-a" {
			all = true
		} else {
			target = f
		}
	}
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
	fields := strings.Fields(arg)
	dir, depth := b.cwd(), 2
	if len(fields) > 0 {
		d, err := b.resolveDir(fields[0])
		if err != nil {
			b.say("%v", err)
			return
		}
		dir = d
	}
	if len(fields) > 1 {
		n, err := strconv.Atoi(fields[1])
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
	b.typeLine("cd -- " + shellQuote(dir))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (b *Bot) cmdCat(arg string) {
	fileArg, rangeArg := splitFirst(arg)
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
		t, err2 := strconv.Atoi(z)
		if !ok || err1 != nil || err2 != nil || f < 1 || t < f {
			b.say("Range looks like 40-90.")
			return
		}
		from, to = f, min(t, len(all), f+catMaxRange-1)
	}
	if from > len(all) {
		b.say("%s has only %d lines.", displayPath(p), len(all))
		return
	}
	lines := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		lines = append(lines, fmt.Sprintf("%4d %s", i, all[i-1]))
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
	target, cmd := splitFirst(arg)
	if target == "" {
		b.say("Open what? %sopen <n|name|path> [tool]. %sprojects lists projects.", b.cfg.Prefix, b.cfg.Prefix)
		return
	}
	var dir string
	if looksLikePath(target) {
		d, err := b.resolveDir(target)
		if err != nil {
			b.say("%v", err)
			return
		}
		dir = d
	} else {
		dirs, err := b.projectDirs()
		if err != nil {
			b.say("Could not read projects_root: %v", err)
			return
		}
		var name string
		if n, convErr := strconv.Atoi(target); convErr == nil {
			list := b.lastProjects
			if len(list) == 0 {
				list = dirs
			}
			if n < 1 || n > len(list) {
				b.say("No project number %d; %sprojects lists them.", n, b.cfg.Prefix)
				return
			}
			name = list[n-1]
		} else if name, err = matchName(target, dirs, "project"); err != nil {
			b.say("%v", err)
			return
		}
		dir = filepath.Join(b.cfg.ProjectsRoot, name)
	}
	b.startSession(b.uniqueName(sanitizeName(filepath.Base(dir))), dir, cmd)
}
