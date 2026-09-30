# chat-term guide

How to get things done from your phone. For setup, see the
[README](../README.md). For a one-page summary to keep on your phone, send
`.guide` in the chat or download the [cheat sheet (PDF)](cheatsheet.pdf). New to it? Send `.help`
for the six steps that get you going.

The one rule: **a message that starts with `.` is a chat-term command;
anything else is typed into your terminal, followed by Enter.**

## Start Claude (or Codex, Gemini, OpenCode) on a project

```
.projects          list your project folders, numbered
.open 3 claude     start a session in folder 3 and launch claude
```

You get Claude's welcome screen back. Now just type, as you would in the
terminal:

```
add a --verbose flag to the CLI
```

chat-term waits until Claude has finished (it knows when Claude, Codex, Gemini,
OpenCode and others are still working) and sends you the new part of the
screen, including the tool's status bar.

Swap `claude` for `codex`, `gemini`, `opencode`, or any other command.
`.open 3` on its own starts a plain shell there.

## Answer a menu or permission prompt

When a tool asks something like:

```
Do you want to make this edit?
❯ 1. Yes
  2. Yes, and don't ask again
  3. No
```

send the number (`1`). chat-term presses Enter after it, which these menus
ignore; if a tool ever acts on that extra Enter, send `.raw 1` instead (types
without Enter). To move a cursor instead, use `.up`, `.down`, then
`.enter`. `.esc` backs out. `.stab` (Shift+Tab) cycles Claude Code's modes.

## Check on something long

Run a build, a test suite, or a long AI task and go do something else.

- If it keeps printing, you get a progress update every minute.
- `.screen` shows the whole screen right now.
- `.more` shows the screen plus the lines above it (`.more 200` for more).
- `.c` stops it (Ctrl+C).

## Work in several sessions

```
.ss              list sessions, numbered; the active one is marked
.s 2             switch to session 2 (or .s myapp)
.back            return to the previous session
.new tests       start another shell session called "tests"
.rename api      rename the active session
.kill            end the active session (asks first; reply yes)
.detach          stop viewing; everything keeps running
```

Sessions are ordinary tmux sessions. On a computer, `tmux attach -t myapp`
shows the same session, so you can move between phone and keyboard.

## Look at files without opening an editor

```
.pwd             where am I
.ls              list files, numbered
.cat 3           show file 3 from the list (or .cat notes.txt 40-80)
.cd src          change folder (or .cd 2 from the list)
.tree            folder tree
```

## Typing tips

- Autocorrect is undone for shell commands: `Ls.` runs `ls`, curly quotes
  become straight, a dash becomes `--`.
- To type text that starts with a dot, start it with two: `..hidden-file`.
- `.raw text` types without pressing Enter. `.enter` presses Enter.
- A mistyped command gets a suggestion (`.opne` answers "Did you mean .open?").
- `.help` is the short start-here list, `.help all` lists every command, and
  `.help <command>` explains one command with examples.

## When something looks wrong

| You see | What it means | Try |
| --- | --- | --- |
| `(no change)` | The screen did not change after your message. | `.screen` |
| `start scrolled off: .more 200` | The output was longer than the screen. | `.more 200` |
| `still running… (.c stops it)` | The command is still going. | wait, or `.c` |
| `Skipped messages that arrived more than 2 minutes late` | Your phone was offline; old messages were not run. | send again |
| `No active session` | Nothing is selected. | `.ss` then `.s 1`, or `.open` |
| A reply cut off mid-answer | The tool paused without looking busy. | `.screen` |
| No reply at all | The connection dropped while sending. Your message was still run. | `.screen` |
