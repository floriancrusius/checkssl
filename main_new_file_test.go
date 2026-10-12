package main

import (
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
