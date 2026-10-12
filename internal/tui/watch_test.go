package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/floriancrusius/checkssl/internal/config"
)

func TestStartFileWatcher_NoEntriesReturnsNil(t *testing.T) {
	// -d only (no file provenance): nothing to watch, so no watcher
	// should be created. Callers must tolerate that return.
	w, err := startFileWatcher(nil, []config.Entry{{Domain: "x.example"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != nil {
		_ = w.Close()
		t.Fatal("expected nil watcher when no entries carry a file")
	}
}

func TestStartFileWatcher_WatchesParentDir(t *testing.T) {
	// A watcher must come back even when the tea.Program is nil — the
	// watcher just won't be able to notify, but it must not crash on
	// init. That's the surface this test cares about.
	dir := t.TempDir()
	file := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(file, []byte("example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := []config.Entry{{Domain: "example.com", File: file, Line: 1}}
	w, err := startFileWatcher(nil, entries, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w == nil {
		t.Fatal("expected a non-nil watcher")
	}
	defer func() { _ = w.Close() }()
	// WatchList is the fsnotify-maintained set of watched paths.
	got := w.WatchList()
	if len(got) == 0 {
		t.Fatal("expected watcher to be watching at least one dir")
	}
}
