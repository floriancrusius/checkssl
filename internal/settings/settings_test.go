package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaults_PopulatesEverything(t *testing.T) {
	s := Defaults()
	if s.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v", s.Timeout)
	}
	if s.Concurrency != 100 {
		t.Errorf("Concurrency = %d", s.Concurrency)
	}
	if s.Format != "table" {
		t.Errorf("Format = %q", s.Format)
	}
	for _, k := range KnownKeys() {
		if got := s.Source[k]; got != SourceDefault {
			t.Errorf("source for %q = %q, want default", k, got)
		}
	}
}

func TestLoad_NoConfigFileIsFine(t *testing.T) {
	// Pointing at a non-existent file should give pure defaults, not
	// an error — matches the first-run scenario.
	s, err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want default", s.Timeout)
	}
}

func TestLoad_ConfigOverlaysDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	content := `
timeout = "12s"
concurrency = 42
show_ip = true

[html]
style = "terminal"

[nagios]
warning_days = 60

[tui]
sort = "domain"
sort_desc = true

[editor]
cmd = "code --wait --goto {file}:{line}"
`
	if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.Timeout != 12*time.Second {
		t.Errorf("Timeout = %v", s.Timeout)
	}
	if s.Concurrency != 42 {
		t.Errorf("Concurrency = %d", s.Concurrency)
	}
	if !s.ShowIP {
		t.Error("ShowIP should be true")
	}
	if s.HTMLStyle != "terminal" {
		t.Errorf("HTMLStyle = %q", s.HTMLStyle)
	}
	if s.NagiosWarning != 60 {
		t.Errorf("NagiosWarning = %d", s.NagiosWarning)
	}
	if s.TUISort != "domain" {
		t.Errorf("TUISort = %q", s.TUISort)
	}
	if !s.TUISortDesc {
		t.Error("TUISortDesc should be true")
	}
	if s.EditorCmd != "code --wait --goto {file}:{line}" {
		t.Errorf("EditorCmd = %q", s.EditorCmd)
	}
	// Keys we did set must be marked SourceConfig; keys we didn't must
	// remain SourceDefault.
	if got := s.Source["timeout"]; got != SourceConfig {
		t.Errorf("source[timeout] = %q", got)
	}
	if got := s.Source["format"]; got != SourceDefault {
		t.Errorf("source[format] = %q (should be default, we didn't set it)", got)
	}
}

func TestLoad_MalformedConfigIsAnError(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(cfg, []byte("timeout = this is not valid toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(cfg); err == nil {
		t.Fatal("expected an error on malformed TOML")
	}
}

func TestLoad_EnvOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte(`timeout = "12s"`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHECKSSL_TIMEOUT", "99s")
	s, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.Timeout != 99*time.Second {
		t.Errorf("env should have won: Timeout = %v", s.Timeout)
	}
	if got := s.Source["timeout"]; got != SourceEnv {
		t.Errorf("source[timeout] = %q, want env", got)
	}
}

func TestApplyEnv_InvalidValueIsLoggedNotFatal(t *testing.T) {
	// A junk env value must not blow up Load — the user's config/defaults
	// should keep working. (The warning goes to stderr; we don't capture
	// it here, just verify the value stayed defaulted.)
	t.Setenv("CHECKSSL_CONCURRENCY", "not-a-number")
	s, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if s.Concurrency != 100 {
		t.Errorf("Concurrency = %d, want default 100", s.Concurrency)
	}
}

func TestMarkCLI_FlagsProvenance(t *testing.T) {
	s := Defaults()
	s.MarkCLI("timeout")
	if got := s.Source["timeout"]; got != SourceCLI {
		t.Errorf("source[timeout] = %q", got)
	}
}

func TestExampleConfig_IsValidTOML(t *testing.T) {
	// Writing the template to disk and reading it back must succeed —
	// otherwise `checkssl config init` would hand the user a config
	// that `config show` can't parse.
	cfg := filepath.Join(t.TempDir(), "example.toml")
	if err := os.WriteFile(cfg, []byte(ExampleConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(cfg); err != nil {
		t.Fatalf("Load on ExampleConfig: %v", err)
	}
}

func TestExampleConfig_MentionsEveryKnownKey(t *testing.T) {
	// The template is the user's doc. If we add a new key but forget to
	// mention it here, the user will never discover it.
	for _, k := range KnownKeys() {
		// Keys in the file are in their bare TOML form (nested tables
		// render as the leaf). Convert dotted to leaf-only for the check.
		leaf := k
		if i := strings.LastIndex(k, "."); i >= 0 {
			leaf = k[i+1:]
		}
		if !strings.Contains(ExampleConfig, leaf) {
			t.Errorf("key %q not mentioned in ExampleConfig", k)
		}
	}
}
