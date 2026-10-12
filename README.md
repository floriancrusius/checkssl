# checkssl

> A single-binary TLS certificate checker for humans and cron jobs.

```
$ checkssl -d google.com -d expired.badssl.com -d self-signed.badssl.com
===========================================================
| expired.badssl.com     | 13.04.2015 | expired 4176d ago |   ← red
| google.com             | 27.11.2026 |        in 67 days |   ← green
| self-signed.badssl.com | 14.09.2028 |       in 728 days |   ← red (invalid)
===========================================================

❌ Errors encountered:

   self-signed.badssl.com: x509: certificate signed by unknown authority
```

The exit code is `1` — so cron notices, Grafana notices, you notice.

---

## Why?

`openssl s_client -connect host:443 </dev/null | openssl x509 -noout -dates`
tells you what you need to know, once, if you enjoy shell pipelines. `checkssl`
is what you want when you have a dozen domains, want CSV or JSON out, want the
exit code to mean something, and want a coloured "expires in N days" column
that jumps out when a cert is about to lapse.

It reports **expired**, **self-signed**, and **hostname-mismatch** certs as
data — not as connection errors — so `checkssl` can tell you _why_ a cert is
unhappy instead of just "handshake failed".

## Install

### macOS / Linux, from source

```
git clone git@github.com:floriancrusius/checkssl.git
cd checkssl
make install                # → /usr/local/bin/checkssl
make install-completions    # → bash / zsh / fish completions
make install-manpage        # → /usr/local/share/man/man1/checkssl.1
```

`make install` uses `INSTALL_PREFIX=/usr/local`; override with
`make install INSTALL_PREFIX=$HOME/.local`. `make uninstall` removes
everything the install targets placed.

### Shell completion without installing

```
# fish
checkssl completion fish | source

# bash
eval "$(checkssl completion bash)"

# zsh (needs a compinit-managed session)
checkssl completion zsh > ~/.zsh/completions/_checkssl
```

### Cross-compile all platforms

```
make build-all              # → dist/checkssl-{darwin,linux,windows}-{amd64,arm64}
```

Static, CGO-free, ~5.9 MB per binary.

## Usage

```
checkssl -d example.com
checkssl -d example.com -d other.com --format json
checkssl -f domains.txt --concurrency 200
```

### Options

| Flag                          | Description                                         |
|-------------------------------|-----------------------------------------------------|
| `-d, --domain <domain>`       | check one domain (repeatable)                       |
| `-f, --file <file>`           | read one domain per line from a file                |
| `-s, --silent`                | suppress the error summary                          |
| `    --format <type>`         | `table` (default), `csv`, `json`, `nagios`, `html`  |
| `    --concurrency <n>`       | max parallel TLS handshakes (default `100`)         |
| `    --timeout <dur>`         | per-domain handshake timeout (default `5s`)         |
| `    --nagios-warning <n>`    | warn threshold in days (default `30`, nagios only)  |
| `    --nagios-critical <n>`   | critical threshold in days (default `14`, nagios only) |
| `    --only <status,...>`     | only show rows with a matching status; exit 1 if any match, 0 otherwise |
| `    --show-ip`               | add the resolved IP address as a column (table/csv/json/html) |
| `    --html-style <style>`    | HTML visual style: `monitor` (default) or `terminal`         |
| `-i, --tui`                   | launch the interactive terminal UI                           |
| `-h, --help`                  | show help                                           |
| `-v, --version`               | show version                                        |

### Sub-commands

- `checkssl config {path|edit|show|init|migrate|gc}` — manage the user
  config file and backup cache. See **Configuration** below.
- `checkssl completion {bash|zsh|fish}` — print a completion script
  suitable for `eval` or `source`.

### Interactive terminal UI

```
checkssl -i                       # scan the configured source in a live TUI
checkssl -i -f ~/.domains/prod.list --show-ip
```

Keybindings:

**Navigation**
- `j` / `k` — next / previous row (also `↑` / `↓`, `ctrl+d` / `ctrl+u` jump ten)
- `g` / `G` — first / last row
- `l` / `Enter` — details panel (full source path:line, issuer, SANs, error)

**Edit the source**
- `e` — open the configured editor on the source file at the right line
- `#` — comment out the selected line
- `D` — delete the selected line (needs `y` to confirm)
- `N` — create a new include-list file and open it in the editor
- `,` — open `config.toml` in the editor

**Filter / sort**
- `s` / `S` — cycle sort column (days → domain → ip → status → issuer)
- `d` — toggle sort direction (asc ↔ desc)
- `1`–`5` — toggle valid / expiring / expired / invalid / error
- `/` — focus the filter box (Enter apply, Esc cancel)

**Session**
- `r` — re-scan from scratch
- `?` — help overlay
- `q` / `Ctrl+C` — quit

Rows appear as handshakes complete, so a slow or large list is immediately
usable rather than making you wait for everything.

**File handling.** The TUI tracks each domain's source file and line
and watches those files for external edits (vim save, VSCode, etc.) via
`fsnotify` — any change triggers an automatic re-scan after a short
debounce. In-TUI mutations (`#`, `D`) write directly to the source
file; the first mutation per session snapshots the original into
`<cache dir>/backups/<session timestamp>/`, so an accidental delete is
one `cp` away from being undone (`checkssl config gc` prunes old
snapshots). Domains added via `-d` or the hardcoded default have no
source and are skipped by the edit keys with an explanatory status-line
note.

## Configuration

`checkssl` keeps its files in the OS-native user config directory:

| OS | Config dir | Cache dir |
|---|---|---|
| Linux | `~/.config/checkssl/` | `~/.cache/checkssl/` |
| macOS | `~/Library/Application Support/checkssl/` | `~/Library/Caches/checkssl/` |
| Windows | `%AppData%\checkssl\` | `%LocalAppData%\checkssl\` |

Override via `$CHECKSSL_CONFIG_DIR` / `$CHECKSSL_CACHE_DIR` (useful for
tests or running two instances side by side).

```
<config dir>/
├── config.toml      # defaults — scan behaviour, editor, TUI
├── domains          # primary domain list (one per line, `#` comments)
└── domains.d/       # per-group lists matched by domains_glob
    ├── customer-a.list
    └── production.list
```

### Getting started

Create the layout with a single command:

```
$ checkssl config init                # interactive — pick your editor
$ checkssl config init --editor code  # non-interactive, pin VSCode
```

Then edit:

```
$ checkssl config edit                # opens config.toml in your editor
$ checkssl config path                # prints the config dir (handy in scripts)
$ checkssl config show                # all effective values + where each came from
```

### Config file (`config.toml`)

Every key is optional. Precedence: **CLI flag > `CHECKSSL_*` env var > `config.toml` > hardcoded default**.

```toml
# ~/.config/checkssl/config.toml

timeout = "5s"              # per-handshake timeout
concurrency = 100           # max parallel handshakes
format = "table"            # table | csv | json | html | nagios
show_ip = false             # add the resolved-IP column

domains_file = "domains"            # primary list, relative to config dir
domains_glob = "domains.d/*.list"   # include glob, relative to config dir
fallback_domain = "google.com"      # scanned when nothing else is configured

[html]
style = "monitor"           # monitor (dashboard) | terminal (phosphor mock)

[nagios]
warning_days = 30
critical_days = 14

[tui]
sort = "days"               # days | domain | ip | status | issuer
sort_desc = false
hot_reload = true           # fsnotify-driven re-scan on config edits
backup_retention = "720h"   # 30 days — see `checkssl config gc`

[editor]
# {file} / {line} get substituted. Empty = fall back to $VISUAL / $EDITOR.
# Common presets:
#   vim/nvim:    cmd = "nvim +{line} {file}"
#   VSCode:      cmd = "code --wait --goto {file}:{line}"
#   Sublime:     cmd = "subl --wait {file}:{line}"
#   Zed:         cmd = "zed --wait {file}:{line}"
#   Helix:       cmd = "hx {file}:{line}"
cmd = ""
```

### Environment variables

Every setting has an equivalent env var named `CHECKSSL_<KEY>` in SCREAMING_SNAKE_CASE
(dotted keys use `_`, e.g. `nagios.warning_days` → `CHECKSSL_NAGIOS_WARNING_DAYS`).
Great for CI and Docker:

```
CHECKSSL_TIMEOUT=10s CHECKSSL_FORMAT=json checkssl > report.json
```

### Domain list format

One domain per line. `#` starts a comment (whole-line or end-of-line).
Lines starting with `@include <path>` pull in another file, or a glob
of files. Paths may be absolute, start with `~/` (home-expanded), or
resolve relative to the including file. Glob patterns (`*`, `?`,
`[…]`) are matched and visited in sorted order; a wildcard glob that
matches zero files is silently accepted (so an empty `domains.d/` is
not an error); directories are silently skipped; include cycles are
detected and reported. A concrete include path with no wildcards that
doesn't exist still reports an error — that's a real typo, not an
empty-collection case.

```
# ~/.config/checkssl/domains
@include domains.d/*.list

# extra domains that don't live in a group file yet
staging.example.com     # rotated 2026-08
```

### Legacy ~/.checkssl + ~/.domains/

If you upgraded from a pre-v2 install, `checkssl` reads `~/.checkssl`
and `~/.domains/*` as a fallback — EITHER the legacy layout OR the
new one, never both — and prints a one-shot hint on stderr. Move
everything to the new location at your own pace:

```
$ checkssl config migrate --dry-run    # preview, including @include rewrites
$ checkssl config migrate              # copy; originals are kept
$ rm ~/.checkssl && rm -rf ~/.domains/ # when you're confident
```

`migrate` copies `~/.checkssl` to `<config dir>/domains` and every
`~/.domains/<name>` to `<config dir>/domains.d/<name>.list`. Any
`@include ~/.domains/...` directives in the primary file are rewritten
to point at the new `domains.d/` layout — without that rewrite the
migrated primary would still reference the old path, and removing the
legacy `~/.domains/` would leave it pointing at nothing. Each rewrite
is printed to stdout so you can audit the change.

### Backups and `config gc`

Every in-TUI mutation (`#`, `D`) snapshots the file before writing.
Snapshots live under `<cache dir>/backups/<session timestamp>/` — never
next to the original — so no `@include` glob picks them up.

Prune old snapshots with:

```
$ checkssl config gc                          # honours tui.backup_retention
$ checkssl config gc --older-than 7           # keep last 7 days
$ checkssl config gc --older-than 168h        # same, in Go duration syntax
```

## Output formats

### `table` (default)

Coloured when stdout is a TTY. `NO_COLOR=1` disables colours everywhere.

- **green** — valid, more than 30 days left
- **yellow** — valid, but expires within 30 days
- **red** — expired _or_ invalid (self-signed, wrong hostname, unknown CA…)
- **dim** — no cert obtained (timeout, connection refused)

### `csv`

Script-friendly, header row included.

```csv
Domain,Expiration,DaysUntilExpiry
api.example.com,27.11.2026,67
www.example.com,03.01.2026,102
```

### `html`

Standalone HTML report with inline CSS, per-status toggle chips and a
live search box that filters rows against every cell. Two visual
styles, pick with `--html-style`:

- **monitor** (default) — dashboard look: metric tiles across the top,
  severity stripe + days-remaining bar on every row, follows the
  viewer's light/dark theme.
- **terminal** — monospace terminal mock, box-drawing frames,
  phosphor-coloured status badges, dark-committed. `/` focuses the
  filter box.

```
checkssl -f ~/.checkssl --format html > report.html                   # monitor
checkssl -f ~/.checkssl --format html --html-style terminal > tty.html
open report.html
```

### `nagios`

Single-line Nagios-compatible plugin output; exit code is `0 / 1 / 2 / 3`
= `OK / WARNING / CRITICAL / UNKNOWN`.

```
$ checkssl -f domains.txt --format nagios ; echo "exit=$?"
WARNING - 3 expiring within 30d (next: api.example.com in 12 days) | total=42 valid=39 warning=3 critical=0 expired=0 invalid=0 error=0 min_days=12;30;14
exit=1
```

### `json`

```json
[
  {
    "domain": "api.example.com",
    "expiration": "27.11.2026",
    "daysUntilExpiry": 67,
    "status": "valid"
  },
  {
    "domain": "self-signed.badssl.com",
    "expiration": "14.09.2028",
    "daysUntilExpiry": 728,
    "status": "invalid",
    "authorizationError": "x509: certificate signed by unknown authority"
  }
]
```

## Status buckets

| Status           | Meaning                                                             |
|------------------|---------------------------------------------------------------------|
| `valid`          | trusted, more than 30 days remaining                                |
| `expiring_soon`  | trusted, expires within 30 days                                     |
| `expired`        | NotAfter is in the past                                             |
| `invalid`        | cert served but rejected (self-signed, wrong DNS name, unknown CA…) |
| `error`          | no cert obtained (connection refused, timeout, DNS…)                |

## Exit code

- `0` — every domain came back `valid` or `expiring_soon`
- `1` — at least one domain came back `expired`, `invalid`, or `error`
- `2` — the CLI itself was invoked incorrectly (bad flag, unknown format)

That makes `checkssl` a drop-in cron / CI probe:

```cron
# ~/.crontab — nightly at 06:00, alert via ntfy on any red
0 6 * * *  out=$(checkssl -f ~/.checkssl --only error,invalid,expired) \
             && [ -n "$out" ] \
             && curl -H "Priority: high" -H "Title: SSL check failed" \
                 -d "$out" https://ntfy.example.com/ssl
```

`--only` turns `checkssl` into a self-silencing probe: it prints nothing
and exits `0` when every domain is green, and prints only the matching
rows (and exits `1`) when something is wrong. Great for pipelines where
"no output" means "no problem".

## Development

```
make test           # go test ./...
make test-race      # with the race detector
make lint           # golangci-lint v2
make fmt            # gofmt -w .
make build          # host binary → bin/checkssl
make build-all      # all platforms → dist/
make clean
```

### Layout

```
main.go                    CLI parsing + orchestration
cmd_config.go              checkssl config {path|edit|show|init|migrate|gc}
internal/cert/             tls.Dial + independent x509 verification
internal/config/           domain-list parser (+ include globs, writer, backups)
internal/editor/           editor template resolver ({file}/{line}, $VISUAL fallback)
internal/paths/            OS-native config/cache dir resolution
internal/render/           table / csv / json / html / nagios formatters
internal/settings/         config.toml loader + env + source-tracked precedence
internal/tui/              Bubble Tea TUI, fsnotify hot-reload, in-place edits
```

The `internal/cert` package is import-safe from other Go programs — if you
want a certificate-checking function inside a bigger service, `cert.Check`
returns the same `Result` type `checkssl` uses.

## License

MIT — see [LICENSE](LICENSE).
