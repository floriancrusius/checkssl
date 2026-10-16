package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/floriancrusius/checkssl/internal/config"
	"github.com/floriancrusius/checkssl/internal/settings"
)

func TestTargetFromEntries_DominantDirWins(t *testing.T) {
	entries := []config.Entry{
		{Domain: "a.example", File: "/home/u/.domains/hermine.ai", Line: 1},
		{Domain: "b.example", File: "/home/u/.domains/hermine.ai", Line: 2},
		{Domain: "c.example", File: "/home/u/.domains/zauberware.com", Line: 1},
		{Domain: "d.example", File: "/home/u/.checkssl", Line: 1}, // one direct entry
	}
	dir, suf, ok := targetFromEntries(entries)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if dir != "/home/u/.domains" {
		t.Errorf("dir = %q, want /home/u/.domains", dir)
	}
	if suf != "" {
		t.Errorf("suffix = %q, want empty (legacy files have no extension)", suf)
	}
}

func TestTargetFromEntries_DotListMajorityPinsSuffix(t *testing.T) {
	entries := []config.Entry{
		{Domain: "a.example", File: "/cfg/domains.d/customer-a.list", Line: 1},
		{Domain: "b.example", File: "/cfg/domains.d/customer-a.list", Line: 2},
		{Domain: "c.example", File: "/cfg/domains.d/customer-b.list", Line: 1},
	}
	dir, suf, ok := targetFromEntries(entries)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if dir != "/cfg/domains.d" {
		t.Errorf("dir = %q", dir)
	}
	if suf != ".list" {
		t.Errorf("suffix = %q, want .list", suf)
	}
}

func TestTargetFromEntries_NoProvenanceReturnsFalse(t *testing.T) {
	// All entries from -d → no source files → caller falls back.
	entries := []config.Entry{{Domain: "example.com"}, {Domain: "other.net"}}
	if _, _, ok := targetFromEntries(entries); ok {
		t.Error("expected ok=false when no entry has provenance")
	}
}

func TestTargetFromEntries_TopLevelConfigIgnored(t *testing.T) {
	// Entries that live directly in ~/.checkssl (or domains.list) are
	// indexes, not list files to drop neighbours next to. If those are
	// the only entries with provenance, ok must be false so the caller
	// routes to the legacy ~/.domains/ fallback instead of writing new
	// files into the user's HOME directory.
	entries := []config.Entry{
		{Domain: "verena.de", File: "/Users/u/.checkssl", Line: 1},
		{Domain: "api.verena.de", File: "/Users/u/.checkssl", Line: 2},
	}
	if _, _, ok := targetFromEntries(entries); ok {
		t.Error("expected ok=false — entries from ~/.checkssl must not pin $HOME as N-target")
	}
}

func TestTargetFromEntries_TopLevelMixedWithIncludeFile(t *testing.T) {
	// A mix: direct entries in ~/.checkssl plus entries from one include
	// file. The include-file dir wins; the top-level config vote drops.
	entries := []config.Entry{
		{Domain: "verena.de", File: "/Users/u/.checkssl", Line: 1},
		{Domain: "a.example", File: "/Users/u/.domains/customer-a", Line: 1},
		{Domain: "b.example", File: "/Users/u/.domains/customer-a", Line: 2},
	}
	dir, suf, ok := targetFromEntries(entries)
	if !ok {
		t.Fatal("expected ok=true (one include file present)")
	}
	if dir != "/Users/u/.domains" {
		t.Errorf("dir = %q, want /Users/u/.domains", dir)
	}
	if suf != "" {
		t.Errorf("suffix = %q, want empty", suf)
	}
}

func TestResolveNewFileTarget_LegacyFallbackWhenNoIncludes(t *testing.T) {
	// Simulate a user whose ~/.checkssl exists but has only direct
	// entries (no @include matches). N must land in ~/.domains/, NOT
	// in <cfgDir>/domains.d/ — otherwise the legacy ~/.checkssl never
	// sees the new file.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CHECKSSL_CONFIG_DIR", filepath.Join(home, "cfgdir"))
	// Create ~/.checkssl so DetectLegacy picks it up.
	if err := os.WriteFile(filepath.Join(home, ".checkssl"), []byte("verena.de\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := settings.Defaults()
	entries := []config.Entry{
		{Domain: "verena.de", File: filepath.Join(home, ".checkssl"), Line: 1},
	}
	dir, suf := resolveNewFileTarget(cfg, entries)
	wantDir := filepath.Join(home, ".domains")
	if dir != wantDir {
		t.Errorf("dir = %q, want %q (legacy fallback)", dir, wantDir)
	}
	if suf != "" {
		t.Errorf("suffix = %q, want empty", suf)
	}
}

func TestResolveNewFileTarget_PrefersEntriesDirOverConfig(t *testing.T) {
	// Entries live in ~/.domains/ (legacy) — new file must go there,
	// NOT in the config-pinned <cfgDir>/domains.d/.
	t.Setenv("CHECKSSL_CONFIG_DIR", "/tmp/cfg")
	cfg := settings.Defaults() // DomainsGlob = "domains.d/*.list"
	entries := []config.Entry{
		{Domain: "x.example", File: "/home/u/.domains/foo", Line: 1},
	}
	dir, suf := resolveNewFileTarget(cfg, entries)
	if dir != "/home/u/.domains" {
		t.Errorf("dir = %q, want /home/u/.domains (follow entries, not config)", dir)
	}
	if suf != "" {
		t.Errorf("suffix = %q, want empty for legacy", suf)
	}
}

func TestResolveNewFileTarget_FallsBackToConfigWhenNoEntries(t *testing.T) {
	// Isolate from a real legacy ~/.checkssl on the dev machine, which
	// would otherwise route the new file to ~/.domains/.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CHECKSSL_CONFIG_DIR", "/tmp/cfg")
	cfg := settings.Defaults()
	dir, suf := resolveNewFileTarget(cfg, nil)
	wantDir := filepath.Join("/tmp/cfg", "domains.d")
	if dir != wantDir {
		t.Errorf("dir = %q, want %q", dir, wantDir)
	}
	if suf != ".list" {
		t.Errorf("suffix = %q, want .list", suf)
	}
}
