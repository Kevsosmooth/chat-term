# chat-term

[![CI](https://github.com/Kevsosmooth/chat-term/actions/workflows/ci.yml/badge.svg)](https://github.com/Kevsosmooth/chat-term/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Kevsosmooth/chat-term)](https://github.com/Kevsosmooth/chat-term/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/Kevsosmooth/chat-term.svg)](https://pkg.go.dev/github.com/Kevsosmooth/chat-term)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Use your terminal from a chat app, as if you were SSH'd in.

chat-term links to your chat account and connects the conversation to tmux.
Every message is typed into a terminal session, and you get the screen back
once it settles. Because it only drives tmux, it works with any CLI tool:
Claude Code, Codex, Gemini CLI, OpenCode, Grok, a plain shell, or sessions made
by tools that run on tmux (for example Agent Deck). It uses whatever login the
tool already has, including a subscription login, so no API keys are needed.

WhatsApp is the first transport. The chat side is a small interface, so
Telegram, Discord, Signal and others can be added
(see [Adding a transport](#adding-a-transport)).

New here? Read the [guide](docs/GUIDE.md): it shows how to start Claude on a
project, answer its prompts and check on long jobs, all from your phone.

Status: **early release (v0.1)**. Expect rough edges. It uses about 15-20 MB of RAM and ships as a
single static binary.

```
You:     .open myapp claude
chat-term: [myapp · claude] ... Claude Code welcome screen ...
You:     add a --verbose flag to the CLI
chat-term: [myapp · claude] ... Claude's answer ...
You:     .ss
chat-term: 1. myapp (claude) *active*   2. build (bash)
```

## Warning: read before using

- **This is a full remote shell.** Anyone who can send a message from an
  allowed account can run any command as the user running chat-term. Keep the
  allowlist to your own number and do not run chat-term as root unless you mean to.
- **WhatsApp does not support unofficial clients.** chat-term uses
  [whatsmeow](https://github.com/tulir/whatsmeow), which links like WhatsApp
  Web. Automated use can get a number restricted or banned. You accept that
  risk. A spare number is safest.
- The login store (`store_path`) is a logged-in device. Keep it private.
  chat-term creates it with mode 0600.

## Requirements

- Linux or macOS with `tmux` 3.0 or newer
- Go 1.26 or newer, only if you build from source
- The CLI tools you want to drive, already installed and logged in

## Install

Pick one:

- **Download a binary.** Grab the archive for your system (Linux or macOS,
  Intel or ARM) from the [releases page](https://github.com/Kevsosmooth/chat-term/releases),
  unpack it and put `chat-term` somewhere on your `PATH`.
- **With Go:**
  ```sh
  go install github.com/Kevsosmooth/chat-term/cmd/chat-term@latest
  ```
- **From source:**
  ```sh
  git clone https://github.com/Kevsosmooth/chat-term.git && cd chat-term
  go build -o bin/chat-term ./cmd/chat-term
  ```

Check it worked with `chat-term version`. The examples below use
`./bin/chat-term` from a source build; use plain `chat-term` if you installed
it another way.

## Try it locally first

The console transport needs no account. You type messages in your terminal,
and replies print as they would in the chat:

```sh
./bin/chat-term console -socket wa-demo   # -socket keeps it off your normal tmux server
.help
.new
ls
.open myproject claude
```

## Connect WhatsApp

1. Copy the config and fill in the `[whatsapp]` section:

   ```sh
   mkdir -p ~/.config/chat-term
   cp config.example.toml ~/.config/chat-term/config.toml
   chmod 600 ~/.config/chat-term/config.toml
   ```

   Put your number, with country code and digits only, in `allowed_numbers`.

2. Run it and pair:

   ```sh
   ./bin/chat-term run
   ```

   Scan the QR code in WhatsApp under **Settings > Linked devices > Link a
   device**. If you can't scan, use a code instead:
   `./bin/chat-term run -pair-phone 15551234567`, then choose
   **Link with phone number** on the phone.

3. Message it:
   - **Linked to a separate number:** message that number from an allowed
     number.
   - **Linked to your own number:** use the **Message yourself** chat. chat-term
     answers there and ignores all your other chats. Your own number must be in
     `allowed_numbers`.

Run one chat-term per account: two bridges on the same number would read each
other's replies in "Message yourself" as commands.

Pairing is saved, so later runs connect straight away. To keep it running, use
a systemd user service, or start it in its own tmux window.

## How it works

- Messages that don't start with `.` are typed into the active session and
  followed by Enter. Multi-line messages are pasted as one block.
- chat-term then watches the screen. Once it has been still for `settle_ms`, you
  get the new part as a monospace block. Lines you were already sent are
  skipped, even when a tool redraws the screen or a menu closes. A ticking
  clock or cpu meter in a status bar does not count as activity, so replies
  are not held back by it. Long-running work sends a progress update every
  `progress_every_s`.
- While an AI CLI shows that it is working, the reply waits even if the screen
  is still. The busy signals are per tool (Claude Code, Codex, Gemini CLI,
  OpenCode, Copilot, Pi and others), keyed by the program running in the
  pane. A program that is slow to draw its first screen is waited for too.
- The tool's own status bar comes along with each reply, so whatever it shows
  (model, effort, mode, context left, usage limits) is visible from the chat.
- Commands start with `.` because the AI CLIs already use `/`. To type text
  that starts with a dot, begin it with `..`.
- Each chat starts in `projects_root` and has its own active session. The
  sessions themselves are ordinary tmux sessions. You can attach to the same session from a real
  terminal (`tmux attach -t myapp`) and switch between phone and keyboard.

## Commands

Send `.help` for a short start-here list, `.help all` for every command, or
`.help <command>` for details. `.guide` sends a cheat sheet picture you can
save on your phone. For step-by-step walkthroughs, see the
[guide](docs/GUIDE.md); for a printable copy, the
[cheat sheet (PDF)](docs/cheatsheet.pdf).

| Group | Commands |
|---|---|
| Directories | `.pwd` `.ls [path\|n] [-a]` `.tree [path\|n] [depth]` `.cd <path\|n>` `.cat <file\|n> [from-to\|from-]` `.projects` `.open <n\|folder> [cmd]` |
| Sessions | `.ss` `.s <n\|name>` `.new [name] [dir] [cmd]` `.back` `.detach` `.kill [n\|name]` + `.yes` `.rename <name>` |
| Keys | `.enter .esc .tab .stab .up .down .left .right .bs .space`, `.c` (Ctrl-C), `.d` (Ctrl-D), `.z` (Ctrl-Z), `.l` (Ctrl-L), each with an optional repeat count (`.down 3`), `.key <tmux keys>`, `.raw <text>` |
| View | `.screen` `.more [lines]` |
| Other | `.help [all\|command]` `.guide` `.status` |

Numbers refer to the last list shown: `.ls` then `.cat 3`, `.projects` then
`.open 2`, `.ss` then `.s 1`. `.open` with a name works like `cd`: the folder is
relative to where you are, and if it doesn't exist you are told where you are
and how to look around.

### Forgiving input

Phones and thumbs make typos, so chat-term is lenient:

- Extra spaces and case don't matter: `.   Open  myapp` works, and so does a
  space after the dot (`. open`).
- A mistyped command gets a suggestion: `.opne` answers "Did you mean .open?".
- Folder names fall back to a case-insensitive match (`Src` finds `src`), and
  paths may contain spaces (`.cat my notes/todo.txt`) or be quoted.
- A period the keyboard adds is ignored (`.Pwd.`, `Ls.`), and a bare `yes`
  confirms a pending `.kill`.
- In a plain shell, phone autocorrect is undone before typing: curly quotes
  become straight, an em or en dash becomes `--`, `…` becomes `...`, and a
  capitalized command (`Mkdir`) is lowercased when only the lowercase one
  exists. A dash is only turned back into `--` where it starts a word, and
  case is left alone in multi-line messages. AI tools get your text unchanged.
- With no active session, typing text explains what to do next instead of
  failing silently.

### Approval and auto modes

chat-term does not change how a tool asks for permission. You can answer prompts
from the chat (`.up`, `.down`, `.enter`, `1`, `y`), switch modes with keys
(`.stab` is Shift+Tab, which cycles Claude Code's modes), or start the tool in
the mode you want:

```
.open myapp claude --permission-mode acceptEdits
.open myapp codex -a never
```

Flags that skip every approval, such as `--dangerously-skip-permissions`, also
work. Over a chat link they deserve extra care.

## Configuration

See [`config.example.toml`](config.example.toml). Settings at the top level
apply to every transport. Each transport has its own section, for example
`[whatsapp]`. The default path is `~/.config/chat-term/config.toml`; use
`-config <path>` to point elsewhere.

## Project layout

```
cmd/chat-term/      entry point: picks a transport, wires it to the bot
internal/chat/      the Transport interface (the only thing a platform implements)
internal/bot/       commands, sessions, screen watching (platform-neutral)
internal/term/      tmux wrapper
internal/screen/    screen cleanup, diffing, chunking
internal/wa/        WhatsApp transport (whatsmeow)
internal/console/   stdin/stdout transport for local testing
internal/guide/     the cheat sheet picture .guide sends (generated; see guide.go)
```

## Adding a transport

A transport only moves text. (It may also implement the optional
`chat.ImageSender` to send the `.guide` picture; without it, `.guide` sends
text.) It implements one interface from
`internal/chat`:

```go
type Transport interface {
    Run(ctx context.Context, handle Handler) error
}
type Handler func(chatID, text string, reply Reply)
```

Its `Run` must:

1. **Allowlist first.** Drop messages from users who are not allowed before
   calling `handle`. The bot trusts every message it receives.
2. **Keep order.** Call `handle` for any one chat in message order, one message
   at a time (`handle` may block while it watches the screen).
3. **No echoes.** Never pass the bot's own replies back to `handle`.
   Also skip messages sent before the transport started and edits of old
   messages, so nothing is re-run by accident.
4. Give `handle` a stable `chatID` for each conversation, and a `reply` that
   sends text back to that conversation. Replies arrive as Markdown-style text
   (`*bold*`, fenced code blocks) of at most `chunk_chars` characters.
   Convert it if the platform formats text differently.

Then add a config section (like `config.WhatsApp`) and a `case` in
`cmd/chat-term/main.go`. `internal/console/console.go` is the smallest complete
example; `internal/wa/wa.go` shows a real platform, including self-chat
handling. Good candidates are Telegram (bot API, simple), Discord (bot plus a
private channel), Signal (signal-cli), Matrix and Slack.

## Tests

```sh
go test ./...
```

The bot tests run a real tmux server on a private socket and never touch your
sessions.

## Contributing and security

New transports, fixes and docs are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md).
Please report security problems privately as described in [SECURITY.md](SECURITY.md).

## Roadmap

See [ROADMAP.md](ROADMAP.md).

## License

MIT. See [LICENSE](LICENSE).

The per-tool busy signals in `internal/screen/busy.go` are adapted from
[Agent Deck](https://github.com/asheshgoplani/agent-deck) (MIT, Copyright (c)
2025 Ashesh Goplani).
