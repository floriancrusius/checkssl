package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// writeFile creates path (with parents) and returns it.
func writeFile(t *testing.T, path, contents string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// domainsOf flattens the entries slice into just the domain names,
// preserving order — handy for comparing against the pre-config-package
// behaviour that only returned []string.
func domainsOf(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Domain
	}
	return out
}

func TestReadFile_PlainFile(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, filepath.Join(dir, "domains.txt"), `
# a comment
example.com
test.org # inline comment

not-a-domain
`)

	var errs []string
	entries := ReadFile(f, func(s string) { errs = append(errs, s) })
	got := strings.Join(domainsOf(entries), ",")
	if got != "example.com,test.org" {
		t.Errorf("domains = %q, want example.com,test.org", got)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "not-a-domain") {
		t.Errorf("errors = %v", errs)
	}
}

func TestReadFile_SourceAndLineTracked(t *testing.T) {
	dir := t.TempDir()
	// Deliberately put entries on specific lines (2, 4) with blanks and
	// comments between — the parser must still land on the real 1-indexed
	// line of each domain for `e`/edit to jump correctly.
	f := writeFile(t, filepath.Join(dir, "d.txt"),
		"# header\nexample.com\n\ntest.org\n")

	var errs []string
	entries := ReadFile(f, func(s string) { errs = append(errs, s) })
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	abs, _ := filepath.Abs(f)
	cases := []struct {
		domain string
		line   int
	}{
		{"example.com", 2},
		{"test.org", 4},
	}
	for i, c := range cases {
		if entries[i].Domain != c.domain {
			t.Errorf("entry %d domain = %q, want %q", i, entries[i].Domain, c.domain)
		}
		if entries[i].File != abs {
			t.Errorf("entry %d file = %q, want %q", i, entries[i].File, abs)
		}
		if entries[i].Line != c.line {
			t.Errorf("entry %d line = %d, want %d", i, entries[i].Line, c.line)
		}
	}
}

func TestReadFile_IncludeAbsolute(t *testing.T) {
	dir := t.TempDir()
	sub := writeFile(t, filepath.Join(dir, "sub.txt"), "included.com\n")
	main := writeFile(t, filepath.Join(dir, "main.txt"), "top.com\n@include "+sub+"\n")

	var errs []string
	entries := ReadFile(main, func(s string) { errs = append(errs, s) })
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got := domainsOf(entries)
	if len(got) != 2 || got[0] != "top.com" || got[1] != "included.com" {
		t.Errorf("domains = %v", got)
	}
	// Included entry must carry the sub-file as its source, not the parent.
	absSub, _ := filepath.Abs(sub)
	if entries[1].File != absSub {
		t.Errorf("included entry file = %q, want %q", entries[1].File, absSub)
	}
}

func TestReadFile_IncludeRelative(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.txt"), "included.com\n")
	main := writeFile(t, filepath.Join(dir, "main.txt"), "@include sub.txt\n")

	var errs []string
	entries := ReadFile(main, func(s string) { errs = append(errs, s) })
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(entries) != 1 || entries[0].Domain != "included.com" {
		t.Errorf("entries = %v", entries)
	}
}

func TestReadFile_IncludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "domains", "prod.list"), "prod.example\n")
	writeFile(t, filepath.Join(dir, "domains", "stage.list"), "stage.example\n")
	main := writeFile(t, filepath.Join(dir, "main.txt"), "@include domains/*.list\n")

	var errs []string
	entries := ReadFile(main, func(s string) { errs = append(errs, s) })
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got := domainsOf(entries)
	sort.Strings(got)
	want := []string{"prod.example", "stage.example"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("domains = %v, want %v", got, want)
	}
}

func TestReadFile_IncludeGlobSkipsDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "domains", "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "domains", "one.list"), "one.example\n")
	main := writeFile(t, filepath.Join(dir, "main.txt"), "@include domains/*\n")

	var errs []string
	entries := ReadFile(main, func(s string) { errs = append(errs, s) })
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	got := domainsOf(entries)
	if len(got) != 1 || got[0] != "one.example" {
		t.Errorf("domains = %v", got)
	}
}

func TestReadFile_IncludeNoMatchReportsError(t *testing.T) {
	// A concrete include path (no wildcards) that resolves to nothing
	// is a real error — the user named a specific file that isn't there.
	dir := t.TempDir()
	main := writeFile(t, filepath.Join(dir, "main.txt"), "@include missing.list\n")

	var errs []string
	entries := ReadFile(main, func(s string) { errs = append(errs, s) })
	if len(entries) != 0 {
		t.Errorf("entries = %v", entries)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "matched no files") {
		t.Errorf("errors = %v", errs)
	}
}

func TestReadFile_IncludeEmptyGlobIsSilent(t *testing.T) {
	// A wildcard glob that matches no files is NOT an error — the user
	// may just have cleaned out ~/.domains/ or hasn't created any lists
	// yet. The include directive stays, the glob simply yields nothing.
	dir := t.TempDir()
	main := writeFile(t, filepath.Join(dir, "main.txt"), "@include empty/*.list\n")

	var errs []string
	entries := ReadFile(main, func(s string) { errs = append(errs, s) })
	if len(entries) != 0 {
		t.Errorf("entries = %v", entries)
	}
	if len(errs) != 0 {
		t.Errorf("expected no errors for empty glob, got %v", errs)
	}
}

func TestReadFile_IncludeCycleDetected(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	writeFile(t, a, "one.example\n@include "+b+"\n")
	writeFile(t, b, "two.example\n@include "+a+"\n")

	var errs []string
	entries := ReadFile(a, func(s string) { errs = append(errs, s) })

	got := strings.Join(domainsOf(entries), ",")
	if got != "one.example,two.example" {
		t.Errorf("domains = %q", got)
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e, "include cycle") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an include-cycle error, got %v", errs)
	}
}

func TestExpandInclude_HomeTildeExpansion(t *testing.T) {
	// Tilde-prefixed concrete path (no wildcards) that doesn't exist →
	// still a real error, and the error message must have ~/ expanded.
	var errs []string
	out := expandInclude("~/definitely-not-a-real-checkssl-path.list", "/tmp",
		func(s string) { errs = append(errs, s) }, map[string]struct{}{})
	if len(out) != 0 {
		t.Errorf("expected empty result, got %v", out)
	}
	if len(errs) == 0 || !strings.Contains(errs[0], "matched no files") {
		t.Errorf("expected 'matched no files' error, got %v", errs)
	}
	if strings.Contains(errs[0], "~/") {
		t.Errorf("tilde should have been expanded, error still has ~/: %s", errs[0])
	}
}

func TestExpandInclude_SkipsBackupFiles(t *testing.T) {
	// Reproduce the bug the user hit: an @include glob matched
	// backup files written by older checkssl versions, so a
	// commented-out line was effectively resurrected from the backup.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real.txt"), "live.example\n")
	writeFile(t, filepath.Join(dir, "real.txt.checkssl-20261012-144038.bak"),
		"ghost.example\n")
	// Nested .bak.bak that an older session produced should also be
	// skipped — the pattern anchors on the suffix only, so no amount
	// of chaining tricks it.
	writeFile(t, filepath.Join(dir, "real.txt.checkssl-20261012-144038.bak.checkssl-20261012-150000.bak"),
		"deeper-ghost.example\n")

	entries := expandInclude(filepath.Join(dir, "*"), dir, func(string) {}, map[string]struct{}{})
	got := domainsOf(entries)
	if len(got) != 1 || got[0] != "live.example" {
		t.Errorf("expandInclude pulled %v, want only [live.example]", got)
	}
}

func TestDedupeFirst_KeepsFirstOccurrence(t *testing.T) {
	in := []Entry{
		{Domain: "a.example", File: "/first.txt", Line: 1},
		{Domain: "b.example", File: "/first.txt", Line: 2},
		{Domain: "a.example", File: "/second.txt", Line: 42}, // dup — must be dropped
	}
	out := DedupeFirst(in)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].File != "/first.txt" || out[0].Line != 1 {
		t.Errorf("first occurrence lost: %+v", out[0])
	}
}
