package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/floriancrusius/checkssl/internal/cert"
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
