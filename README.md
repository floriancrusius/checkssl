# checkssl

A small CLI that inspects the TLS certificate of one or more domains and
tells you when they expire.

Written in Go; ships as a single static ~6 MB binary with no runtime
dependencies.

## Install

### From source

```
git clone git@github.com:floriancrusius/checkssl.git
cd checkssl
make install                    # → /usr/local/bin/checkssl
```

`make install` uses `INSTALL_PREFIX=/usr/local` by default. Override with
`make install INSTALL_PREFIX=$HOME/.local`.

### Cross-compile

```
make build-all                  # → dist/checkssl-{darwin,linux,windows}-{amd64,arm64}
```

## Usage

```
checkssl -d example.com
checkssl -d example.com -d other.com --format json
checkssl -f domains.txt --concurrency 200
```

### Options

```
-d, --domain <domain>   check a specific domain (repeatable)
-f, --file <file>       read one domain per line from a file (repeatable)
-s, --silent            suppress error output
    --format <type>     output format: table (default), csv, json
    --concurrency <n>   max parallel TLS handshakes (default 100)
    --timeout <dur>     per-domain handshake timeout (default 5s)
-h, --help              show help
-v, --version           show version
```

### Config file

If you don't pass `-d` or `-f`, `checkssl` reads `~/.checkssl` — one
domain per line, `#` starts a comment.

```
# production
api.example.com
www.example.com

# staging
staging.example.com
```

### Output

**table** (default, colored when stdout is a TTY, `NO_COLOR` respected):

```
===========================================================
| expired.badssl.com     | 13.04.2015 | expired 4176d ago |
| google.com             | 27.11.2026 |        in 70 days |
| self-signed.badssl.com | 14.09.2028 |       in 728 days |
===========================================================
```

**csv** (script-friendly, header row):

```
Domain,Expiration,DaysUntilExpiry
example.com,27.11.2026,70
```

**json**:

```json
[
  {
    "domain": "example.com",
    "expiration": "27.11.2026",
    "daysUntilExpiry": 70,
    "status": "valid"
  }
]
```

### Status buckets

- `valid` — authorized and expires in more than 30 days
- `expiring_soon` — authorized and expires within 30 days
- `expired` — expiry date is in the past
- `invalid` — cert obtained but rejected (self-signed, hostname
  mismatch, unknown CA…); `authorizationError` explains why
- `error` — no cert could be fetched (connection refused, timeout…)

### Exit code

`0` when every domain is `valid` or `expiring_soon`; `1` when at least
one comes back `expired`, `invalid`, or `error`. Handy for cron and CI.

## Development

```
make test                       # go test ./...
make test-race
make lint                       # go vet + gofmt check
make fmt                        # gofmt -w .
```

Layout:

```
main.go                         # CLI entry point
internal/cert/                  # TLS handshake + status classification
internal/render/                # table/csv/json output
```

## License

ISC
