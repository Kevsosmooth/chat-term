package wa

import (
	"fmt"
	"testing"
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
