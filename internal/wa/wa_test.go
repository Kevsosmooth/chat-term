package wa

import (
	"fmt"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestIDSetEvictsOldest(t *testing.T) {
	s := newIDSet(3)
	for i := 1; i <= 4; i++ {
		s.add(fmt.Sprintf("id%d", i))
	}
	if s.has("id1") {
		t.Error("id1 should have been evicted")
	}
	for _, id := range []string{"id2", "id3", "id4"} {
		if !s.has(id) {
			t.Errorf("%s missing", id)
		}
	}
}

func TestIsMediaIgnoresProtocolMessages(t *testing.T) {
	if isMedia(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}) {
		t.Error("protocol message counted as media")
	}
	if !isMedia(&waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}) {
		t.Error("image not counted as media")
	}
}
