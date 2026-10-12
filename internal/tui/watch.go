package tui

import (
	"path/filepath"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"

	"github.com/floriancrusius/checkssl/internal/config"
)

// filesChangedMsg lands when a tracked config file was modified on disk.
// The TUI responds by triggering a full re-scan. Payload is informational
// only — the file list in opts.Entries drives what gets re-read.
type filesChangedMsg struct {
	paths []string
}

// debounceWindow is how long we wait after the last file event before
// pushing a reload. Editors like vim often emit a flurry of events
// (write temp, rename, chmod); 500ms covers that without feeling slow.
const debounceWindow = 500 * time.Millisecond

// startFileWatcher watches every parent directory of the entries' source
// files and sends a filesChangedMsg to prog whenever one of those files
// changes. Returns the watcher (so callers can Close it on quit) and an
// error if the OS watcher could not be created.
//
// We deliberately watch directories, not files, so atomic saves
// (write-tmp + rename) that editors like vim perform still trigger
// reloads — a direct file watch gets dropped on the delete half of
// the rename.
//
// writer (optional) lets the watcher distinguish our own in-TUI mutations
// from genuine external edits. Our mutations already trigger an immediate
// reload, so letting fsnotify fire a second one would make the TUI
// visibly re-render twice per keystroke.
func startFileWatcher(prog *tea.Program, entries []config.Entry, writer *config.Writer) (*fsnotify.Watcher, error) {
	tracked := make(map[string]struct{})
	dirs := make(map[string]struct{})
	for _, e := range entries {
		if e.File == "" {
			continue
		}
		tracked[e.File] = struct{}{}
		dirs[filepath.Dir(e.File)] = struct{}{}
	}
	if len(dirs) == 0 {
		// Nothing to watch (every entry is from -d). Return a nil
		// watcher — callers must tolerate that.
		return nil, nil
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for d := range dirs {
		if err := w.Add(d); err != nil {
			// Best-effort: a single unwatchable dir shouldn't kill the
			// whole feature.
			continue
		}
	}

	var (
		mu    sync.Mutex
		timer *time.Timer
		queue []string
	)
	flush := func() {
		mu.Lock()
		paths := queue
		queue = nil
		mu.Unlock()
		if len(paths) == 0 {
			return
		}
		prog.Send(filesChangedMsg{paths: paths})
	}

	go func() {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				// Only care about write/create/rename on tracked files.
				// Chmod and purely-attribute changes are ignored.
				if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				if _, interesting := tracked[ev.Name]; !interesting {
					continue
				}
				// Our own in-TUI mutations fire fsnotify too; skip them
				// so the user doesn't see a second re-render after the
				// immediate reload. The grace window covers debounce
				// plus some filesystem event lag.
				if writer != nil && writer.WroteRecently(ev.Name, debounceWindow+500*time.Millisecond) {
					continue
				}
				mu.Lock()
				queue = append(queue, ev.Name)
				if timer == nil {
					timer = time.AfterFunc(debounceWindow, flush)
				} else {
					timer.Reset(debounceWindow)
				}
				mu.Unlock()
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
				// Swallow — losing a few events is better than crashing
				// the TUI.
			}
		}
	}()
	return w, nil
}
