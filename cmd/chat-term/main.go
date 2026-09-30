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

	"github.com/Kevsosmooth/chat-term/internal/bot"
	"github.com/Kevsosmooth/chat-term/internal/chat"
	"github.com/Kevsosmooth/chat-term/internal/config"
	"github.com/Kevsosmooth/chat-term/internal/console"
	"github.com/Kevsosmooth/chat-term/internal/term"
	"github.com/Kevsosmooth/chat-term/internal/wa"
)

// version is set at release build time.
var version = "dev"

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
	if s, ok := transport.(chat.ImageSender); ok {
		router.Images = s
	}
	defer router.Close()
	if err := transport.Run(ctx, router.Handle); err != nil {
		log.Fatal(err)
	}
}
