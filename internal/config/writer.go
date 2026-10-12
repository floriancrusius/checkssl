package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Writer mutates checkssl config files in place. It keeps a per-session
// record of which files have already been backed up so the first write
// to each file snapshots the original to a session folder inside the
// user's cache directory — a safety net for the user when the TUI
// deletes or comments a line out directly, bypassing any editor's undo.
type Writer struct {
	mu           sync.Mutex
	backedUp     map[string]string    // original path → backup path
	recentWrites map[string]time.Time // path → when the Writer wrote to it
	timestamp    string
	// backupRoot is where this session's backup folder lives. Each file
	// gets a mirror-path underneath — e.g. for /home/u/.config/checkssl
	// /domains it becomes <backupRoot>/<sessionTS>/domains.
	backupRoot string
}

// NewWriter returns a Writer with a fresh session timestamp and the
// given backup root — typically paths.BackupsDir(). One Writer per TUI
// session is the intended pattern. An empty backupRoot disables
// backups entirely (callers must have asked for that explicitly).
func NewWriter(backupRoot string) *Writer {
	return &Writer{
		backedUp:     make(map[string]string),
		recentWrites: make(map[string]time.Time),
		timestamp:    time.Now().Format("20060102-150405"),
		backupRoot:   backupRoot,
	}
}

// BackupPath returns the backup path that would be / was used for path
// in this session, or empty if the file has not been touched yet.
func (w *Writer) BackupPath(path string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.backedUp[path]
}

// WroteRecently reports whether this Writer wrote to path within the
// given window. Lets a file watcher distinguish our own atomic rename
// (which fires a Create event on the original) from a genuine external
// edit, so an in-TUI `#` doesn't trigger a redundant hot-reload a few
// hundred ms after the immediate one.
func (w *Writer) WroteRecently(path string, within time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.recentWrites[path]
	if !ok {
		return false
	}
	return time.Since(t) <= within
}

// markWrote records that we just finished writing path.
func (w *Writer) markWrote(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.recentWrites[path] = time.Now()
}

// CommentOutLine rewrites path so the given 1-indexed line is prefixed
// with "# ". Idempotent: a line that already starts with `#` is left
// alone. Backs up the file on the first write of the session.
func (w *Writer) CommentOutLine(path string, line int) error {
	lines, err := w.readLines(path, line)
	if err != nil {
		return err
	}
	trimmed := strings.TrimLeft(lines[line-1], " \t")
	if strings.HasPrefix(trimmed, "#") {
		return nil // already commented, nothing to do
	}
	if err := w.ensureBackup(path); err != nil {
		return err
	}
	lines[line-1] = "# " + lines[line-1]
	if err := writeAtomic(path, []byte(strings.Join(lines, "\n"))); err != nil {
		return err
	}
	w.markWrote(path)
	return nil
}

// DeleteLine removes the given 1-indexed line from path entirely. The
// resulting line count is one shorter. Backs up on first write.
func (w *Writer) DeleteLine(path string, line int) error {
	lines, err := w.readLines(path, line)
	if err != nil {
		return err
	}
	if err := w.ensureBackup(path); err != nil {
		return err
	}
	lines = append(lines[:line-1], lines[line:]...)
	if err := writeAtomic(path, []byte(strings.Join(lines, "\n"))); err != nil {
		return err
	}
	w.markWrote(path)
	return nil
}

// readLines loads path and splits it on "\n", returning an error when
// line is out of range. The trailing newline survives the round trip
// because strings.Split on content ending in "\n" produces an extra
// empty element that strings.Join reintroduces.
func (w *Writer) readLines(path string, line int) ([]string, error) {
	if line < 1 {
		return nil, fmt.Errorf("mutate %s: line %d is not 1-indexed", path, line)
	}
	//nolint:gosec // path comes from an entry the TUI knows about.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	if line > len(lines) {
		return nil, fmt.Errorf("mutate %s: line %d is past EOF (file has %d lines)", path, line, len(lines))
	}
	return lines, nil
}

// ensureBackup copies path into the session's backup folder the first
// time it is called for path. Subsequent calls are no-ops so the backup
// always reflects the pre-session state.
//
// Backups live in <backupRoot>/<sessionTS>/<mirror of absolute path>,
// which keeps them out of any @include glob the user has pointed at
// their config dir. No parser-side filter needed, and old sessions'
// backups can be pruned by date later (see PruneOlderThan).
func (w *Writer) ensureBackup(path string) error {
	w.mu.Lock()
	if _, done := w.backedUp[path]; done {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()

	if w.backupRoot == "" {
		// Backups disabled — proceed without snapshot. The caller
		// opted into this.
		w.mu.Lock()
		w.backedUp[path] = ""
		w.mu.Unlock()
		return nil
	}

	//nolint:gosec // see readLines().
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s for backup: %w", path, err)
	}
	backup, err := w.backupPath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(backup), 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(backup), err)
	}
	//nolint:gosec // backup path is derived from backupRoot + the config path.
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		return fmt.Errorf("write backup %s: %w", backup, err)
	}
	w.mu.Lock()
	w.backedUp[path] = backup
	w.mu.Unlock()
	return nil
}

// backupPath turns an original absolute path into its mirror location
// under the session backup dir. A leading separator in the mirror
// stays — the whole absolute path is a key, we just avoid letting it
// collide with another file of the same basename elsewhere.
func (w *Writer) backupPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// On Windows the drive letter looks like "C:\…" — join accepts it;
	// we just strip the colon to keep the path portable.
	safe := strings.ReplaceAll(abs, ":", "")
	return filepath.Join(w.backupRoot, w.timestamp, safe), nil
}

// PruneOlderThan deletes session folders under backupRoot whose
// timestamp is older than `max`. Called from `checkssl config gc`
// and (optionally) at TUI startup to keep the cache dir bounded.
// Returns the number of session folders removed.
func PruneOlderThan(backupRoot string, max time.Duration) (int, error) {
	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().Add(-max)
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Session folders are named "20060102-150405" (writer timestamp).
		t, err := time.Parse("20060102-150405", e.Name())
		if err != nil {
			continue // skip anything that doesn't match our naming
		}
		if t.Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(backupRoot, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}

// writeAtomic writes data to path via a tmp file + rename so a crash
// mid-write cannot leave a half-written config.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".checkssl-write-*")
	if err != nil {
		return fmt.Errorf("mktemp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	// Preserve the original mode when possible so group/world perms
	// survive the rewrite.
	if info, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmpName, info.Mode().Perm())
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}
	return nil
}
