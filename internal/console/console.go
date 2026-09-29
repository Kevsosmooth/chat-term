// Package console is a local transport: stdin lines are messages, replies are
// printed. It is the quickest way to try chat-term and the smallest example of a
// chat.Transport.
package console

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"chat-term/internal/chat"
)

type Transport struct {
	In  io.Reader
	Out io.Writer
}

func (t Transport) Run(ctx context.Context, handle chat.Handler) error {
	fmt.Fprintln(t.Out, "chat-term console. Type messages as you would in the chat app (.help to start, Ctrl-D to quit).")
	reply := func(text string) { fmt.Fprintf(t.Out, "\n--- chat-term ---\n%s\n\n", text) }
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(t.In)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			handle("console", line, reply)
		}
	}
}
