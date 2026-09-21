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
| `-h, --help`                  | show help                                           |
| `-v, --version`               | show version                                        |

### Sub-commands

- `checkssl completion {bash|zsh|fish}` — print a completion script suitable
  for `eval` or `source`.

### Config file

When you pass neither `-d` nor `-f`, `checkssl` reads `~/.checkssl` — one
domain per line, `#` starts a comment.

```
# production
api.example.com
www.example.com

# staging
staging.example.com     # rotated 2026-08
```

### Include directive

Lines starting with `@include <path>` pull in another file, or a glob of
files. Paths may be absolute, start with `~/` (home-expanded), or resolve
relative to the including file. Glob patterns (`*`, `?`, `[…]`) are
matched and visited in sorted order; directories are silently skipped;
include cycles are detected and reported.

```
# ~/.checkssl
@include ~/.domains/*.list

# extra domains that don't live in a group file yet
staging.example.com
```

Handy for splitting a big list by customer, project, or environment:

```
~/.domains/
├── customer-a.list
├── customer-b.list
├── production.list
└── staging.list
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

Standalone HTML report with inline CSS, dark-mode support, and a
click-to-sort table. Suitable for emailing, dropping into a static file
server, or printing. No external assets — one `.html` file, no CDN.

```
checkssl -f ~/.checkssl --format html > report.html
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
0 6 * * *  checkssl -f ~/.checkssl -s || curl -H "Priority: high" \
             -H "Title: SSL check failed" \
             -d "$(checkssl -f ~/.checkssl --format json)" \
             https://ntfy.example.com/ssl
```

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
internal/cert/             tls.Dial + independent x509 verification
  cert.go
  cert_test.go             unit tests via injected Dialer
  integration_test.go      end-to-end via httptest with a local CA
internal/render/           table / csv / json formatters
```

The `internal/cert` package is import-safe from other Go programs — if you
want a certificate-checking function inside a bigger service, `cert.Check`
returns the same `Result` type `checkssl` uses.

## License

ISC
