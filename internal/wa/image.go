package wa

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// SendImage implements chat.ImageSender.
func (t *Transport) SendImage(chatID string, data []byte, caption string) error {
	if t.cli == nil {
		return errors.New("not connected")
	}
	chat, err := types.ParseJID(chatID)
	if err != nil {
		return err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("read image: %w", err)
	}
	up, err := t.cli.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return t.send(chat, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		Caption:       proto.String(caption),
		Mimetype:      proto.String("image/png"),
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		Width:         proto.Uint32(uint32(cfg.Width)),
		Height:        proto.Uint32(uint32(cfg.Height)),
	}})
}
