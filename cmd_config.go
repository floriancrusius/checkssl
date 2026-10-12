package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/floriancrusius/checkssl/internal/config"
	"github.com/floriancrusius/checkssl/internal/editor"
	"github.com/floriancrusius/checkssl/internal/paths"
	"github.com/floriancrusius/checkssl/internal/settings"
)

// runConfigCmd dispatches `checkssl config …` sub-commands.
func runConfigCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: checkssl config <path|edit|show|init|migrate|gc> [flags]")
		return exitCodeUsageError
	}
	switch args[0] {
	case "path":
		return cfgPathCmd(stdout, stderr)
	case "edit":
		return cfgEditCmd(stdout, stderr)
	case "show":
		return cfgShowCmd(stdout, stderr)
	case "init":
		return cfgInitCmd(args[1:], stdin, stdout, stderr)
	case "migrate":
		return cfgMigrateCmd(args[1:], stdout, stderr)
	case "gc":
		return cfgGCCmd(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q — supported: path, edit, show, init, migrate, gc\n", args[0])
		return exitCodeUsageError
	}
}

// cfgPathCmd prints the config dir. Script-friendly, no newline-less
// surprises — one line, trailing \n.
func cfgPathCmd(stdout, stderr io.Writer) int {
	dir, err := paths.ConfigDir()
	if err != nil {
		fmt.Fprintln(stderr, "config dir unresolvable:", err)
		return exitCodeError
	}
	fmt.Fprintln(stdout, dir)
	return exitCodeSuccess
}

// cfgEditCmd opens the config file in the user's editor. Creates the
// file (and its dir) from the ExampleConfig template if it doesn't
// exist yet, so the user never lands in a blank buffer.
func cfgEditCmd(stdout, stderr io.Writer) int {
	cfgPath, err := paths.ConfigFile()
	if err != nil {
		fmt.Fprintln(stderr, "config dir unresolvable:", err)
		return exitCodeError
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
			fmt.Fprintln(stderr, "mkdir:", err)
			return exitCodeError
		}
		if err := os.WriteFile(cfgPath, []byte(settings.ExampleConfig), 0o600); err != nil {
			fmt.Fprintln(stderr, "write:", err)
			return exitCodeError
		}
		fmt.Fprintln(stdout, "created", cfgPath)
	}
	// Load settings purely to pick up editor.cmd — if even that fails
	// we still launch the fallback editor so the user can fix things.
	s, _ := settings.Load(cfgPath)
	cmd, err := editor.Resolve(s.EditorCmd, cfgPath, 1)
	if err != nil {
		fmt.Fprintln(stderr, "editor:", err)
		return exitCodeError
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(stderr, "editor exited:", err)
		return exitCodeError
	}
	return exitCodeSuccess
}

// cfgShowCmd prints the effective config with per-key provenance so
// the user can see which overrides are in play.
func cfgShowCmd(stdout, stderr io.Writer) int {
	cfgPath, _ := paths.ConfigFile()
	s, err := settings.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, "config:", err)
		return exitCodeError
	}
	// Longest key sets the column width; keep readable but tight.
	keys := settings.KnownKeys()
	keyWidth := 0
	for _, k := range keys {
		if len(k) > keyWidth {
			keyWidth = len(k)
		}
	}
	for _, k := range keys {
		fmt.Fprintf(stdout, "%-*s = %-30s (%s)\n", keyWidth, k, settingValue(s, k), s.Source[k])
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "config file:", cfgPath)
	return exitCodeSuccess
}

// settingValue extracts a key's value from Settings as a display string.
// Centralised so cfgShowCmd stays short.
func settingValue(s settings.Settings, key string) string {
	switch key {
	case "timeout":
		return s.Timeout.String()
	case "concurrency":
		return strconv.Itoa(s.Concurrency)
	case "format":
		return s.Format
	case "show_ip":
		return strconv.FormatBool(s.ShowIP)
	case "domains_file":
		return s.DomainsFile
	case "domains_glob":
		return s.DomainsGlob
	case "fallback_domain":
		return s.FallbackDomain
	case "html.style":
		return s.HTMLStyle
	case "nagios.warning_days":
		return strconv.Itoa(s.NagiosWarning)
	case "nagios.critical_days":
		return strconv.Itoa(s.NagiosCritical)
	case "tui.sort":
		return s.TUISort
	case "tui.sort_desc":
		return strconv.FormatBool(s.TUISortDesc)
	case "tui.hot_reload":
		return strconv.FormatBool(s.TUIHotReload)
	case "tui.backup_retention":
		return s.BackupRetention.String()
	case "editor.cmd":
		if s.EditorCmd == "" {
			return "(fall through to $VISUAL/$EDITOR)"
		}
		return s.EditorCmd
	}
	return "?"
}

// cfgInitCmd creates the config dir + initial files. If --editor is
// given OR stdin isn't a TTY, runs non-interactively; otherwise asks
// the user which editor to pin.
func cfgInitCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var (
		editorFlag    string
		nonInteractiv bool
		force         bool
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--non-interactive":
			nonInteractiv = true
		case a == "--force":
			force = true
		case a == "--editor" && i+1 < len(args):
			editorFlag = args[i+1]
			i++
		case strings.HasPrefix(a, "--editor="):
			editorFlag = strings.TrimPrefix(a, "--editor=")
		default:
			fmt.Fprintln(stderr, "unknown flag:", a)
			return exitCodeUsageError
		}
	}

	cfgDir, err := paths.ConfigDir()
	if err != nil {
		fmt.Fprintln(stderr, "config dir unresolvable:", err)
		return exitCodeError
	}
	cfgFile := filepath.Join(cfgDir, "config.toml")
	domainsFile := filepath.Join(cfgDir, "domains")
	domainsDir := filepath.Join(cfgDir, "domains.d")

	// Refuse to clobber unless --force was passed.
	if _, err := os.Stat(cfgFile); err == nil && !force {
		fmt.Fprintf(stderr, "%s already exists — pass --force to overwrite\n", cfgFile)
		return exitCodeError
	}

	tmpl := settings.ExampleConfig
	if editorFlag == "" && !nonInteractiv && isTTY(stdin) {
		picked := pickEditorInteractively(stdin, stdout)
		if picked != "" {
			tmpl = injectEditorCmd(tmpl, picked)
		}
	} else if editorFlag != "" {
		if p, ok := editor.FindPreset(editorFlag); ok {
			tmpl = injectEditorCmd(tmpl, p.Tmpl)
		} else {
			tmpl = injectEditorCmd(tmpl, editorFlag) // raw template
		}
	}

	if err := os.MkdirAll(domainsDir, 0o700); err != nil {
		fmt.Fprintln(stderr, "mkdir:", err)
		return exitCodeError
	}
	if err := os.WriteFile(cfgFile, []byte(tmpl), 0o600); err != nil {
		fmt.Fprintln(stderr, "write config:", err)
		return exitCodeError
	}
	// Empty domains file so the user has something to append to.
	if _, err := os.Stat(domainsFile); os.IsNotExist(err) {
		if err := os.WriteFile(domainsFile,
			[]byte("# One domain per line. `#` starts a comment.\n"), 0o600); err != nil {
			fmt.Fprintln(stderr, "write domains:", err)
			return exitCodeError
		}
	}

	fmt.Fprintf(stdout, "Created %s\n", cfgDir)
	fmt.Fprintln(stdout, "  config.toml       # defaults — edit with: checkssl config edit")
	fmt.Fprintln(stdout, "  domains           # empty — one domain per line")
	fmt.Fprintln(stdout, "  domains.d/        # for include-globbed group files")
	return exitCodeSuccess
}

// injectEditorCmd uncomments / replaces the editor.cmd line in the
// template so the user's picked preset lands active on disk.
func injectEditorCmd(tmpl, cmd string) string {
	// The template has `#cmd = ""` under [editor]; swap it for the live
	// value. Falls back to append if the sentinel isn't there.
	marker := `#cmd = ""`
	live := fmt.Sprintf("cmd = %q", cmd)
	if strings.Contains(tmpl, marker) {
		return strings.Replace(tmpl, marker, live, 1)
	}
	return tmpl + "\n[editor]\n" + live + "\n"
}

// pickEditorInteractively prompts the user to choose from the preset
// list, returning the chosen template. Empty return means "skip,
// leave editor.cmd empty".
func pickEditorInteractively(stdin io.Reader, stdout io.Writer) string {
	fmt.Fprintln(stdout, "Welcome to checkssl.")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Which editor should checkssl use for `e` (edit source)")
	fmt.Fprintln(stdout, "and `,` (edit config)?")
	for i, p := range editor.Presets {
		mark := "  "
		if _, err := exec.LookPath(p.Bin); err == nil {
			mark = "✓ "
		}
		fmt.Fprintf(stdout, "  %d. %s%s\n", i+1, mark, p.Name)
	}
	fmt.Fprintln(stdout, "  0.    Skip — fall back to $VISUAL / $EDITOR")
	fmt.Fprint(stdout, "> ")
	r := bufio.NewReader(stdin)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" || line == "0" {
		return ""
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(editor.Presets) {
		fmt.Fprintln(stdout, "invalid choice — skipping, you can set it later with `checkssl config edit`")
		return ""
	}
	return editor.Presets[n-1].Tmpl
}

// isTTY reports whether r is an *os.File backed by a terminal. Keeps
// the init prompt from launching when stdin is a pipe.
func isTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// cfgMigrateCmd copies the legacy ~/.checkssl and ~/.domains/* into
// the new config dir. Originals are kept so the user can roll back.
func cfgMigrateCmd(args []string, stdout, stderr io.Writer) int {
	dryRun := false
	for _, a := range args {
		switch a {
		case "--dry-run", "-n":
			dryRun = true
		default:
			fmt.Fprintln(stderr, "unknown flag:", a)
			return exitCodeUsageError
		}
	}

	legacy := paths.DetectLegacy()
	if !legacy.Any() {
		fmt.Fprintln(stdout, "no legacy ~/.checkssl or ~/.domains/ found — nothing to migrate")
		return exitCodeSuccess
	}

	cfgDir, err := paths.ConfigDir()
	if err != nil {
		fmt.Fprintln(stderr, "config dir unresolvable:", err)
		return exitCodeError
	}
	if err := os.MkdirAll(filepath.Join(cfgDir, "domains.d"), 0o700); err != nil {
		fmt.Fprintln(stderr, "mkdir:", err)
		return exitCodeError
	}

	// Primary file: ~/.checkssl → <cfgDir>/domains, with any
	// `@include ~/.domains/...` directives rewritten to point at the
	// new <cfgDir>/domains.d/ layout. Without the rewrite, the migrated
	// primary keeps referencing the legacy path — once the user removes
	// ~/.domains/ they end up with an active primary pointing to a dead
	// directory, even though all the lists sit next to it in domains.d/.
	if legacy.CheckSSLFile != "" {
		dst := filepath.Join(cfgDir, "domains")
		if err := copyPrimaryWithRewrites(legacy.CheckSSLFile, dst, dryRun, stdout); err != nil {
			fmt.Fprintln(stderr, "copy:", err)
			return exitCodeError
		}
	}
	// Includes: ~/.domains/* → <cfgDir>/domains.d/
	if legacy.DomainsDir != "" {
		entries, err := os.ReadDir(legacy.DomainsDir)
		if err != nil {
			fmt.Fprintln(stderr, "read legacy dir:", err)
			return exitCodeError
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			// Skip stray backup pollution from the pre-v2 writer.
			if strings.Contains(e.Name(), ".checkssl-") && strings.HasSuffix(e.Name(), ".bak") {
				continue
			}
			src := filepath.Join(legacy.DomainsDir, e.Name())
			dst := filepath.Join(cfgDir, "domains.d", e.Name()+".list")
			if err := copyWithLog(src, dst, dryRun, stdout); err != nil {
				fmt.Fprintln(stderr, "copy:", err)
				return exitCodeError
			}
		}
	}

	if dryRun {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "(dry run — nothing was actually written)")
	} else {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Done. Legacy files are kept; delete them when you're confident")
		fmt.Fprintln(stdout, "the new config works for you:")
		if legacy.CheckSSLFile != "" {
			fmt.Fprintln(stdout, "  rm", legacy.CheckSSLFile)
		}
		if legacy.DomainsDir != "" {
			fmt.Fprintln(stdout, "  rm -r", legacy.DomainsDir)
		}
	}
	return exitCodeSuccess
}

// copyWithLog is a tiny file copy helper for cfgMigrateCmd. Prints
// each copy to stdout; honours dryRun.
func copyWithLog(src, dst string, dryRun bool, stdout io.Writer) error {
	fmt.Fprintf(stdout, "  %s → %s\n", src, dst)
	if dryRun {
		return nil
	}
	//nolint:gosec // src is from DetectLegacy, dst under ConfigDir.
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	//nolint:gosec // dst is derived from ConfigDir().
	return os.WriteFile(dst, data, 0o600)
}

// copyPrimaryWithRewrites copies the legacy ~/.checkssl to dst, but
// rewrites `@include ~/.domains/...` directives so the migrated file
// points at the new <cfgDir>/domains.d/ layout. Every rewrite is
// reported so the user can see what changed.
func copyPrimaryWithRewrites(src, dst string, dryRun bool, stdout io.Writer) error {
	fmt.Fprintf(stdout, "  %s → %s\n", src, dst)
	//nolint:gosec // src is from DetectLegacy.
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	rewritten, changes := rewriteLegacyIncludes(string(data))
	for _, c := range changes {
		fmt.Fprintf(stdout, "    rewrote @include: %s → %s\n", c.from, c.to)
	}
	if dryRun {
		return nil
	}
	//nolint:gosec // dst is derived from ConfigDir().
	return os.WriteFile(dst, []byte(rewritten), 0o600)
}

type includeRewrite struct{ from, to string }

// rewriteLegacyIncludes finds `@include ~/.domains/...` lines in the
// legacy primary file and rewrites them to point at the new
// <cfgDir>/domains.d/ layout. The rewrite mirrors how cfgMigrateCmd
// copies the include files themselves:
//
//   - `@include ~/.domains/*`      → `@include domains.d/*.list`
//   - `@include ~/.domains/<name>` → `@include domains.d/<name>.list`
//
// Everything else (direct domains, comments, other includes) is
// preserved verbatim. The returned changes list is purely informational
// for the migrate log.
func rewriteLegacyIncludes(content string) (string, []includeRewrite) {
	const prefix = "@include "
	var out strings.Builder
	var changes []includeRewrite
	lines := strings.Split(content, "\n")
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, prefix) {
			spec := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			// Strip any inline `#` comment from the spec, as the parser
			// does — otherwise we'd rewrite the whole trailing comment.
			commentIdx := strings.Index(spec, "#")
			tail := ""
			if commentIdx >= 0 {
				tail = " " + strings.TrimSpace(spec[commentIdx:])
				spec = strings.TrimSpace(spec[:commentIdx])
			}
			if newSpec, ok := rewriteLegacyIncludeSpec(spec); ok {
				changes = append(changes, includeRewrite{from: spec, to: newSpec})
				out.WriteString(prefix + newSpec)
				if tail != "" {
					out.WriteString(tail)
				}
				if i < len(lines)-1 {
					out.WriteString("\n")
				}
				continue
			}
		}
		out.WriteString(raw)
		if i < len(lines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String(), changes
}

// rewriteLegacyIncludeSpec applies the ~/.domains/… → domains.d/…
// mapping to a single @include spec. Returns ok=false for specs that
// don't start with the legacy prefix (left untouched).
func rewriteLegacyIncludeSpec(spec string) (string, bool) {
	const legacyPrefix = "~/.domains/"
	if !strings.HasPrefix(spec, legacyPrefix) {
		return "", false
	}
	rest := spec[len(legacyPrefix):]
	switch {
	case rest == "" || rest == "*":
		return "domains.d/*.list", true
	case strings.ContainsAny(rest, "*?["):
		// Preserve the user's glob shape but anchor it under domains.d/.
		// A trailing `*` becomes `*.list` to mirror the per-file rename
		// the migration performs (hermine.ai → hermine.ai.list).
		if strings.HasSuffix(rest, "*") {
			return "domains.d/" + rest + ".list", true
		}
		return "domains.d/" + rest, true
	default:
		return "domains.d/" + rest + ".list", true
	}
}

// cfgGCCmd prunes old session backups.
func cfgGCCmd(args []string, stdout, stderr io.Writer) int {
	// Resolve retention from settings so one source of truth drives it.
	cfgPath, _ := paths.ConfigFile()
	s, err := settings.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, "config:", err)
		return exitCodeError
	}
	retention := s.BackupRetention
	for i := 0; i < len(args); i++ {
		if args[i] == "--older-than" && i+1 < len(args) {
			d, err := durationFromArg(args[i+1])
			if err != nil {
				fmt.Fprintln(stderr, "bad --older-than:", err)
				return exitCodeUsageError
			}
			retention = d
			i++
		}
	}
	root, _ := paths.BackupsDir()
	n, err := config.PruneOlderThan(root, retention)
	if err != nil {
		fmt.Fprintln(stderr, "prune:", err)
		return exitCodeError
	}
	fmt.Fprintf(stdout, "removed %d session folder(s) under %s\n", n, root)
	// Also list what's left so the user has a sense of the cache size.
	remaining, _ := os.ReadDir(root)
	if len(remaining) > 0 {
		sort.Slice(remaining, func(i, j int) bool {
			return remaining[i].Name() < remaining[j].Name()
		})
		fmt.Fprintln(stdout, "kept:")
		for _, r := range remaining {
			fmt.Fprintln(stdout, "  ", r.Name())
		}
	}
	return exitCodeSuccess
}

// durationFromArg accepts Go duration syntax plus a plain integer-days
// convenience (e.g. "7" → 7 days). Returned 0 means "forever" to
// PruneOlderThan.
func durationFromArg(s string) (time.Duration, error) {
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
