// Package editor resolves the command checkssl uses to launch the
// user's editor on a source file. It understands config.toml's
// [editor] cmd template, the standard $VISUAL / $EDITOR env vars, and
// the vi-style "+<line> <file>" convention as a last resort.
package editor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ErrNoEditor is returned by Resolve when nothing — not the config
// template, not $VISUAL, not $EDITOR, not the hardcoded `vi` fallback —
// could produce a runnable editor. In practice this almost never
// fires because every POSIX system ships vi, but Windows without a
// $EDITOR would hit it.
var ErrNoEditor = errors.New("no editor configured and $VISUAL/$EDITOR are unset")

// Resolve builds an exec.Cmd that opens file at (1-indexed) line in
// the user's editor. tmpl is the configured template; it may be empty,
// in which case the function falls back through $VISUAL, $EDITOR and
// finally "vi".
//
// Supported placeholders in tmpl:
//
//	{file}  — absolute path to the file
//	{line}  — 1-indexed line number
//
// Shell quoting is NOT performed — paths with spaces require the user
// to quote them in the template, or to set $VISUAL/$EDITOR which bypass
// the template entirely.
func Resolve(tmpl string, file string, line int) (*exec.Cmd, error) {
	if line < 1 {
		line = 1
	}
	if tmpl != "" {
		return cmdFromTemplate(tmpl, file, line)
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return viStyle(v, file, line), nil
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return viStyle(v, file, line), nil
	}
	if _, err := exec.LookPath("vi"); err == nil {
		return viStyle("vi", file, line), nil
	}
	return nil, ErrNoEditor
}

// cmdFromTemplate expands {file}/{line} in tmpl and splits the result
// into argv. We respect simple double-quoted segments so a path with a
// space in it survives when the user has wrapped it in quotes.
func cmdFromTemplate(tmpl, file string, line int) (*exec.Cmd, error) {
	expanded := strings.ReplaceAll(tmpl, "{file}", file)
	expanded = strings.ReplaceAll(expanded, "{line}", strconv.Itoa(line))
	parts, err := splitFields(expanded)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("editor template produced an empty command line: %q", tmpl)
	}
	//nolint:gosec // editor is user-configured; invoking it is the whole point.
	return exec.Command(parts[0], parts[1:]...), nil
}

// viStyle wraps a command string the way vi, nvim, nano and most emacs
// frontends expect: `<cmd> +<line> <file>`. The cmd argument may itself
// contain flags (e.g. "nvim --clean"), which get respected.
func viStyle(cmd, file string, line int) *exec.Cmd {
	parts, _ := splitFields(cmd)
	parts = append(parts, "+"+strconv.Itoa(line), file)
	//nolint:gosec // editor is user-configured; invoking it is the whole point.
	return exec.Command(parts[0], parts[1:]...)
}

// splitFields is a tiny shell-field splitter that honours double quotes
// so a path with a space (e.g. "Application Support") survives. Single
// quotes and backslash escapes are NOT supported — users with weirder
// paths should move them or symlink.
func splitFields(s string) ([]string, error) {
	var (
		out     []string
		cur     strings.Builder
		inQuote bool
	)
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case !inQuote && (r == ' ' || r == '\t'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quote in editor command: %q", s)
	}
	flush()
	return out, nil
}

// Preset names a common editor and the template that drives it. Used
// by `checkssl config init` to offer a short interactive picker.
type Preset struct {
	Name string // short label the user picks
	Bin  string // executable we look for in $PATH
	Tmpl string // template to write to config.toml
}

// Presets lists the editors the init wizard offers. Order matters for
// the picker's display.
var Presets = []Preset{
	{Name: "vim / nvim", Bin: "nvim", Tmpl: "nvim +{line} {file}"},
	{Name: "vim (classic)", Bin: "vim", Tmpl: "vim +{line} {file}"},
	{Name: "nano", Bin: "nano", Tmpl: "nano +{line} {file}"},
	{Name: "VSCode", Bin: "code", Tmpl: "code --wait --goto {file}:{line}"},
	{Name: "Sublime Text", Bin: "subl", Tmpl: "subl --wait {file}:{line}"},
	{Name: "Zed", Bin: "zed", Tmpl: "zed --wait {file}:{line}"},
	{Name: "JetBrains (idea/clion/goland)", Bin: "idea", Tmpl: "idea --line {line} {file}"},
	{Name: "Helix", Bin: "hx", Tmpl: "hx {file}:{line}"},
	{Name: "Emacs (terminal)", Bin: "emacs", Tmpl: "emacs -nw +{line} {file}"},
}

// FindPreset returns the preset matching name (case-insensitive, prefix
// match on the Name or exact match on Bin). Returns false if nothing
// matches.
func FindPreset(name string) (Preset, bool) {
	n := strings.ToLower(name)
	for _, p := range Presets {
		if strings.EqualFold(p.Bin, name) || strings.HasPrefix(strings.ToLower(p.Name), n) {
			return p, true
		}
	}
	return Preset{}, false
}
