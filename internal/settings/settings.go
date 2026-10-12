// Package settings is the single source of truth for checkssl's
// configurable defaults. It loads a TOML config file, overlays env
// variables, and tracks where every effective value came from so
// `checkssl config show` can tell the user exactly what's active.
//
// Precedence (highest first):
//
//  1. CLI flag (applied by main after settings.Load returns)
//  2. CHECKSSL_* env var
//  3. ~/.config/checkssl/config.toml (or OS equivalent)
//  4. Hardcoded fallback shipped with checkssl
package settings

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// Source names where a particular setting came from. Used for the
// `config show` provenance column.
type Source string

const (
	SourceDefault Source = "default"
	SourceConfig  Source = "config"
	SourceEnv     Source = "env"
	SourceCLI     Source = "cli"
)

// Settings is the fully-resolved effective configuration — what the
// rest of the program should consult. Zero values are hardcoded
// fallbacks; call Load() to overlay a config file + env.
type Settings struct {
	Timeout         time.Duration
	Concurrency     int
	Format          string
	ShowIP          bool
	DomainsFile     string
	DomainsGlob     string
	FallbackDomain  string
	HTMLStyle       string
	NagiosWarning   int
	NagiosCritical  int
	TUISort         string
	TUISortDesc     bool
	TUIHotReload    bool
	BackupRetention time.Duration
	EditorCmd       string // template with {file} / {line}

	// Provenance per key. Keys match the TOML field name (dotted for
	// nested tables), e.g. "timeout", "nagios.warning_days".
	Source map[string]Source
}

// Defaults returns a Settings with every field populated from the
// hardcoded fallback. The provenance map says SourceDefault for every
// known key.
func Defaults() Settings {
	s := Settings{
		Timeout:         5 * time.Second,
		Concurrency:     100,
		Format:          "table",
		ShowIP:          false,
		DomainsFile:     "domains",
		DomainsGlob:     "domains.d/*.list",
		FallbackDomain:  "google.com",
		HTMLStyle:       "monitor",
		NagiosWarning:   30,
		NagiosCritical:  14,
		TUISort:         "days",
		TUISortDesc:     false,
		TUIHotReload:    true,
		BackupRetention: 30 * 24 * time.Hour,
		EditorCmd:       "", // empty → fall through to $VISUAL/$EDITOR/vi
	}
	s.Source = map[string]Source{}
	for _, k := range KnownKeys() {
		s.Source[k] = SourceDefault
	}
	return s
}

// KnownKeys lists every settable key in canonical dotted form. Order
// matters for `config show` output.
func KnownKeys() []string {
	return []string{
		"timeout",
		"concurrency",
		"format",
		"show_ip",
		"domains_file",
		"domains_glob",
		"fallback_domain",
		"html.style",
		"nagios.warning_days",
		"nagios.critical_days",
		"tui.sort",
		"tui.sort_desc",
		"tui.hot_reload",
		"tui.backup_retention",
		"editor.cmd",
	}
}

// fileSchema mirrors the TOML structure. Pointer fields let us tell
// "absent" apart from "zero value explicitly set".
type fileSchema struct {
	Timeout        *string `toml:"timeout"`
	Concurrency    *int    `toml:"concurrency"`
	Format         *string `toml:"format"`
	ShowIP         *bool   `toml:"show_ip"`
	DomainsFile    *string `toml:"domains_file"`
	DomainsGlob    *string `toml:"domains_glob"`
	FallbackDomain *string `toml:"fallback_domain"`

	HTML struct {
		Style *string `toml:"style"`
	} `toml:"html"`

	Nagios struct {
		WarningDays  *int `toml:"warning_days"`
		CriticalDays *int `toml:"critical_days"`
	} `toml:"nagios"`

	TUI struct {
		Sort            *string `toml:"sort"`
		SortDesc        *bool   `toml:"sort_desc"`
		HotReload       *bool   `toml:"hot_reload"`
		BackupRetention *string `toml:"backup_retention"`
	} `toml:"tui"`

	Editor struct {
		Cmd *string `toml:"cmd"`
	} `toml:"editor"`
}

// Load builds effective settings. configPath may be empty — then only
// defaults + env are considered. A missing config file is NOT an error
// (that's the "no config yet" first-run case); a malformed one is.
func Load(configPath string) (Settings, error) {
	s := Defaults()
	if configPath != "" {
		if err := applyFile(configPath, &s); err != nil {
			return s, err
		}
	}
	applyEnv(&s)
	return s, nil
}

// applyFile reads configPath and overlays it onto s. Missing file is OK.
func applyFile(configPath string, s *Settings) error {
	//nolint:gosec // path is derived from paths.ConfigFile(), controlled by env.
	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", configPath, err)
	}
	var f fileSchema
	if err := toml.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse %s: %w", configPath, err)
	}
	if f.Timeout != nil {
		d, err := time.ParseDuration(*f.Timeout)
		if err != nil {
			return fmt.Errorf("%s: timeout %q: %w", configPath, *f.Timeout, err)
		}
		s.Timeout = d
		s.Source["timeout"] = SourceConfig
	}
	if f.Concurrency != nil {
		s.Concurrency = *f.Concurrency
		s.Source["concurrency"] = SourceConfig
	}
	if f.Format != nil {
		s.Format = *f.Format
		s.Source["format"] = SourceConfig
	}
	if f.ShowIP != nil {
		s.ShowIP = *f.ShowIP
		s.Source["show_ip"] = SourceConfig
	}
	if f.DomainsFile != nil {
		s.DomainsFile = *f.DomainsFile
		s.Source["domains_file"] = SourceConfig
	}
	if f.DomainsGlob != nil {
		s.DomainsGlob = *f.DomainsGlob
		s.Source["domains_glob"] = SourceConfig
	}
	if f.FallbackDomain != nil {
		s.FallbackDomain = *f.FallbackDomain
		s.Source["fallback_domain"] = SourceConfig
	}
	if f.HTML.Style != nil {
		s.HTMLStyle = *f.HTML.Style
		s.Source["html.style"] = SourceConfig
	}
	if f.Nagios.WarningDays != nil {
		s.NagiosWarning = *f.Nagios.WarningDays
		s.Source["nagios.warning_days"] = SourceConfig
	}
	if f.Nagios.CriticalDays != nil {
		s.NagiosCritical = *f.Nagios.CriticalDays
		s.Source["nagios.critical_days"] = SourceConfig
	}
	if f.TUI.Sort != nil {
		s.TUISort = *f.TUI.Sort
		s.Source["tui.sort"] = SourceConfig
	}
	if f.TUI.SortDesc != nil {
		s.TUISortDesc = *f.TUI.SortDesc
		s.Source["tui.sort_desc"] = SourceConfig
	}
	if f.TUI.HotReload != nil {
		s.TUIHotReload = *f.TUI.HotReload
		s.Source["tui.hot_reload"] = SourceConfig
	}
	if f.TUI.BackupRetention != nil {
		d, err := time.ParseDuration(*f.TUI.BackupRetention)
		if err != nil {
			return fmt.Errorf("%s: tui.backup_retention %q: %w", configPath, *f.TUI.BackupRetention, err)
		}
		s.BackupRetention = d
		s.Source["tui.backup_retention"] = SourceConfig
	}
	if f.Editor.Cmd != nil {
		s.EditorCmd = *f.Editor.Cmd
		s.Source["editor.cmd"] = SourceConfig
	}
	return nil
}

// envTable maps CHECKSSL_* env var names to a setter that overlays the
// value onto s. Order doesn't matter — each key is independent.
func applyEnv(s *Settings) {
	type entry struct {
		env string
		key string
		set func(string) error
	}
	entries := []entry{
		{"CHECKSSL_TIMEOUT", "timeout", func(v string) error {
			d, err := time.ParseDuration(v)
			if err != nil {
				return err
			}
			s.Timeout = d
			return nil
		}},
		{"CHECKSSL_CONCURRENCY", "concurrency", func(v string) error {
			n, err := strconv.Atoi(v)
			if err != nil {
				return err
			}
			s.Concurrency = n
			return nil
		}},
		{"CHECKSSL_FORMAT", "format", func(v string) error { s.Format = v; return nil }},
		{"CHECKSSL_SHOW_IP", "show_ip", func(v string) error {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return err
			}
			s.ShowIP = b
			return nil
		}},
		{"CHECKSSL_DOMAINS_FILE", "domains_file", func(v string) error { s.DomainsFile = v; return nil }},
		{"CHECKSSL_DOMAINS_GLOB", "domains_glob", func(v string) error { s.DomainsGlob = v; return nil }},
		{"CHECKSSL_FALLBACK_DOMAIN", "fallback_domain", func(v string) error { s.FallbackDomain = v; return nil }},
		{"CHECKSSL_HTML_STYLE", "html.style", func(v string) error { s.HTMLStyle = v; return nil }},
		{"CHECKSSL_NAGIOS_WARNING_DAYS", "nagios.warning_days", func(v string) error {
			n, err := strconv.Atoi(v)
			if err != nil {
				return err
			}
			s.NagiosWarning = n
			return nil
		}},
		{"CHECKSSL_NAGIOS_CRITICAL_DAYS", "nagios.critical_days", func(v string) error {
			n, err := strconv.Atoi(v)
			if err != nil {
				return err
			}
			s.NagiosCritical = n
			return nil
		}},
		{"CHECKSSL_TUI_SORT", "tui.sort", func(v string) error { s.TUISort = v; return nil }},
		{"CHECKSSL_TUI_SORT_DESC", "tui.sort_desc", func(v string) error {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return err
			}
			s.TUISortDesc = b
			return nil
		}},
		{"CHECKSSL_TUI_HOT_RELOAD", "tui.hot_reload", func(v string) error {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return err
			}
			s.TUIHotReload = b
			return nil
		}},
		{"CHECKSSL_TUI_BACKUP_RETENTION", "tui.backup_retention", func(v string) error {
			d, err := time.ParseDuration(v)
			if err != nil {
				return err
			}
			s.BackupRetention = d
			return nil
		}},
		{"CHECKSSL_EDITOR_CMD", "editor.cmd", func(v string) error { s.EditorCmd = v; return nil }},
	}
	for _, e := range entries {
		v := os.Getenv(e.env)
		if v == "" {
			continue
		}
		if err := e.set(v); err != nil {
			// Bad env is more interesting than silent — surface it, keep
			// prior value. Caller can print this to stderr.
			fmt.Fprintf(os.Stderr, "checkssl: %s=%q invalid: %v (ignored)\n", e.env, v, err)
			continue
		}
		s.Source[e.key] = SourceEnv
	}
}

// MarkCLI flips a key's provenance to SourceCLI. Call this from main
// after a CLI flag was explicitly passed, so `config show` tells the
// truth.
func (s *Settings) MarkCLI(key string) {
	if s.Source == nil {
		s.Source = map[string]Source{}
	}
	s.Source[key] = SourceCLI
}
