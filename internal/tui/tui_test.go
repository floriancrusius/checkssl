package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/floriancrusius/checkssl/internal/cert"
	"github.com/floriancrusius/checkssl/internal/config"
)

// seedResults populates a model with a handful of rows so navigation and
// filtering have something to work on without hitting the network.
func seedResults(m model) model {
	m.total = 3
	m.results = []cert.Result{
		{Domain: "a.example", Status: cert.StatusValid,
			ExpiresAt: time.Now().Add(90 * 24 * time.Hour), ResolvedIP: "1.1.1.1"},
		{Domain: "b.example", Status: cert.StatusExpiringSoon,
			ExpiresAt: time.Now().Add(10 * 24 * time.Hour), ResolvedIP: "2.2.2.2"},
		{Domain: "c.example", Status: cert.StatusExpired,
			ExpiresAt: time.Now().Add(-5 * 24 * time.Hour), ResolvedIP: "3.3.3.3"},
	}
	m.rebuildRows()
	return m
}

// pressKey sends a synthetic KeyPressMsg carrying the given rune/text and
// returns the resulting model. Mirrors what bubbletea delivers for a plain
// printable character.
func pressKey(m model, s string) model {
	// Use a Text/Code-populated Key so .String() returns `s`.
	msg := tea.KeyPressMsg(tea.Key{Text: s, Code: rune(s[0])})
	next, _ := m.Update(msg)
	return next.(model)
}

func TestNavigation_JKMovesCursor(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}
	m = pressKey(m, "j")
	if m.cursor != 1 {
		t.Fatalf("after j cursor = %d, want 1", m.cursor)
	}
	m = pressKey(m, "j")
	if m.cursor != 2 {
		t.Fatalf("after j cursor = %d, want 2", m.cursor)
	}
	// Clamp at bottom.
	m = pressKey(m, "j")
	if m.cursor != 2 {
		t.Fatalf("after j at end cursor = %d, want 2 (clamp)", m.cursor)
	}
	m = pressKey(m, "k")
	if m.cursor != 1 {
		t.Fatalf("after k cursor = %d, want 1", m.cursor)
	}
}

func TestNavigation_GandBigGJumpEnds(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	m = pressKey(m, "G")
	if m.cursor != 2 {
		t.Fatalf("after G cursor = %d, want 2", m.cursor)
	}
	m = pressKey(m, "g")
	if m.cursor != 0 {
		t.Fatalf("after g cursor = %d, want 0", m.cursor)
	}
}

func TestFilter_ChipToggleHides(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	if len(m.rows) != 3 {
		t.Fatalf("initial rows = %d, want 3", len(m.rows))
	}
	// Toggle valid off (key "1").
	m = pressKey(m, "1")
	if len(m.rows) != 2 {
		t.Fatalf("after chip 1 off rows = %d, want 2", len(m.rows))
	}
	m = pressKey(m, "1")
	if len(m.rows) != 3 {
		t.Fatalf("after chip 1 on rows = %d, want 3", len(m.rows))
	}
}

func TestSort_SCyclesColumnKeepsDirection(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	// Initial: days ascending.
	if m.sort != sortByDays {
		t.Fatalf("initial sort = %v, want days", m.sort)
	}
	// Flip to descending while on days.
	m = pressKey(m, "d")
	if !m.sortDesc {
		t.Fatal("after d expected sortDesc=true")
	}
	// Cycle to the next column — descending must persist so the user can
	// stay in 'newest first' mode across columns.
	m = pressKey(m, "s")
	if m.sort != sortByDomain {
		t.Fatalf("after s sort = %v, want domain", m.sort)
	}
	if !m.sortDesc {
		t.Fatal("after s expected sortDesc to persist (true)")
	}
	m = pressKey(m, "s")
	if m.sort != sortByIP {
		t.Fatalf("after 2x s sort = %v", m.sort)
	}
	// Reverse cycle back to domain.
	m = pressKey(m, "S")
	if m.sort != sortByDomain {
		t.Fatalf("after S sort = %v, want domain", m.sort)
	}
	// Toggle back to ascending.
	m = pressKey(m, "d")
	if m.sortDesc {
		t.Fatal("after second d expected sortDesc=false")
	}
}

func TestSort_DaysAscPutsLowestFirst(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	// Initial state is days ascending — the expired cert (negative days)
	// should land first, then the expiring-soon cert, then the valid one.
	want := []string{"c.example", "b.example", "a.example"}
	for i, d := range want {
		if m.rows[i].Domain != d {
			t.Errorf("row %d domain = %q, want %q", i, m.rows[i].Domain, d)
		}
	}
}

func TestSort_DaysDescReverses(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	m = pressKey(m, "d") // toggle to descending while still on sortByDays
	want := []string{"a.example", "b.example", "c.example"}
	for i, d := range want {
		if m.rows[i].Domain != d {
			t.Errorf("row %d domain = %q, want %q (desc)", i, m.rows[i].Domain, d)
		}
	}
}

func TestFilterMode_SlashOpensAndEscCancels(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	m = pressKey(m, "/")
	if m.mode != modeFilter {
		t.Fatalf("after / mode = %v, want filter", m.mode)
	}
	// Keys typed while in filter mode land in the buffer, not navigation.
	m = pressKey(m, "a")
	if m.filterBuf != "a" {
		t.Fatalf("after a filterBuf = %q, want %q", m.filterBuf, "a")
	}
	if m.cursor != 0 {
		t.Fatalf("cursor moved while in filter mode: %d", m.cursor)
	}
	// Esc clears and returns to normal.
	esc := tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	next, _ := m.Update(esc)
	m = next.(model)
	if m.mode != modeNormal {
		t.Fatalf("after Esc mode = %v, want normal", m.mode)
	}
	if m.filter != "" {
		t.Fatalf("after Esc filter = %q, want empty", m.filter)
	}
}

// TestCommentOut_HashKeypressRoundTrip is the full-path version of the
// bug the user hit: press `#` on a real row, verify (a) the file lands
// commented out AND (b) a subsequent re-parse through the user's actual
// Reload callback no longer surfaces the domain. This caught the fact
// that a captured value receiver was dropping the Entries overwrite.
func TestCommentOut_HashKeypressRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(cfg, []byte("keep.example\ndrop.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Simulate the main.go reparse: on each call, read the file fresh
	// and return the current entry set. Comment-out must make
	// drop.example disappear here.
	reparse := func() []config.Entry {
		return config.ReadFile(cfg, func(string) {})
	}

	opts := Options{
		Entries: reparse(),
		Reload:  reparse,
	}
	m := initialModel(opts)
	// Seed the model with results that look like a scan already ran,
	// matching the Entries we passed in — otherwise the handler would
	// have nothing to act on.
	m.results = []cert.Result{
		{Domain: "keep.example", Status: cert.StatusValid, Source: cfg, SourceLine: 1},
		{Domain: "drop.example", Status: cert.StatusValid, Source: cfg, SourceLine: 2},
	}
	m.rebuildRows()
	m.cursor = 1 // on drop.example

	// Press `#`. The handler should rewrite the file and refresh the
	// model's Entries via the Reload callback.
	m = pressKey(m, "#")

	// File must be commented out now.
	got, _ := os.ReadFile(cfg) //nolint:gosec // test file in tempdir
	if string(got) != "keep.example\n# drop.example\n" {
		t.Errorf("file after # = %q", got)
	}
	// And the re-parsed entry list must have forgotten drop.example.
	fresh := m.opts.Entries
	if len(fresh) != 1 || fresh[0].Domain != "keep.example" {
		t.Errorf("Entries after # reload = %+v, want only keep.example", fresh)
	}
}

func TestCommentOut_WritesHashAndReloads(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "d.txt")
	if err := os.WriteFile(f, []byte("a.example\nb.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := seedResults(initialModel(Options{}))
	// Point every row at line 1 of our temp file so wherever the cursor
	// lands post-sort, the mutation hits a known line.
	for i := range m.results {
		m.results[i].Source = f
		m.results[i].SourceLine = 1
	}
	m.rebuildRows()
	m.cursor = 0
	cmd := m.commentOutSelected()
	if cmd == nil {
		t.Fatal("expected a reload cmd")
	}
	got, _ := os.ReadFile(f) //nolint:gosec // test file in tempdir
	if string(got) != "# a.example\nb.example\n" {
		t.Errorf("file after comment = %q", got)
	}
}

func TestDelete_RequiresConfirmThenWrites(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "d.txt")
	if err := os.WriteFile(f, []byte("a.example\nb.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := seedResults(initialModel(Options{}))
	for i := range m.results {
		m.results[i].Source = f
		m.results[i].SourceLine = 2 // target b.example
	}
	m.rebuildRows()
	m.cursor = 0
	// `D` only arms the modal — nothing on disk yet.
	m = pressKey(m, "D")
	if m.mode != modeConfirmDelete {
		t.Fatalf("expected modeConfirmDelete, got %v", m.mode)
	}
	before, _ := os.ReadFile(f) //nolint:gosec // test file in tempdir
	if string(before) != "a.example\nb.example\n" {
		t.Errorf("file touched before confirm: %q", before)
	}
	// Cancel path first: any key other than y/Y just closes the modal.
	m = pressKey(m, "n")
	if m.mode != modeNormal {
		t.Fatal("n should have cancelled the modal")
	}
	stillUntouched, _ := os.ReadFile(f) //nolint:gosec // test file in tempdir
	if string(stillUntouched) != "a.example\nb.example\n" {
		t.Errorf("file changed on cancel: %q", stillUntouched)
	}
	// Confirm path.
	m = pressKey(m, "D")
	m = pressKey(m, "y")
	got, _ := os.ReadFile(f) //nolint:gosec // test file in tempdir
	if string(got) != "a.example\n" {
		t.Errorf("file after delete = %q, want 'a.example\\n'", got)
	}
}

func TestEdit_NoSourceReportsError(t *testing.T) {
	// Rows without a source file (e.g. a -d command-line domain) should
	// not try to launch an editor — instead the status line reports why.
	m := seedResults(initialModel(Options{}))
	// seedResults leaves Source/SourceLine as zero values, which is
	// exactly the no-source case we want.
	cmd := m.editSelected()
	if cmd != nil {
		t.Fatal("expected no cmd when selected row has no source")
	}
	if m.lastError == "" {
		t.Fatal("expected lastError to be set when editing a sourceless row")
	}
}

func TestEdit_WithSourceReturnsCommand(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	// Tag every result so wherever the cursor lands post-sort has
	// provenance.
	for i := range m.results {
		m.results[i].Source = "/tmp/some-config"
		m.results[i].SourceLine = 7
	}
	m.rebuildRows()
	m.cursor = 0
	// Guarantee $EDITOR is set to something harmless so we don't fall
	// back to vi, which may or may not be installed in CI.
	t.Setenv("EDITOR", "/bin/true")
	cmd := m.editSelected()
	if cmd == nil {
		t.Fatal("expected a cmd when selected row has a source")
	}
}

func TestNewFile_PromptAndCreate(t *testing.T) {
	// `N` enters modeNewFile; typing + Enter creates the file in
	// NewFileDir with the configured suffix appended.
	dir := t.TempDir()
	m := seedResults(initialModel(Options{
		NewFileDir:    dir,
		NewFileSuffix: ".list",
		// $EDITOR unlikely to exist, use a template that doesn't try
		// to spawn — the Resolve test suite already covers template
		// expansion; here we just need a cmd that doesn't crash.
		EditorCmd: "/bin/true {file}",
	}))
	m = pressKey(m, "N")
	if m.mode != modeNewFile {
		t.Fatalf("after N mode = %v, want modeNewFile", m.mode)
	}
	for _, r := range "lfcr.de" {
		m = pressKey(m, string(r))
	}
	if m.newBuf != "lfcr.de" {
		t.Fatalf("newBuf = %q, want lfcr.de", m.newBuf)
	}
	// Hit Enter — the file must exist with the suffix.
	enter := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	next, _ := m.Update(enter)
	m = next.(model)
	full := filepath.Join(dir, "lfcr.de.list")
	data, err := os.ReadFile(full) //nolint:gosec // test file in tempdir
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(string(data), "lfcr.de") {
		t.Errorf("header missing domain name: %q", data)
	}
}

func TestNewFile_EmptyNameIsNoop(t *testing.T) {
	dir := t.TempDir()
	m := seedResults(initialModel(Options{NewFileDir: dir, NewFileSuffix: ".list"}))
	m = pressKey(m, "N")
	// Enter with empty buffer.
	enter := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	next, _ := m.Update(enter)
	m = next.(model)
	if m.mode != modeNormal {
		t.Fatalf("after Enter mode = %v", m.mode)
	}
	// Nothing should have been created.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected empty dir, got %d entries", len(entries))
	}
}

func TestNewFile_EscCancels(t *testing.T) {
	dir := t.TempDir()
	m := seedResults(initialModel(Options{NewFileDir: dir, NewFileSuffix: ".list"}))
	m = pressKey(m, "N")
	for _, r := range "abc" {
		m = pressKey(m, string(r))
	}
	esc := tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	next, _ := m.Update(esc)
	m = next.(model)
	if m.mode != modeNormal {
		t.Errorf("after Esc mode = %v", m.mode)
	}
	if m.newBuf != "" {
		t.Errorf("newBuf = %q after cancel", m.newBuf)
	}
}

func TestNewFile_NoDirConfiguredReportsError(t *testing.T) {
	m := seedResults(initialModel(Options{})) // NewFileDir empty
	m = pressKey(m, "N")
	for _, r := range "x" {
		m = pressKey(m, string(r))
	}
	enter := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	next, _ := m.Update(enter)
	m = next.(model)
	if !strings.Contains(m.lastError, "no include-file directory") {
		t.Errorf("expected explanatory lastError, got %q", m.lastError)
	}
}

func TestDetailsMode_LOpensEscCloses(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	m = pressKey(m, "l")
	if m.mode != modeDetails {
		t.Fatalf("after l mode = %v, want details", m.mode)
	}
	// j/k still move the cursor while the panel is open so the user can
	// scrub through rows without closing it.
	before := m.cursor
	m = pressKey(m, "j")
	if m.cursor == before {
		t.Fatalf("j did not move cursor in details mode")
	}
	// Esc closes.
	esc := tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	next, _ := m.Update(esc)
	m = next.(model)
	if m.mode != modeNormal {
		t.Fatalf("after Esc mode = %v, want normal", m.mode)
	}
}

func TestDetailsMode_LOnEmptyListIsNoop(t *testing.T) {
	// A fresh model has no rows yet — pressing l should NOT enter details.
	m := initialModel(Options{})
	m = pressKey(m, "l")
	if m.mode != modeNormal {
		t.Fatalf("with no rows, l should stay in normal mode, got %v", m.mode)
	}
}

func TestQuitKeySendsQuitCmd(t *testing.T) {
	m := seedResults(initialModel(Options{}))
	msg := tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'})
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Fatal("expected quit cmd for q, got nil")
	}
	out := cmd()
	if _, ok := out.(tea.QuitMsg); !ok {
		t.Fatalf("cmd returned %T, want tea.QuitMsg", out)
	}
}
