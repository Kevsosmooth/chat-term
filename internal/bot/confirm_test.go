package bot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenOffersToCreateFolder(t *testing.T) {
	h := newHarness(t)
	root := h.b.cfg.ProjectsRoot
	h.expect(".open fresh-app", "no folder fresh-app", "Create it and open a terminal there", ".yes")
	if _, err := os.Stat(filepath.Join(root, "fresh-app")); err == nil {
		t.Fatal("folder made before .yes")
	}
	h.expect("yes", "Started fresh-app")
	if st, err := os.Stat(filepath.Join(root, "fresh-app")); err != nil || !st.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	h.drain()

	h.expect(".open missing/deeper", "No folder")
	h.expect(".yes", "Nothing to confirm")
	if _, err := os.Stat(filepath.Join(root, "missing")); err == nil {
		t.Fatal("made a folder whose parent was missing")
	}
}
