package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriter_CommentOutLine(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, filepath.Join(dir, "d.txt"),
		"# header\nexample.com\ntest.org\n")

	w := NewWriter()
	if err := w.CommentOutLine(f, 2); err != nil {
		t.Fatalf("CommentOutLine: %v", err)
	}
	got, _ := os.ReadFile(f) //nolint:gosec // test file in tempdir
	if string(got) != "# header\n# example.com\ntest.org\n" {
		t.Errorf("got %q", got)
	}
}

func TestWriter_CommentOutLineIdempotent(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, filepath.Join(dir, "d.txt"), "# already\n")
	w := NewWriter()
	before, _ := os.ReadFile(f) //nolint:gosec // test file in tempdir
	if err := w.CommentOutLine(f, 1); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(f) //nolint:gosec // test file in tempdir
	if string(before) != string(after) {
		t.Errorf("file changed on idempotent commentout: before=%q after=%q", before, after)
	}
	// And since nothing changed, no backup should have been made.
	if w.BackupPath(f) != "" {
		t.Errorf("backup created for a no-op mutation: %s", w.BackupPath(f))
	}
}

func TestWriter_DeleteLine(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, filepath.Join(dir, "d.txt"),
		"example.com\ntest.org\nother.net\n")

	w := NewWriter()
	if err := w.DeleteLine(f, 2); err != nil {
		t.Fatalf("DeleteLine: %v", err)
	}
	got, _ := os.ReadFile(f) //nolint:gosec // test file in tempdir
	if string(got) != "example.com\nother.net\n" {
		t.Errorf("got %q", got)
	}
}

func TestWriter_BackupOnFirstWriteOnly(t *testing.T) {
	dir := t.TempDir()
	original := "example.com\ntest.org\nkeep.me\n"
	f := writeFile(t, filepath.Join(dir, "d.txt"), original)

	w := NewWriter()
	if err := w.DeleteLine(f, 1); err != nil {
		t.Fatal(err)
	}
	backup := w.BackupPath(f)
	if backup == "" {
		t.Fatal("no backup recorded")
	}
	// Backup must land next to the original — the parser's isBackupFile
	// filter keeps a `~/.domains/*` glob from re-ingesting it.
	if filepath.Dir(backup) != dir {
		t.Errorf("backup went to %q, want parent dir %q", backup, dir)
	}
	data, err := os.ReadFile(backup) //nolint:gosec // test file in tempdir
	if err != nil {
		t.Fatalf("backup unreadable: %v", err)
	}
	if string(data) != original {
		t.Errorf("backup contents = %q, want original %q", data, original)
	}

	// Overwrite the backup with a sentinel to prove the second write
	// does NOT touch it — the session's snapshot stays pre-session.
	if err := os.WriteFile(backup, []byte("SENTINEL"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.CommentOutLine(f, 1); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(backup) //nolint:gosec // test file in tempdir
	if string(after) != "SENTINEL" {
		t.Errorf("second write overwrote backup — got %q", after)
	}
}

func TestWriter_WroteRecentlyFlagsSelfWrites(t *testing.T) {
	// After a successful CommentOutLine the Writer must report
	// WroteRecently(true) for both the original path AND the backup
	// path it just created — otherwise the file watcher would treat
	// one of them as an external edit and fire a redundant reload.
	dir := t.TempDir()
	f := writeFile(t, filepath.Join(dir, "d.txt"), "example.com\n")
	w := NewWriter()
	if err := w.CommentOutLine(f, 1); err != nil {
		t.Fatal(err)
	}
	if !w.WroteRecently(f, 2*time.Second) {
		t.Error("original path not flagged as recently-written")
	}
	if backup := w.BackupPath(f); backup == "" {
		t.Fatal("no backup path recorded")
	} else if !w.WroteRecently(backup, 2*time.Second) {
		t.Error("backup path not flagged as recently-written")
	}
	// And something we did NOT write must come back false, so the
	// watcher doesn't accidentally silence genuine external edits.
	if w.WroteRecently("/nope/never-touched", 2*time.Second) {
		t.Error("WroteRecently returned true for an unknown path")
	}
}

func TestWriter_BackupNextToOriginal(t *testing.T) {
	// A full round-trip: write, then re-parse via a glob that would
	// have matched the backup if it lived next to the original. The
	// commented-out domain must stay commented-out — not resurrect
	// from the backup.
	dir := t.TempDir()
	f := writeFile(t, filepath.Join(dir, "list.txt"), "a.example\nb.example\n")

	w := NewWriter()
	if err := w.CommentOutLine(f, 1); err != nil {
		t.Fatal(err)
	}
	// Simulate the user's ~/.checkssl @include ~/.domains/* glob.
	entries := expandInclude(filepath.Join(dir, "*"), dir, func(string) {}, map[string]struct{}{})
	domains := domainsOf(entries)
	if len(domains) != 1 || domains[0] != "b.example" {
		t.Errorf("expandInclude pulled %v, want only [b.example]", domains)
	}
}

func TestWriter_OutOfRangeLineReturnsError(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, filepath.Join(dir, "d.txt"), "only.one\n")
	w := NewWriter()
	if err := w.CommentOutLine(f, 42); err == nil {
		t.Error("expected an error for line past EOF")
	}
	if err := w.CommentOutLine(f, 0); err == nil {
		t.Error("expected an error for non-1-indexed line")
	}
}

func TestWriter_PreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, filepath.Join(dir, "d.txt"), "example.com\n")
	// Lock the file down tight so we notice if the atomic-rewrite path
	// loses the mode.
	if err := os.Chmod(f, 0o600); err != nil {
		t.Fatal(err)
	}
	w := NewWriter()
	if err := w.CommentOutLine(f, 1); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(f)
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode after write = %o, want 0600", got)
	}
}

// (writeFile is defined in config_test.go in the same package.)
var _ = strings.TrimSpace
