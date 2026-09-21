package main

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

func TestReadDomainsFromFile_PlainFile(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, filepath.Join(dir, "domains.txt"), `
# a comment
example.com
test.org # inline comment

not-a-domain
`)

	var errs []string
	domains := readDomainsFromFile(f, func(s string) { errs = append(errs, s) })
	got := strings.Join(domains, ",")
	if got != "example.com,test.org" {
		t.Errorf("domains = %q, want example.com,test.org", got)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "not-a-domain") {
		t.Errorf("errors = %v", errs)
	}
}

func TestReadDomainsFromFile_IncludeAbsolute(t *testing.T) {
	dir := t.TempDir()
	sub := writeFile(t, filepath.Join(dir, "sub.txt"), "included.com\n")
	main := writeFile(t, filepath.Join(dir, "main.txt"), "top.com\n@include "+sub+"\n")

	var errs []string
	domains := readDomainsFromFile(main, func(s string) { errs = append(errs, s) })
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(domains) != 2 || domains[0] != "top.com" || domains[1] != "included.com" {
		t.Errorf("domains = %v", domains)
	}
}

func TestReadDomainsFromFile_IncludeRelative(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub.txt"), "included.com\n")
	main := writeFile(t, filepath.Join(dir, "main.txt"), "@include sub.txt\n")

	var errs []string
	domains := readDomainsFromFile(main, func(s string) { errs = append(errs, s) })
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(domains) != 1 || domains[0] != "included.com" {
		t.Errorf("domains = %v", domains)
	}
}

func TestReadDomainsFromFile_IncludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "domains", "prod.list"), "prod.example\n")
	writeFile(t, filepath.Join(dir, "domains", "stage.list"), "stage.example\n")
	main := writeFile(t, filepath.Join(dir, "main.txt"), "@include domains/*.list\n")

	var errs []string
	domains := readDomainsFromFile(main, func(s string) { errs = append(errs, s) })
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// filepath.Glob returns sorted-ish, we sort ours defensively.
	sort.Strings(domains)
	want := []string{"prod.example", "stage.example"}
	if strings.Join(domains, ",") != strings.Join(want, ",") {
		t.Errorf("domains = %v, want %v", domains, want)
	}
}

func TestReadDomainsFromFile_IncludeGlobSkipsDirs(t *testing.T) {
	dir := t.TempDir()
	// A dir that matches the glob but should be skipped, not errored on.
	if err := os.MkdirAll(filepath.Join(dir, "domains", "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "domains", "one.list"), "one.example\n")
	main := writeFile(t, filepath.Join(dir, "main.txt"), "@include domains/*\n")

	var errs []string
	domains := readDomainsFromFile(main, func(s string) { errs = append(errs, s) })
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(domains) != 1 || domains[0] != "one.example" {
		t.Errorf("domains = %v", domains)
	}
}

func TestReadDomainsFromFile_IncludeNoMatchReportsError(t *testing.T) {
	dir := t.TempDir()
	main := writeFile(t, filepath.Join(dir, "main.txt"), "@include missing/*.list\n")

	var errs []string
	domains := readDomainsFromFile(main, func(s string) { errs = append(errs, s) })
	if len(domains) != 0 {
		t.Errorf("domains = %v", domains)
	}
	if len(errs) != 1 || !strings.Contains(errs[0], "matched no files") {
		t.Errorf("errors = %v", errs)
	}
}

func TestReadDomainsFromFile_IncludeCycleDetected(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	writeFile(t, a, "one.example\n@include "+b+"\n")
	writeFile(t, b, "two.example\n@include "+a+"\n")

	var errs []string
	domains := readDomainsFromFile(a, func(s string) { errs = append(errs, s) })

	// We expect both domains to be present exactly once and one cycle error.
	got := strings.Join(domains, ",")
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
	// Only checks that the tilde is consumed and no error is raised when the
	// resulting glob simply matches nothing.
	var errs []string
	out := expandInclude("~/definitely-not-a-real-checkssl-path/*.list", "/tmp",
		func(s string) { errs = append(errs, s) }, map[string]struct{}{})
	if len(out) != 0 {
		t.Errorf("expected empty result, got %v", out)
	}
	if len(errs) == 0 || !strings.Contains(errs[0], "matched no files") {
		t.Errorf("expected 'matched no files' error, got %v", errs)
	}
	// If tilde-expansion had not happened, the error would still mention "~/".
	if strings.Contains(errs[0], "~/") {
		t.Errorf("tilde should have been expanded, error still has ~/: %s", errs[0])
	}
}
