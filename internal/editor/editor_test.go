package editor

import (
	"strings"
	"testing"
)

func TestResolve_TemplateExpandsPlaceholders(t *testing.T) {
	cmd, err := Resolve("code --wait --goto {file}:{line}", "/tmp/foo.list", 7)
	if err != nil {
		t.Fatal(err)
	}
	// Args[0] on *exec.Cmd is the Path by convention; Args is argv.
	gotArgs := cmd.Args
	wantArgs := []string{"code", "--wait", "--goto", "/tmp/foo.list:7"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("argv = %v, want %v", gotArgs, wantArgs)
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Errorf("argv[%d] = %q, want %q", i, gotArgs[i], wantArgs[i])
		}
	}
}

func TestResolve_LineClampsToOne(t *testing.T) {
	// A new file reports line 0; we must open at line 1 instead of
	// handing editors a nonsense +0 flag.
	cmd, err := Resolve("vim +{line} {file}", "/tmp/foo", 0)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Args[1] != "+1" {
		t.Errorf("argv[1] = %q, want +1", cmd.Args[1])
	}
}

func TestResolve_EnvFallback_UsesVisualFirst(t *testing.T) {
	t.Setenv("VISUAL", "nvim")
	t.Setenv("EDITOR", "nano")
	cmd, err := Resolve("", "/tmp/foo", 3)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Args[0] != "nvim" {
		t.Errorf("argv[0] = %q, want nvim (VISUAL)", cmd.Args[0])
	}
	// vi-style arguments: +<line> <file>
	if cmd.Args[1] != "+3" || cmd.Args[2] != "/tmp/foo" {
		t.Errorf("argv = %v, want [nvim +3 /tmp/foo]", cmd.Args)
	}
}

func TestResolve_EnvFallback_UsesEditorWhenVisualUnset(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nano")
	cmd, err := Resolve("", "/tmp/foo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Args[0] != "nano" {
		t.Errorf("argv[0] = %q, want nano", cmd.Args[0])
	}
}

func TestResolve_UnterminatedQuoteIsAnError(t *testing.T) {
	_, err := Resolve(`code --wait "foo`, "/tmp/x", 1)
	if err == nil || !strings.Contains(err.Error(), "unterminated quote") {
		t.Errorf("want unterminated-quote error, got %v", err)
	}
}

func TestSplitFields_RespectsDoubleQuotes(t *testing.T) {
	parts, err := splitFields(`code --wait "my path/foo bar" extra`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"code", "--wait", "my path/foo bar", "extra"}
	if len(parts) != len(want) {
		t.Fatalf("parts = %v, want %v", parts, want)
	}
	for i := range want {
		if parts[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, parts[i], want[i])
		}
	}
}

func TestFindPreset_MatchesByBinAndNamePrefix(t *testing.T) {
	cases := []struct {
		in      string
		wantOk  bool
		wantBin string
	}{
		{"code", true, "code"},
		{"VSCODE", true, "code"},
		{"vim", true, "nvim"}, // "vim / nvim" prefix wins first
		{"nano", true, "nano"},
		{"zed", true, "zed"},
		{"nothing", false, ""},
	}
	for _, c := range cases {
		p, ok := FindPreset(c.in)
		if ok != c.wantOk {
			t.Errorf("FindPreset(%q) ok = %v, want %v", c.in, ok, c.wantOk)
			continue
		}
		if ok && p.Bin != c.wantBin {
			t.Errorf("FindPreset(%q).Bin = %q, want %q", c.in, p.Bin, c.wantBin)
		}
	}
}
