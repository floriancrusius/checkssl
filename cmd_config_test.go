package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setConfigDir points CHECKSSL_CONFIG_DIR at a fresh tempdir for the
// duration of a test. Reset via t.Setenv.
func setConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CHECKSSL_CONFIG_DIR", dir)
	return dir
}

func TestCfgPath_PrintsConfigDir(t *testing.T) {
	dir := setConfigDir(t)
	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"path"}, strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	if got := strings.TrimSpace(out.String()); got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestCfgShow_WithNoConfigShowsAllDefaults(t *testing.T) {
	setConfigDir(t)
	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"show"}, strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	s := out.String()
	// Every known key must appear in output with (default) marker.
	for _, k := range []string{"timeout", "concurrency", "html.style", "tui.sort", "editor.cmd"} {
		if !strings.Contains(s, k) {
			t.Errorf("output missing key %q:\n%s", k, s)
		}
	}
	if !strings.Contains(s, "(default)") {
		t.Error("output missing (default) provenance marker")
	}
}

func TestCfgShow_PicksUpConfigFile(t *testing.T) {
	dir := setConfigDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte(`timeout = "42s"`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"show"}, strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	if !strings.Contains(out.String(), "42s") {
		t.Errorf("output missing config value:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "(config)") {
		t.Error("timeout should be marked as sourced from config")
	}
}

func TestCfgInit_NonInteractiveCreatesLayout(t *testing.T) {
	dir := setConfigDir(t)
	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"init", "--non-interactive"},
		strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	for _, want := range []string{"config.toml", "domains", "domains.d"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing after init: %s (%v)", want, err)
		}
	}
}

func TestCfgInit_RefusesToClobberWithoutForce(t *testing.T) {
	dir := setConfigDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Pre-existing config.
	if err := os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte(`# user wrote this`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"init", "--non-interactive"},
		strings.NewReader(""), &out, &errBuf)
	if rc == exitCodeSuccess {
		t.Fatal("init should have refused on existing config")
	}
	// --force must overwrite.
	out.Reset()
	errBuf.Reset()
	rc = runConfigCmd([]string{"init", "--non-interactive", "--force"},
		strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("--force failed: %q", errBuf.String())
	}
	data, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if strings.Contains(string(data), "user wrote this") {
		t.Error("--force did not overwrite")
	}
}

func TestCfgInit_EditorFlagInjectsIntoTemplate(t *testing.T) {
	dir := setConfigDir(t)
	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"init", "--non-interactive", "--editor", "code"},
		strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `code --wait --goto {file}:{line}`) {
		t.Errorf("editor template not written:\n%s", data)
	}
}

func TestCfgInit_RawEditorTemplateAlsoWorks(t *testing.T) {
	// If --editor doesn't match a preset, treat it as a raw template.
	dir := setConfigDir(t)
	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"init", "--non-interactive",
		`--editor=mything --wait --goto {file}:{line}`},
		strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	data, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if !strings.Contains(string(data), "mything") {
		t.Errorf("raw template not written:\n%s", data)
	}
}

func TestCfgMigrate_DryRunCopiesNothing(t *testing.T) {
	// Point HOME at a tempdir with a legacy ~/.checkssl, then migrate
	// --dry-run and verify the new location is still empty.
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := t.TempDir()
	t.Setenv("CHECKSSL_CONFIG_DIR", cfgDir)

	if err := os.WriteFile(filepath.Join(home, ".checkssl"),
		[]byte("legacy.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".domains"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".domains", "customer-a"),
		[]byte("c-a.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"migrate", "--dry-run"}, strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	// Dry run must not create files.
	if _, err := os.Stat(filepath.Join(cfgDir, "domains")); err == nil {
		t.Error("dry-run wrote files to the new location")
	}
	// Must mention both the primary and the include.
	for _, want := range []string{".checkssl", "customer-a"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCfgMigrate_RealRunCopiesLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := t.TempDir()
	t.Setenv("CHECKSSL_CONFIG_DIR", cfgDir)
	if err := os.WriteFile(filepath.Join(home, ".checkssl"),
		[]byte("primary.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".domains"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".domains", "customer-a"),
		[]byte("c-a.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A stale .bak file from an older checkssl version — must NOT be copied.
	if err := os.WriteFile(filepath.Join(home, ".domains", "customer-a.checkssl-000.bak"),
		[]byte("ghost.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"migrate"}, strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	primary, _ := os.ReadFile(filepath.Join(cfgDir, "domains"))
	if string(primary) != "primary.example\n" {
		t.Errorf("primary copy = %q", primary)
	}
	customer, _ := os.ReadFile(filepath.Join(cfgDir, "domains.d", "customer-a.list"))
	if string(customer) != "c-a.example\n" {
		t.Errorf("customer-a copy = %q", customer)
	}
	// Must have skipped the .bak file.
	if _, err := os.Stat(filepath.Join(cfgDir, "domains.d", "customer-a.checkssl-000.bak.list")); err == nil {
		t.Error("backup file was copied — should have been skipped")
	}
}

func TestCfgMigrate_NothingToDo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CHECKSSL_CONFIG_DIR", t.TempDir())
	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"migrate"}, strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	if !strings.Contains(out.String(), "nothing to migrate") {
		t.Errorf("want 'nothing to migrate' message, got: %s", out.String())
	}
}

func TestCfgGC_RemovesOldSessions(t *testing.T) {
	// Point CACHE_DIR at a tempdir with an "old" session folder, run
	// gc, verify it's gone.
	cacheDir := t.TempDir()
	t.Setenv("CHECKSSL_CACHE_DIR", cacheDir)
	// Session folder name is a timestamp — use an obviously-old one.
	old := filepath.Join(cacheDir, "backups", "20000101-000000")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	rc := runConfigCmd([]string{"gc", "--older-than", "1"}, strings.NewReader(""), &out, &errBuf)
	if rc != exitCodeSuccess {
		t.Fatalf("rc = %d, stderr = %q", rc, errBuf.String())
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("old session still exists after gc")
	}
}
