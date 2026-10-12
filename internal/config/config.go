// Package config loads checkssl's domain lists. It understands the
// one-domain-per-line format, `#` comments (whole-line and end-of-line),
// `@include <path>` directives with glob support, and tracks the origin
// of each domain so the TUI can show it, open an editor at the right
// line, and later edit the file in place.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Entry is a domain together with where it came from. For command-line
// `-d` args, File is empty and Line is zero.
type Entry struct {
	Domain string
	File   string // absolute path of the file this entry was parsed from
	Line   int    // 1-indexed line number inside File
}

// domainRegex is a lenient hostname check — just enough to reject
// obvious garbage. Preserves the shape the pre-config-package parser
// accepted so no previously-accepted file suddenly fails.
var domainRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}$`)

// IsValidDomain reports whether d looks like a usable hostname.
func IsValidDomain(d string) bool {
	d = strings.TrimSpace(strings.ToLower(d))
	if len(d) == 0 || len(d) > 253 {
		return false
	}
	return domainRegex.MatchString(d)
}

// ReadFile parses path and every file it transitively includes. The
// returned entries preserve source order and keep duplicates — callers
// that want uniqueness should dedupe on Entry.Domain themselves.
// Errors are accumulated via addErr and reported alongside whatever
// could still be parsed.
func ReadFile(path string, addErr func(string)) []Entry {
	return readFileWithSeen(path, addErr, map[string]struct{}{})
}

// readFileWithSeen implements the include-cycle guard. `seen` holds
// absolute paths already on the current include chain.
func readFileWithSeen(path string, addErr func(string), seen map[string]struct{}) []Entry {
	abs, err := filepath.Abs(path)
	if err != nil {
		addErr(fmt.Sprintf("resolve %s: %v", path, err))
		return nil
	}
	if _, cycle := seen[abs]; cycle {
		addErr(fmt.Sprintf("include cycle: %s already visited", abs))
		return nil
	}
	seen[abs] = struct{}{}

	//nolint:gosec // path comes from the user via -f/--file; that is the intended use.
	data, err := os.ReadFile(abs)
	if err != nil {
		addErr(fmt.Sprintf("read %s: %v", abs, err))
		return nil
	}

	baseDir := filepath.Dir(abs)
	var out []Entry
	for i, raw := range strings.Split(string(data), "\n") {
		lineNo := i + 1
		line := raw
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if rest, ok := strings.CutPrefix(line, "@include "); ok {
			out = append(out, expandInclude(strings.TrimSpace(rest), baseDir, addErr, seen)...)
			continue
		}

		if !IsValidDomain(line) {
			addErr(fmt.Sprintf("invalid domain in %s:%d: %s", abs, lineNo, line))
			continue
		}
		out = append(out, Entry{Domain: line, File: abs, Line: lineNo})
	}
	return out
}

// hasGlobMeta reports whether spec contains any filepath.Glob
// metacharacter. Used to decide whether an empty match is an error
// (concrete missing path) or a benign "nothing to include yet" case
// (wildcard glob that matches no files).
func hasGlobMeta(spec string) bool {
	return strings.ContainsAny(spec, "*?[")
}

// isBackupFile reports whether the basename matches the backup pattern
// that pre-v2 Writer versions dropped next to the original config file.
// Current writes land in the user's cache dir, but existing pollution
// from older versions can still live in ~/.domains/ — skip them so a
// commented-out domain doesn't resurrect from a stale backup.
func isBackupFile(name string) bool {
	return strings.Contains(name, ".checkssl-") && strings.HasSuffix(name, ".bak")
}

// expandInclude resolves a single @include argument. Supports ~ prefix
// and glob patterns (via filepath.Glob). Relative paths resolve against
// baseDir.
func expandInclude(spec, baseDir string, addErr func(string), seen map[string]struct{}) []Entry {
	if strings.HasPrefix(spec, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			spec = filepath.Join(home, spec[2:])
		}
	} else if !filepath.IsAbs(spec) {
		spec = filepath.Join(baseDir, spec)
	}

	matches, err := filepath.Glob(spec)
	if err != nil {
		addErr(fmt.Sprintf("invalid include pattern %q: %v", spec, err))
		return nil
	}
	if len(matches) == 0 {
		// A wildcard glob that matches nothing is fine — the user may
		// just have no list files right now. A concrete path with no
		// wildcards that doesn't match something is a real error.
		if !hasGlobMeta(spec) {
			addErr(fmt.Sprintf("include %q matched no files", spec))
		}
		return nil
	}
	sort.Strings(matches)

	var out []Entry
	for _, m := range matches {
		//nolint:gosec // paths come from an explicit user glob; that is the intended use.
		info, statErr := os.Stat(m)
		if statErr != nil {
			addErr(fmt.Sprintf("stat %s: %v", m, statErr))
			continue
		}
		if info.IsDir() {
			continue
		}
		if isBackupFile(filepath.Base(m)) {
			// Skip our own backup files — see isBackupFile for why.
			continue
		}
		out = append(out, readFileWithSeen(m, addErr, seen)...)
	}
	return out
}

// DedupeFirst keeps the first occurrence of each domain and drops
// later duplicates — matching the pre-config-package behaviour where
// a domain repeated across include files was collapsed to one check.
func DedupeFirst(entries []Entry) []Entry {
	seen := make(map[string]struct{}, len(entries))
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if _, ok := seen[e.Domain]; ok {
			continue
		}
		seen[e.Domain] = struct{}{}
		out = append(out, e)
	}
	return out
}
