# chat-term

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

Status: **working prototype**. It uses about 15-20 MB of RAM and ships as a
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
- Go 1.26 or newer to build it
- The CLI tools you want to drive, already installed and logged in

## Build

```sh
git clone <repo-url> chat-term && cd chat-term
go build -o bin/chat-term ./cmd/chat-term
```

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

Pairing is saved, so later runs connect straight away. To keep it running, use
a systemd user service, or start it in its own tmux window.

## How it works

- Messages that don't start with `.` are typed into the active session and
  followed by Enter. Multi-line messages are pasted as one block.
- chat-term then watches the screen. Once it has been still for `settle_ms`, you
  get the new part as a monospace block. Long-running work sends a progress
  update every `progress_every_s`.
- Commands start with `.` because the AI CLIs already use `/`. To type text
  that starts with a dot, begin it with `..`.
- Each chat has its own active session, and the sessions themselves are
  ordinary tmux sessions. You can attach to the same session from a real
  terminal (`tmux attach -t myapp`) and switch between phone and keyboard.

## Commands

Send `.help` for the list, or `.help <command>` for details.

| Group | Commands |
|---|---|
| Directories | `.pwd` `.ls [path\|n] [-a]` `.tree [path\|n] [depth]` `.cd <path\|n>` `.cat <file\|n> [from-to]` `.projects` `.open <n\|name\|path> [cmd]` |
| Sessions | `.ss` `.s <n\|name>` `.new [name] [dir] [cmd]` `.back` `.detach` `.kill [n\|name]` + `.yes` `.rename <name>` |
| Keys | `.enter .esc .tab .stab .up .down .left .right .bs .space`, `.c` (Ctrl-C), `.d` (Ctrl-D), `.z` (Ctrl-Z), `.l` (Ctrl-L), each with an optional repeat count (`.down 3`), `.key <tmux keys>`, `.raw <text>` |
| View | `.screen` `.more [lines]` |
| Other | `.help [command]` `.status` |

Numbers refer to the last list shown: `.ls` then `.cat 3`, `.projects` then
`.open 2`, `.ss` then `.s 1`.

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
```

## Adding a transport

A transport only moves text. It implements one interface from
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

## Roadmap

See [ROADMAP.md](ROADMAP.md).

## License

MIT. See [LICENSE](LICENSE).
