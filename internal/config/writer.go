package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Writer mutates checkssl config files in place. It keeps a per-session
// record of which files have already been backed up so the first write
// to each file snapshots the original to <file>.checkssl-<timestamp>.bak
// — a safety net for the user when the TUI deletes or comments a line
// out directly, bypassing any editor's own undo.
type Writer struct {
	backedUp  map[string]string // original path → backup path
	timestamp string
}

// NewWriter returns a Writer with a fresh session timestamp. One
// Writer per TUI session is the intended pattern.
func NewWriter() *Writer {
	return &Writer{
		backedUp:  make(map[string]string),
		timestamp: time.Now().Format("20060102-150405"),
	}
}

// BackupPath returns the backup path that would be / was used for path
// in this session, or empty if the file has not been touched yet.
func (w *Writer) BackupPath(path string) string {
	return w.backedUp[path]
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
	return writeAtomic(path, []byte(strings.Join(lines, "\n")))
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
	return writeAtomic(path, []byte(strings.Join(lines, "\n")))
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

// backupSubdir is the hidden folder next to the modified file where
// per-session snapshots land. A dotfile name keeps the folder out of
// typical `~/.domains/*` globs; its contents are files, not configs,
// and are silently skipped by the parser's `IsDir` check anyway.
const backupSubdir = ".checkssl-backups"

// ensureBackup copies path to <dir>/.checkssl-backups/<basename>.<ts>.bak
// the first time it is called for path in this session. Subsequent
// calls are no-ops so the backup always reflects the pre-session state.
//
// Backups MUST NOT land next to the original: a config file at
// ~/.domains/foo and a glob of ~/.domains/* would otherwise re-read the
// backup as another config, undoing the commented-out line. The
// hidden subdir sidesteps that, and the parser skips directories
// matched by a glob, so no change to the parser is needed.
func (w *Writer) ensureBackup(path string) error {
	if _, done := w.backedUp[path]; done {
		return nil
	}
	//nolint:gosec // see mutate().
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s for backup: %w", path, err)
	}
	dir := filepath.Join(filepath.Dir(path), backupSubdir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	base := filepath.Base(path)
	backup := filepath.Join(dir, fmt.Sprintf("%s.%s.bak", base, w.timestamp))
	//nolint:gosec // backup path is derived from a tracked config path.
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		return fmt.Errorf("write backup %s: %w", backup, err)
	}
	w.backedUp[path] = backup
	return nil
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
