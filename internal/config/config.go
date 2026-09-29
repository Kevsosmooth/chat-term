// Package config loads and validates the chat-term config file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
)

// Config holds platform-neutral settings plus one section per transport.
type Config struct {
	Transport      string `toml:"transport"`
	Prefix         string `toml:"prefix"`
	ProjectsRoot   string `toml:"projects_root"`
	TmuxSocket     string `toml:"tmux_socket"`
	Width          int    `toml:"width"`
	Height         int    `toml:"height"`
	MaxLine        int    `toml:"max_line"`
	SettleMS       int    `toml:"settle_ms"`
	PollMS         int    `toml:"poll_ms"`
	ProgressEveryS int    `toml:"progress_every_s"`
	WatchMaxS      int    `toml:"watch_max_s"`
	ChunkChars     int    `toml:"chunk_chars"`

	WhatsApp WhatsApp `toml:"whatsapp"`
}

type WhatsApp struct {
	AllowedNumbers []string `toml:"allowed_numbers"`
	StorePath      string   `toml:"store_path"`
}

func Default() Config {
	return Config{
		Transport:      "whatsapp",
		Prefix:         ".",
		ProjectsRoot:   "~",
		Width:          60,
		Height:         30,
		MaxLine:        200,
		SettleMS:       2000,
		PollMS:         500,
		ProgressEveryS: 60,
		WatchMaxS:      900,
		ChunkChars:     3500,
		WhatsApp:       WhatsApp{StorePath: "~/.local/share/chat-term/store.db"},
	}
}

func DefaultPath() string {
	return ExpandHome("~/.config/chat-term/config.toml")
}

// Load reads path over the defaults. A missing file yields the defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, &cfg); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	cfg.ProjectsRoot = ExpandHome(cfg.ProjectsRoot)
	cfg.WhatsApp.StorePath = ExpandHome(cfg.WhatsApp.StorePath)
	return cfg, cfg.Validate()
}

func (c *Config) Validate() error {
	if c.Prefix == "" || c.Prefix == "/" || strings.ContainsFunc(c.Prefix, unicode.IsSpace) {
		return fmt.Errorf("prefix %q is not allowed (empty, whitespace, or / which AI CLIs use)", c.Prefix)
	}
	for i, n := range c.WhatsApp.AllowedNumbers {
		d := NormalizeNumber(n)
		if len(d) < 7 || len(d) > 15 {
			return fmt.Errorf("whatsapp.allowed_numbers[%d] %q is not a phone number with country code", i, n)
		}
		c.WhatsApp.AllowedNumbers[i] = d
	}
	if c.Width < 20 || c.Height < 10 {
		return fmt.Errorf("width must be >= 20 and height >= 10")
	}
	if c.SettleMS < 100 || c.PollMS < 50 || c.PollMS > c.SettleMS {
		return fmt.Errorf("need settle_ms >= 100 and 50 <= poll_ms <= settle_ms")
	}
	if c.ChunkChars < 500 || c.ChunkChars > 60000 {
		return fmt.Errorf("chunk_chars must be between 500 and 60000")
	}
	if c.WatchMaxS < 10 || c.ProgressEveryS < 0 || c.MaxLine < 0 {
		return fmt.Errorf("need watch_max_s >= 10, progress_every_s >= 0, max_line >= 0")
	}
	return nil
}

// NormalizeNumber keeps only digits, so "+1 (555) 010-2000" becomes "15550102000".
func NormalizeNumber(n string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, n)
}

func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}
