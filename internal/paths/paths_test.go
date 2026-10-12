package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDir_EnvOverrideWins(t *testing.T) {
	t.Setenv(EnvConfigDir, "/tmp/custom-checkssl")
	got, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/custom-checkssl" {
		t.Errorf("got %q, want /tmp/custom-checkssl", got)
	}
}

func TestCacheDir_EnvOverrideWins(t *testing.T) {
	t.Setenv(EnvCacheDir, "/tmp/custom-cache")
	got, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/custom-cache" {
		t.Errorf("got %q, want /tmp/custom-cache", got)
	}
}

func TestConfigDir_FallsBackToOSDefault(t *testing.T) {
	// Make sure the env override is NOT set and we land under the
	// os.UserConfigDir base with the right project subdir.
	t.Setenv(EnvConfigDir, "")
	got, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, string(os.PathSeparator)+appDir) {
		t.Errorf("got %q, want path ending in %q", got, appDir)
	}
}

func TestConfigAndDomainsFile_UnderConfigDir(t *testing.T) {
	t.Setenv(EnvConfigDir, "/tmp/x")
	cfg, _ := ConfigFile()
	dom, _ := DomainsFile()
	dir, _ := DomainsDir()
	if cfg != "/tmp/x/config.toml" {
		t.Errorf("ConfigFile = %q", cfg)
	}
	if dom != "/tmp/x/domains" {
		t.Errorf("DomainsFile = %q", dom)
	}
	if dir != "/tmp/x/domains.d" {
		t.Errorf("DomainsDir = %q", dir)
	}
}

func TestBackupsDir_UnderCacheDir(t *testing.T) {
	t.Setenv(EnvCacheDir, "/tmp/x-cache")
	got, _ := BackupsDir()
	if got != "/tmp/x-cache/backups" {
		t.Errorf("BackupsDir = %q", got)
	}
}

func TestDetectLegacy_FindsExistingFiles(t *testing.T) {
	// Point HOME at a tempdir so we don't look at the real user's
	// legacy install (which may or may not exist in dev).
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Nothing there yet.
	if got := DetectLegacy(); got.Any() {
		t.Errorf("found something in an empty home: %+v", got)
	}

	// Drop a ~/.checkssl file in; now Any() must flip.
	if err := os.WriteFile(filepath.Join(home, ".checkssl"), []byte("ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := DetectLegacy()
	if l.CheckSSLFile == "" {
		t.Error("CheckSSLFile should be set after creating ~/.checkssl")
	}
	if l.DomainsDir != "" {
		t.Error("DomainsDir should still be empty")
	}

	if err := os.Mkdir(filepath.Join(home, ".domains"), 0o700); err != nil {
		t.Fatal(err)
	}
	l = DetectLegacy()
	if l.DomainsDir == "" {
		t.Error("DomainsDir should be set after creating ~/.domains")
	}
}

func TestDetectLegacy_IgnoresDirForCheckSSLAndFileForDomains(t *testing.T) {
	// Edge cases: a `.checkssl` directory shouldn't be mistaken for the
	// config file, and a `.domains` file (not dir) shouldn't be mistaken
	// for the include dir.
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, ".checkssl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".domains"), []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := DetectLegacy()
	if l.CheckSSLFile != "" {
		t.Error("CheckSSLFile should be empty when ~/.checkssl is a dir")
	}
	if l.DomainsDir != "" {
		t.Error("DomainsDir should be empty when ~/.domains is a file")
	}
}
