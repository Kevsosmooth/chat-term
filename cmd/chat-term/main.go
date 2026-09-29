// Command chat-term runs tmux terminal sessions over a chat app.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"chat-term/internal/bot"
	"chat-term/internal/chat"
	"chat-term/internal/config"
	"chat-term/internal/console"
	"chat-term/internal/term"
	"chat-term/internal/wa"
)

const usage = `chat-term: use tmux sessions from a chat app

Usage:
  chat-term run [-config path] [-pair-phone 15551234567]
      Connect with the configured transport (default whatsapp) and serve
      allowed users. WhatsApp pairs on first run.
  chat-term console [-config path] [-socket name]
      Try every command locally: type messages here, replies print below.

Config: %s (see config.example.toml)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, usage, config.DefaultPath())
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config file")
	pairPhone := fs.String("pair-phone", "", "WhatsApp: pair with a phone-number code instead of a QR (digits with country code)")
	socket := fs.String("socket", "", "tmux socket name (-L); overrides tmux_socket")
	_ = fs.Parse(os.Args[2:])

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if *socket != "" {
		cfg.TmuxSocket = *socket
	}

	var transport chat.Transport
	switch os.Args[1] {
	case "run":
		switch cfg.Transport {
		case "whatsapp":
			transport = wa.New(cfg.WhatsApp, config.NormalizeNumber(*pairPhone))
		default:
			log.Fatalf("unknown transport %q (available: whatsapp)", cfg.Transport)
		}
	case "console":
		transport = console.Transport{In: os.Stdin, Out: os.Stdout}
	default:
		fmt.Fprintf(os.Stderr, usage, config.DefaultPath())
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	router := bot.NewRouter(cfg, term.Tmux{Socket: cfg.TmuxSocket})
	defer router.Close()
	if err := transport.Run(ctx, router.Handle); err != nil {
		log.Fatal(err)
	}
}
