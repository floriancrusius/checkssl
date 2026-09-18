// Command checkssl inspects the TLS certificate of one or more domains.
//
// Usage:
//
//	checkssl -d example.com
//	checkssl -f domains.txt --format json
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/floriancrusius/checkssl/internal/cert"
	"github.com/floriancrusius/checkssl/internal/render"
)

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

const (
	defaultConfigFile   = ".checkssl"
	defaultDomain       = "google.com"
	defaultFormat       = "table"
	defaultConcurrency  = 100
	defaultTimeoutSecs  = 5
	exitCodeSuccess     = 0
	exitCodeError       = 1
	exitCodeUsageError  = 2
)

// domainRegex matches the same shapes the JS version accepted.
var domainRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}$`)

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type cliOptions struct {
	domains     stringSlice
	files       stringSlice
	silent      bool
	format      string
	help        bool
	version     bool
	concurrency int
	timeout     time.Duration
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, parseErr := parseFlags(args, stderr)
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

	if opts.format != "table" && opts.format != "csv" && opts.format != "json" {
		fmt.Fprintf(stderr, "error: invalid format %q — expected table, csv, or json\n", opts.format)
		return exitCodeUsageError
	}

	var errs []string
	addErr := func(s string) { errs = append(errs, s) }

	// Collect domains from -d/--domain and -f/--file first; only fall back
	// to the default config file if the user gave no source at all.
	domains, sourceGiven := collectDomains(opts, addErr)
	if !sourceGiven && len(domains) == 0 {
		home, err := os.UserHomeDir()
		if err == nil {
			path := filepath.Join(home, defaultConfigFile)
			if _, statErr := os.Stat(path); statErr == nil {
				domains = append(domains, readDomainsFromFile(path, addErr)...)
			}
		}
	}
	if len(domains) == 0 {
		domains = []string{defaultDomain}
	}

	// Run TLS checks concurrently, preserving input order.
	results := checkAll(context.Background(), domains, opts, addErr)
	// Certificates come with UTC timestamps; render dates in the user's
	// local time so `23:59 UTC` doesn't display as the day before in CET.
	for i := range results {
		if !results[i].ExpiresAt.IsZero() {
			results[i].ExpiresAt = results[i].ExpiresAt.In(time.Local)
		}
	}
	sorted := render.SortByExpiry(results)

	colorEnabled := shouldColor(stdout)

	switch opts.format {
	case "csv":
		if err := render.CSV(stdout, sorted, time.Time{}); err != nil {
			fmt.Fprintln(stderr, "error rendering CSV:", err)
			return exitCodeError
		}
	case "json":
		if err := render.JSON(stdout, sorted, time.Time{}); err != nil {
			fmt.Fprintln(stderr, "error rendering JSON:", err)
			return exitCodeError
		}
	default:
		if err := render.Table(stdout, sorted, render.TableOptions{ColorEnabled: colorEnabled}); err != nil {
			fmt.Fprintln(stderr, "error rendering table:", err)
			return exitCodeError
		}
	}

	if !opts.silent {
		render.PrintErrors(stderr, errs)
	}

	// Non-zero exit if any domain came back as expired / invalid / errored.
	for _, r := range results {
		if r.Status == cert.StatusExpired ||
			r.Status == cert.StatusInvalid ||
			r.Status == cert.StatusError {
			return exitCodeError
		}
	}
	return exitCodeSuccess
}

func parseFlags(args []string, stderr io.Writer) (cliOptions, error) {
	opts := cliOptions{
		format:      defaultFormat,
		concurrency: defaultConcurrency,
		timeout:     defaultTimeoutSecs * time.Second,
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
	fs.StringVar(&opts.format, "format", defaultFormat, "output format: table, csv, json")
	fs.BoolVar(&opts.help, "h", false, "show help")
	fs.BoolVar(&opts.help, "help", false, "show help")
	fs.BoolVar(&opts.version, "v", false, "show version")
	fs.BoolVar(&opts.version, "version", false, "show version")
	fs.IntVar(&opts.concurrency, "concurrency", defaultConcurrency, "max parallel TLS handshakes")
	fs.DurationVar(&opts.timeout, "timeout", defaultTimeoutSecs*time.Second, "per-domain TLS handshake timeout")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.concurrency < 1 {
		return opts, fmt.Errorf("--concurrency must be >= 1")
	}
	return opts, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: checkssl [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  -d, --domain <domain>   check a specific domain (repeatable)")
	fmt.Fprintln(w, "  -f, --file <file>       read one domain per line from a file (repeatable)")
	fmt.Fprintln(w, "  -s, --silent            suppress error output")
	fmt.Fprintln(w, "      --format <type>     output format: table (default), csv, json")
	fmt.Fprintln(w, "      --concurrency <n>   max parallel TLS handshakes (default 100)")
	fmt.Fprintln(w, "      --timeout <dur>     per-domain handshake timeout (default 5s)")
	fmt.Fprintln(w, "  -h, --help              show this help")
	fmt.Fprintln(w, "  -v, --version           show version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  checkssl -d google.com")
	fmt.Fprintln(w, "  checkssl -f domains.txt --format json")
	fmt.Fprintln(w, "  checkssl -d a.com -d b.com --concurrency 50")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "If no source is given, checkssl reads ~/.checkssl.")
}

// collectDomains merges -d and -f inputs, dedupes, and reports validation
// errors. `sourceGiven` is true if the user provided any -d or -f at all.
func collectDomains(opts cliOptions, addErr func(string)) (domains []string, sourceGiven bool) {
	sourceGiven = len(opts.domains) > 0 || len(opts.files) > 0
	seen := make(map[string]struct{})
	add := func(d string) {
		if _, ok := seen[d]; ok {
			return
		}
		seen[d] = struct{}{}
		domains = append(domains, d)
	}
	for _, path := range opts.files {
		abs, err := filepath.Abs(path)
		if err != nil {
			addErr(fmt.Sprintf("resolve %s: %v", path, err))
			continue
		}
		for _, d := range readDomainsFromFile(abs, addErr) {
			add(d)
		}
	}
	for _, d := range opts.domains {
		d = strings.TrimSpace(d)
		if !isValidDomain(d) {
			addErr(fmt.Sprintf("invalid domain: %s", d))
			continue
		}
		add(d)
	}
	return
}

func readDomainsFromFile(path string, addErr func(string)) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		addErr(fmt.Sprintf("read %s: %v", path, err))
		return nil
	}
	var out []string
	for _, raw := range strings.Split(string(data), "\n") {
		line := raw
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !isValidDomain(line) {
			addErr(fmt.Sprintf("invalid domain in %s: %s", path, line))
			continue
		}
		out = append(out, line)
	}
	return out
}

func isValidDomain(d string) bool {
	d = strings.TrimSpace(strings.ToLower(d))
	if len(d) == 0 || len(d) > 253 {
		return false
	}
	return domainRegex.MatchString(d)
}

// checkAll fans out TLS checks across up to opts.concurrency workers,
// preserving the input order in the result slice.
func checkAll(ctx context.Context, domains []string, opts cliOptions, addErr func(string)) []cert.Result {
	results := make([]cert.Result, len(domains))
	sem := make(chan struct{}, opts.concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, d := range domains {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, d string) {
			defer wg.Done()
			defer func() { <-sem }()
			r := cert.Check(ctx, d, cert.Options{Timeout: opts.timeout})
			results[i] = r
			if r.Status == cert.StatusError {
				mu.Lock()
				addErr(fmt.Sprintf("%s: %s", d, r.Err))
				mu.Unlock()
			} else if r.Status == cert.StatusInvalid {
				mu.Lock()
				msg := r.AuthError
				if msg == "" {
					msg = "invalid certificate"
				}
				addErr(fmt.Sprintf("%s: %s", d, msg))
				mu.Unlock()
			}
		}(i, d)
	}
	wg.Wait()
	return results
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
