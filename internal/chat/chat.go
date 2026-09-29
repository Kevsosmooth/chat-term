// Package chat is the contract between chat-term and a messaging platform.
//
// A transport (WhatsApp, console, and later Telegram, Discord, Signal...) only
// moves text: it receives messages from allowed users, passes them to the
// Handler, and delivers the Handler's replies back to the same chat. Everything
// about tmux, commands, and screens lives in package bot.
package chat

import "context"

// Reply sends one text message back to the chat a message came from.
type Reply = func(text string)

// Handler processes one incoming message. chatID must be stable per
// conversation; the bot keeps separate state (active session etc.) per chatID.
type Handler func(chatID, text string, reply Reply)

// Transport connects to one platform and feeds allowed messages to handle
// until ctx is cancelled. It must:
//   - drop messages from users who are not allow-listed, before calling handle;
//   - call handle for one chat in message order;
//   - never pass the bot's own replies back to handle.
type Transport interface {
	Run(ctx context.Context, handle Handler) error
}
