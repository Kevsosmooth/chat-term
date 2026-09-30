# Contributing

Thanks for helping. Bug fixes, new chat platforms and better docs are all
welcome.

## Before you start

- For anything bigger than a small fix, open an issue first so we can agree on
  the approach.
- New platforms (Telegram, Discord, Signal, Matrix, Slack...) are the most
  useful contribution. Read [Adding a transport](README.md#adding-a-transport)
  and the [roadmap](ROADMAP.md).

## Setup

You need Go (see `go.mod` for the version) and tmux 3.0 or newer.

```sh
go build -o bin/chat-term ./cmd/chat-term
go vet ./...
go test ./...
```

The bot tests start their own tmux server on a private socket, so they never
touch your sessions. `./bin/chat-term console -socket dev` lets you try every
command without a chat account.

## Guidelines

- Keep the bot platform-neutral. Anything specific to one app lives in its own
  `internal/<name>` package and config section.
- A transport must drop messages from users who aren't allowed before the bot
  sees them. The bot trusts every message it receives.
- Keep files small and focused: one job per file, under about 500 lines.
- Add a test for the behavior you change. Run `gofmt`, `go vet` and
  `go test -race ./...` before opening a pull request.
- If you add or change a command, update the cheat sheet: run
  `go test ./internal/bot -run TestCheatsheet -update`, then render the picture
  with the commands in `internal/guide/guide.go`.

## Pull requests

Keep each one about a single change, describe what it does and how you tested
it, and link the issue if there is one.
