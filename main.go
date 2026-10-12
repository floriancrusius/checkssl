// Command checkssl inspects the TLS certificate of one or more domains.
//
// Usage:
//
//	checkssl -d example.com
//	checkssl -f domains.txt --format json
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/floriancrusius/checkssl/internal/cert"
	"github.com/floriancrusius/checkssl/internal/config"
	"github.com/floriancrusius/checkssl/internal/paths"
	"github.com/floriancrusius/checkssl/internal/render"
	"github.com/floriancrusius/checkssl/internal/settings"
	"github.com/floriancrusius/checkssl/internal/tui"
)

//go:embed completions/checkssl.bash
var completionBash string

//go:embed completions/_checkssl
var completionZsh string

//go:embed completions/checkssl.fish
var completionFish string

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

const (
	exitCodeSuccess    = 0
	exitCodeError      = 1
	exitCodeUsageError = 2
)

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type cliOptions struct {
	domains        stringSlice
	files          stringSlice
	silent         bool
	format         string
	help           bool
	version        bool
	concurrency    int
	timeout        time.Duration
	nagiosWarnDays int
	nagiosCritDays int
	only           string
	showIP         bool
	htmlStyle      string
	interactive    bool
}

// allowedOnlyStatuses lists the status values --only accepts.
var allowedOnlyStatuses = []cert.Status{
	cert.StatusValid,
	cert.StatusExpiringSoon,
	cert.StatusExpired,
	cert.StatusInvalid,
	cert.StatusError,
}

// parseOnly turns a comma-separated list like "error,invalid" into a set of
// recognised statuses. Returns an error naming the offending value if any
// token is not a valid status.
func parseOnly(spec string) (map[cert.Status]struct{}, error) {
	if spec == "" {
		return nil, nil
	}
	set := make(map[cert.Status]struct{})
	for _, raw := range strings.Split(spec, ",") {
		token := strings.TrimSpace(strings.ToLower(raw))
		// Accept both "expiring_soon" and "expiring-soon" since humans type both.
		token = strings.ReplaceAll(token, "-", "_")
		if token == "" {
			continue
		}
		ok := false
		for _, s := range allowedOnlyStatuses {
			if token == string(s) {
				set[s] = struct{}{}
				ok = true
				break
			}
		}
		if !ok {
			names := make([]string, len(allowedOnlyStatuses))
			for i, s := range allowedOnlyStatuses {
				names[i] = string(s)
			}
			return nil, fmt.Errorf("unknown status %q — expected one of %s",
				raw, strings.Join(names, ", "))
		}
	}
	return set, nil
}

// filterByStatus returns the subset of results whose Status is in `allow`.
// A nil `allow` is treated as "no filter" and returns the input unchanged.
func filterByStatus(results []cert.Result, allow map[cert.Status]struct{}) []cert.Result {
	if allow == nil {
		return results
	}
	out := make([]cert.Result, 0, len(results))
	for _, r := range results {
		if _, ok := allow[r.Status]; ok {
			out = append(out, r)
		}
	}
	return out
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	// Sub-commands come before flag parsing so `checkssl completion fish`
	// works without competing with `-f`/`--file`.
	if len(args) > 0 && args[0] == "completion" {
		return runCompletion(args[1:], stdout, stderr)
	}

	// Load settings early so flag defaults reflect what the user has
	// pinned in config.toml (and env). A missing config file is fine —
	// settings.Load returns pure defaults in that case. A broken one
	// is fatal: we don't want silent misreads.
	cfgPath, _ := paths.ConfigFile()
	cfg, cfgErr := settings.Load(cfgPath)
	if cfgErr != nil {
		fmt.Fprintln(stderr, "config error:", cfgErr)
		return exitCodeUsageError
	}

	opts, parseErr := parseFlags(args, stderr, cfg)
	if parseErr != nil {
		fmt.Fprintln(stderr, "error:", parseErr)
		printUsage(stderr)
		return exitCodeUsageError
	}

	if opts.help {
		printUsage(stdout)
		return exitCodeSuccess
	}
	if opts.version {
		fmt.Fprintf(stdout, "checkssl %s\n", Version)
		return exitCodeSuccess
	}

	switch opts.format {
	case "table", "csv", "json", "nagios", "html":
	default:
		fmt.Fprintf(stderr, "error: invalid format %q — expected table, csv, json, nagios, or html\n", opts.format)
		return exitCodeUsageError
	}

	onlySet, onlyErr := parseOnly(opts.only)
	if onlyErr != nil {
		fmt.Fprintln(stderr, "error:", onlyErr)
		return exitCodeUsageError
	}

	var errs []string
	addErr := func(s string) { errs = append(errs, s) }

	// Collect domains from -d/--domain and -f/--file first; only fall back
	// to the configured default source if the user gave no source at all.
	entries, sourceGiven := collectEntries(opts, addErr)
	if !sourceGiven && len(entries) == 0 {
		entries = loadDefaultSource(cfg, addErr, stderr)
	}
	if len(entries) == 0 {
		entries = []config.Entry{{Domain: cfg.FallbackDomain}}
	}

	// --tui takes over — swap the one-shot report for the interactive view.
	if opts.interactive {
		// reparse rebuilds the entry list from scratch each time the TUI
		// reloads, so external edits that add/remove domains land on the
		// next scan rather than being invisible until the user restarts.
		// Reload errors are swallowed on purpose: the user already sees
		// them on the first scan, and spamming them on every file-event
		// reload is noise.
		swallow := func(string) {}
		reparse := func() []config.Entry {
			fresh, sourceGivenReload := collectEntries(opts, swallow)
			if !sourceGivenReload && len(fresh) == 0 {
				fresh = loadDefaultSource(cfg, swallow, io.Discard)
			}
			if len(fresh) == 0 {
				fresh = []config.Entry{{Domain: cfg.FallbackDomain}}
			}
			return fresh
		}
		backupRoot, _ := paths.BackupsDir()
		if err := tui.Run(tui.Options{
			Entries:     entries,
			Timeout:     opts.timeout,
			Concurrency: opts.concurrency,
			ShowIP:      opts.showIP,
			BackupRoot:  backupRoot,
			EditorCmd:   cfg.EditorCmd,
			Reload:      reparse,
		}); err != nil {
			fmt.Fprintln(stderr, "tui error:", err)
			return exitCodeError
		}
		return exitCodeSuccess
	}

	// Run TLS checks concurrently, preserving input order.
	results := checkAll(context.Background(), entries, opts, addErr)
	// Certificates come with UTC timestamps; render dates in the user's
	// local time so `23:59 UTC` doesn't display as the day before in CET.
	for i := range results {
		if !results[i].ExpiresAt.IsZero() {
			results[i].ExpiresAt = results[i].ExpiresAt.In(time.Local)
		}
	}
	sorted := render.SortByExpiry(results)
	sorted = filterByStatus(sorted, onlySet)

	// --only + empty result set + table: emit nothing. This is the alerter
	// workflow — pipe stdout into a notifier, fire only when the output is
	// non-empty.
	if onlySet != nil && len(sorted) == 0 {
		return exitCodeSuccess
	}

	colorEnabled := shouldColor(stdout)

	switch opts.format {
	case "csv":
		if err := render.CSV(stdout, sorted, render.CSVOptions{ShowIP: opts.showIP}); err != nil {
			fmt.Fprintln(stderr, "error rendering CSV:", err)
			return exitCodeError
		}
	case "json":
		if err := render.JSON(stdout, sorted, render.JSONOptions{ShowIP: opts.showIP}); err != nil {
			fmt.Fprintln(stderr, "error rendering JSON:", err)
			return exitCodeError
		}
	case "nagios":
		status, err := render.Nagios(stdout, sorted, render.NagiosOptions{
			WarningDays:  opts.nagiosWarnDays,
			CriticalDays: opts.nagiosCritDays,
		})
		if err != nil {
			fmt.Fprintln(stderr, "error rendering nagios:", err)
			return exitCodeError
		}
		// Nagios plugins own the exit code: 0/1/2/3 = OK/WARN/CRIT/UNKNOWN.
		return int(status)
	case "html":
		if err := render.HTML(stdout, sorted, render.HTMLOptions{
			ShowIP: opts.showIP,
			Style:  opts.htmlStyle,
		}); err != nil {
			fmt.Fprintln(stderr, "error rendering HTML:", err)
			return exitCodeError
		}
	default:
		if err := render.Table(stdout, sorted, render.TableOptions{ColorEnabled: colorEnabled, ShowIP: opts.showIP}); err != nil {
			fmt.Fprintln(stderr, "error rendering table:", err)
			return exitCodeError
		}
	}

	if !opts.silent {
		render.PrintErrors(stderr, errs)
	}

	// With --only, exit non-zero when the filter matched anything — turns
	// `checkssl --only error,invalid,expired` into a clean cron probe:
	// exit 1 with the offending rows on stdout, exit 0 silent otherwise.
	if onlySet != nil {
		if len(sorted) > 0 {
			return exitCodeError
		}
		return exitCodeSuccess
	}

	// Default exit-code logic: non-zero if any domain came back as
	// expired / invalid / errored.
	for _, r := range results {
		switch r.Status {
		case cert.StatusExpired, cert.StatusInvalid, cert.StatusError:
			return exitCodeError
		}
	}
	return exitCodeSuccess
}

func parseFlags(args []string, stderr io.Writer, cfg settings.Settings) (cliOptions, error) {
	// Flag defaults come from the effective settings (config file + env),
	// so a CLI invocation without the flag inherits the user's pinned
	// value. CLI-provided flags still override, and we track that via
	// fs.Visit afterwards to feed `config show`.
	opts := cliOptions{
		format:         cfg.Format,
		concurrency:    cfg.Concurrency,
		timeout:        cfg.Timeout,
		nagiosWarnDays: cfg.NagiosWarning,
		nagiosCritDays: cfg.NagiosCritical,
		showIP:         cfg.ShowIP,
		htmlStyle:      cfg.HTMLStyle,
	}

	fs := flag.NewFlagSet("checkssl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// Suppress the default Usage — we print our own via printUsage.
	fs.Usage = func() {}

	fs.Var(&opts.domains, "d", "domain to check (repeatable)")
	fs.Var(&opts.domains, "domain", "domain to check (repeatable)")
	fs.Var(&opts.files, "f", "file with one domain per line (repeatable)")
	fs.Var(&opts.files, "file", "file with one domain per line (repeatable)")
	fs.BoolVar(&opts.silent, "s", false, "suppress error output")
	fs.BoolVar(&opts.silent, "silent", false, "suppress error output")
	fs.StringVar(&opts.format, "format", opts.format, "output format: table, csv, json")
	fs.BoolVar(&opts.help, "h", false, "show help")
	fs.BoolVar(&opts.help, "help", false, "show help")
	fs.BoolVar(&opts.version, "v", false, "show version")
	fs.BoolVar(&opts.version, "version", false, "show version")
	fs.IntVar(&opts.concurrency, "concurrency", opts.concurrency, "max parallel TLS handshakes")
	fs.DurationVar(&opts.timeout, "timeout", opts.timeout, "per-domain TLS handshake timeout")
	fs.IntVar(&opts.nagiosWarnDays, "nagios-warning", opts.nagiosWarnDays, "warn threshold in days for --format nagios")
	fs.IntVar(&opts.nagiosCritDays, "nagios-critical", opts.nagiosCritDays, "critical threshold in days for --format nagios")
	fs.StringVar(&opts.only, "only", "", "comma-separated statuses to include (valid, expiring_soon, expired, invalid, error)")
	fs.BoolVar(&opts.showIP, "show-ip", opts.showIP, "include the resolved IP address in table/csv/json/html output")
	fs.StringVar(&opts.htmlStyle, "html-style", opts.htmlStyle, "visual style for --format html: monitor or terminal")
	fs.BoolVar(&opts.interactive, "tui", false, "launch the interactive terminal UI instead of printing a report")
	fs.BoolVar(&opts.interactive, "i", false, "short for --tui")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.concurrency < 1 {
		return opts, fmt.Errorf("--concurrency must be >= 1")
	}
	return opts, nil
}

// loadDefaultSource reads the entries that the user has configured as
// their standing scan source. Preference order:
//
//  1. New-layout <config dir>/domains + domains.d glob
//  2. Legacy ~/.checkssl + ~/.domains, with a one-shot migration hint
//     on stderr so the user knows to run `checkssl config migrate`
//
// stderr may be io.Discard when called from a reload callback (we don't
// want to spam the hint on every file event). addErr collects parse
// errors as usual.
func loadDefaultSource(cfg settings.Settings, addErr func(string), stderr io.Writer) []config.Entry {
	var entries []config.Entry

	cfgDir, _ := paths.ConfigDir()
	if cfgDir != "" {
		// Primary file.
		primary := filepath.Join(cfgDir, cfg.DomainsFile)
		if _, err := os.Stat(primary); err == nil {
			entries = append(entries, config.ReadFile(primary, addErr)...)
		}
		// Glob — resolved relative to the config dir unless it's absolute.
		glob := cfg.DomainsGlob
		if glob != "" {
			if !filepath.IsAbs(glob) {
				glob = filepath.Join(cfgDir, glob)
			}
			matches, _ := filepath.Glob(glob)
			sort.Strings(matches)
			for _, m := range matches {
				info, err := os.Stat(m)
				if err != nil || info.IsDir() {
					continue
				}
				entries = append(entries, config.ReadFile(m, addErr)...)
			}
		}
	}

	// If the new location has nothing, fall back to the legacy layout
	// one last time. Users who haven't migrated get a hint; it's quiet
	// afterwards (via stderr being Discard on reload paths).
	if len(entries) == 0 {
		legacy := paths.DetectLegacy()
		if legacy.CheckSSLFile != "" {
			fmt.Fprintln(stderr, "hint: reading legacy ~/.checkssl — run `checkssl config migrate` to adopt the new layout")
			entries = append(entries, config.ReadFile(legacy.CheckSSLFile, addErr)...)
		}
	}

	return config.DedupeFirst(entries)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: checkssl [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  -d, --domain <domain>   check a specific domain (repeatable)")
	fmt.Fprintln(w, "  -f, --file <file>       read one domain per line from a file (repeatable)")
	fmt.Fprintln(w, "  -s, --silent            suppress error output")
	fmt.Fprintln(w, "      --format <type>     output format: table (default), csv, json, nagios, html")
	fmt.Fprintln(w, "      --concurrency <n>   max parallel TLS handshakes (default 100)")
	fmt.Fprintln(w, "      --timeout <dur>     per-domain handshake timeout (default 5s)")
	fmt.Fprintln(w, "      --nagios-warning <n>   days threshold (default 30, --format nagios only)")
	fmt.Fprintln(w, "      --nagios-critical <n>  days threshold (default 14, --format nagios only)")
	fmt.Fprintln(w, "      --only <status,...>    only show rows whose status is in the list;")
	fmt.Fprintln(w, "                             exits non-zero when any row matches, 0 when none.")
	fmt.Fprintln(w, "                             statuses: valid, expiring_soon, expired, invalid, error")
	fmt.Fprintln(w, "      --show-ip              include the resolved IP in table/csv/json/html output")
	fmt.Fprintln(w, "      --html-style <style>   HTML visual style: monitor (default) or terminal")
	fmt.Fprintln(w, "  -i, --tui              launch the interactive terminal UI")
	fmt.Fprintln(w, "  -h, --help              show this help")
	fmt.Fprintln(w, "  -v, --version           show version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  checkssl -d google.com")
	fmt.Fprintln(w, "  checkssl -f domains.txt --format json")
	fmt.Fprintln(w, "  checkssl -d a.com -d b.com --concurrency 50")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "If no source is given, checkssl reads its default from the config")
	fmt.Fprintln(w, "directory (see `checkssl config path`). Legacy ~/.checkssl is still")
	fmt.Fprintln(w, "picked up as a fallback — use `checkssl config migrate` to move.")
}

// collectEntries merges -d and -f inputs, dedupes on the domain name
// (first occurrence wins), and reports validation errors.
// `sourceGiven` is true if the user provided any -d or -f at all.
func collectEntries(opts cliOptions, addErr func(string)) (entries []config.Entry, sourceGiven bool) {
	sourceGiven = len(opts.domains) > 0 || len(opts.files) > 0
	for _, path := range opts.files {
		abs, err := filepath.Abs(path)
		if err != nil {
			addErr(fmt.Sprintf("resolve %s: %v", path, err))
			continue
		}
		entries = append(entries, config.ReadFile(abs, addErr)...)
	}
	for _, d := range opts.domains {
		d = strings.TrimSpace(d)
		if !config.IsValidDomain(d) {
			addErr(fmt.Sprintf("invalid domain: %s", d))
			continue
		}
		// Command-line args have no file provenance.
		entries = append(entries, config.Entry{Domain: d})
	}
	return config.DedupeFirst(entries), sourceGiven
}

// checkAll fans out TLS checks across up to opts.concurrency workers,
// preserving the input order in the result slice. Each result is tagged
// with the originating config file / line so the TUI and renderers can
// show provenance.
func checkAll(ctx context.Context, entries []config.Entry, opts cliOptions, addErr func(string)) []cert.Result {
	results := make([]cert.Result, len(entries))
	sem := make(chan struct{}, opts.concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, e := range entries {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, e config.Entry) {
			defer wg.Done()
			defer func() { <-sem }()
			r := cert.Check(ctx, e.Domain, cert.Options{Timeout: opts.timeout})
			r.Source = e.File
			r.SourceLine = e.Line
			results[i] = r
			switch r.Status {
			case cert.StatusError:
				mu.Lock()
				addErr(fmt.Sprintf("%s: %s", e.Domain, r.Err))
				mu.Unlock()
			case cert.StatusInvalid:
				msg := r.AuthError
				if msg == "" {
					msg = "invalid certificate"
				}
				mu.Lock()
				addErr(fmt.Sprintf("%s: %s", e.Domain, msg))
				mu.Unlock()
			}
		}(i, e)
	}
	wg.Wait()
	return results
}

func runCompletion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: checkssl completion <bash|zsh|fish>")
		return exitCodeUsageError
	}
	var script string
	switch args[0] {
	case "bash":
		script = completionBash
	case "zsh":
		script = completionZsh
	case "fish":
		script = completionFish
	default:
		fmt.Fprintf(stderr, "unknown shell %q — supported: bash, zsh, fish\n", args[0])
		return exitCodeUsageError
	}
	fmt.Fprint(stdout, script)
	return exitCodeSuccess
}

func shouldColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
