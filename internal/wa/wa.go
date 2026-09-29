// Package wa connects to WhatsApp as a linked device and relays text messages
// from allow-listed numbers.
package wa

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"chat-term/internal/chat"
	"chat-term/internal/config"
)

type incoming struct {
	chat types.JID
	text string
}

// Transport is the WhatsApp chat.Transport. It links as a device to an
// existing account (QR or phone code on first run).
type Transport struct {
	cfg       config.WhatsApp
	pairPhone string
}

// New builds the transport. If pairPhone is set, first-time pairing uses an
// 8-character code entered on the phone instead of a QR.
func New(cfg config.WhatsApp, pairPhone string) *Transport {
	return &Transport{cfg: cfg, pairPhone: pairPhone}
}

// Run pairs if needed, then relays messages until ctx ends.
func (t *Transport) Run(ctx context.Context, handle chat.Handler) error {
	cfg := t.cfg
	if len(cfg.AllowedNumbers) == 0 {
		return errors.New("[whatsapp] allowed_numbers is empty in the config; refusing to run an open shell")
	}
	allowed := map[string]bool{}
	for _, n := range cfg.AllowedNumbers {
		allowed[n] = true
	}

	if err := os.MkdirAll(filepath.Dir(cfg.StorePath), 0o700); err != nil {
		return err
	}
	dsn := "file:" + cfg.StorePath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	container, err := sqlstore.New(ctx, "sqlite", dsn, waLog.Stdout("DB", "WARN", true))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	if err := os.Chmod(cfg.StorePath, 0o600); err != nil {
		return err
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("load device: %w", err)
	}
	cli := whatsmeow.NewClient(device, waLog.Stdout("WA", "WARN", true))

	// Messages are handled in order on one goroutine so the event loop never blocks.
	queue := make(chan incoming, 64)
	sent := newIDSet(500)
	loggedOut := make(chan struct{}, 1)
	cli.AddEventHandler(func(evt any) {
		switch v := evt.(type) {
		case *events.Message:
			if v.Info.IsGroup || v.Info.Chat.Server == types.BroadcastServer {
				return
			}
			if v.Info.IsFromMe {
				// Own messages count only in "Message yourself", only when this
				// account's number is allowed, and never the bot's own replies.
				if sent.has(v.Info.ID) || !isSelfChat(cli, v.Info.Chat) || !allowed[ownNumber(cli)] {
					return
				}
			} else if !senderAllowed(ctx, cli, v.Info.MessageSource, allowed) {
				return
			}
			text := v.Message.GetConversation()
			if text == "" {
				text = v.Message.GetExtendedTextMessage().GetText()
			}
			if text == "" {
				text = "\x00unsupported"
			}
			select {
			case queue <- incoming{chat: v.Info.Chat, text: text}:
			default:
				log.Printf("message queue full; dropped a message")
			}
		case *events.Connected:
			log.Printf("connected to WhatsApp as %s", cli.Store.ID)
			if allowed[ownNumber(cli)] {
				log.Printf("self-chat on: send commands in your \"Message yourself\" chat")
			}
		case *events.LoggedOut:
			log.Printf("logged out by the phone; delete %s and pair again", cfg.StorePath)
			loggedOut <- struct{}{}
		}
	})

	if cli.Store.ID == nil {
		if err := pair(ctx, cli, t.pairPhone); err != nil {
			return err
		}
	} else if err := cli.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer cli.Disconnect()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-loggedOut:
			return errors.New("logged out")
		case m := <-queue:
			chat := m.chat
			reply := func(text string) {
				msg := &waE2E.Message{Conversation: proto.String(text)}
				id := cli.GenerateMessageID()
				sent.add(id) // before sending, so the echo in self-chat is never read back
				if _, err := cli.SendMessage(context.Background(), chat, msg, whatsmeow.SendRequestExtra{ID: id}); err != nil {
					log.Printf("send failed: %v", err)
				}
			}
			if m.text == "\x00unsupported" {
				reply("Only text messages are supported so far.")
				continue
			}
			handle(chat.String(), m.text, reply)
		}
	}
}

func ownNumber(cli *whatsmeow.Client) string {
	if cli.Store.ID == nil {
		return ""
	}
	return cli.Store.ID.User
}

// isSelfChat reports whether chat is this account's "Message yourself" chat,
// addressed by phone number or by privacy ID (LID).
func isSelfChat(cli *whatsmeow.Client, chat types.JID) bool {
	switch chat.Server {
	case types.DefaultUserServer:
		return chat.User == ownNumber(cli)
	case types.HiddenUserServer:
		return !cli.Store.LID.IsEmpty() && chat.User == cli.Store.LID.User
	}
	return false
}

// idSet remembers the most recent message IDs the bot sent.
type idSet struct {
	mu    sync.Mutex
	ids   map[types.MessageID]bool
	order []types.MessageID
	max   int
}

func newIDSet(max int) *idSet {
	return &idSet{ids: map[types.MessageID]bool{}, max: max}
}

func (s *idSet) add(id types.MessageID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids[id] = true
	s.order = append(s.order, id)
	if len(s.order) > s.max {
		delete(s.ids, s.order[0])
		s.order = s.order[1:]
	}
}

func (s *idSet) has(id types.MessageID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ids[id]
}

// senderAllowed checks the sender's phone number, resolving WhatsApp's
// privacy IDs (LIDs) to numbers when needed. Rejections log the number only.
func senderAllowed(ctx context.Context, cli *whatsmeow.Client, src types.MessageSource, allowed map[string]bool) bool {
	var seen string
	for _, jid := range []types.JID{src.Sender, src.SenderAlt} {
		switch jid.Server {
		case types.DefaultUserServer:
			seen = jid.User
		case types.HiddenUserServer:
			pn, err := cli.Store.LIDs.GetPNForLID(ctx, jid)
			if err != nil || pn.IsEmpty() {
				continue
			}
			seen = pn.User
		default:
			continue
		}
		if allowed[seen] {
			return true
		}
	}
	if seen == "" {
		log.Printf("ignored a message from an unresolved sender")
	} else {
		log.Printf("ignored a message from +%s (not in allowed_numbers)", seen)
	}
	return false
}

func pair(ctx context.Context, cli *whatsmeow.Client, phone string) error {
	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		return fmt.Errorf("start pairing: %w", err)
	}
	if err := cli.Connect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	codeShown := false
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			if phone == "" {
				fmt.Println("\nOn the bot phone: WhatsApp > Settings > Linked devices > Link a device, then scan:")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				continue
			}
			if codeShown {
				continue
			}
			code, err := cli.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
			if err != nil {
				return fmt.Errorf("pair by phone: %w", err)
			}
			codeShown = true
			fmt.Printf("\nOn the bot phone: WhatsApp > Linked devices > Link with phone number, enter: %s\n", code)
		case "success":
			fmt.Println("Paired.")
			return nil
		case "timeout":
			return errors.New("pairing timed out; run again")
		default:
			if evt.Error != nil {
				return fmt.Errorf("pairing failed: %w", evt.Error)
			}
		}
	}
	return errors.New("pairing ended without success")
}
