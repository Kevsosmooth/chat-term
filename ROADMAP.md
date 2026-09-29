# Roadmap

Ideas, not promises. The prototype is done and works; everything below is open
for contributors. Open an issue before starting a large one.

## More platforms

Each is a new `chat.Transport` (see README, "Adding a transport").

- **Telegram:** bot API, long polling, allowlist by user ID. Probably the
  easiest, and there is no ban risk.
- **Discord:** bot restricted to one private channel or DMs, allowlist by user
  ID.
- **Signal:** through `signal-cli` (JSON-RPC mode), allowlist by number.
- **Matrix, Slack:** same pattern.

## Security

- **PIN lock (drafted, not built).** The bridge starts locked. `.unlock <pin>`
  unlocks it for the chat, and it locks again after N idle minutes or on
  `.lock`. The PIN is stored only as a slow hash (bcrypt or argon2) and set with
  `chat-term pin` from the terminal, never over chat. Wrong attempts are rate
  limited, and the PIN message is deleted from the chat after use where the
  platform allows it. This protects against someone picking up an unlocked
  phone.
- Read-only mode: view screens, but no typing or keys.
- An optional command allowlist for shared setups.

## Usability

- **Background alerts:** tell the chat when a session that isn't active
  finishes, or waits for approval.
- **`.watch` / `.unwatch`:** stream a session's changes without sending
  anything.
- **Per-tool cleanup:** hide spinners, status bars and repeated headers for
  Claude, Codex, Gemini and others, so replies show only the answer.
- **Files in and out:** `.get <file>` sends a file as a document; an incoming
  file is saved into the current folder.
- **Voice notes:** transcribe locally (for example whisper.cpp) and type the
  text.
- **Screenshots:** render the pane to an image for complex TUIs.

## Packaging

- Release binaries (Linux and macOS, amd64 and arm64) with GoReleaser.
- A systemd user unit and a launchd plist.
- A Docker image that talks to the host's tmux socket.
