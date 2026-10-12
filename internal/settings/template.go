package settings

// ExampleConfig is the content `checkssl config init` writes when no
// config exists yet. Every setting is present but commented out —
// uncomment to override the hardcoded fallback. Keep this in sync with
// KnownKeys and the Defaults() values above.
const ExampleConfig = `# checkssl defaults.
#
# Every key is optional — the shipped defaults are what checkssl falls
# back to when a key is absent or commented out. CLI flags and
# CHECKSSL_* env vars always win over this file.
#
# Edit with:   checkssl config edit
# Verify with: checkssl config show
# Path with:   checkssl config path

# -- Scan behaviour -----------------------------------------------------

# How long one TLS handshake may take before it is reported as error.
# Go duration syntax: 300ms, 5s, 2m.
#timeout = "5s"

# Maximum TLS handshakes in flight. Higher is faster but can upset load
# balancers; 100 is a safe ceiling.
#concurrency = 100

# Default output format when neither --format nor the TUI is used.
# One of: table | csv | json | html | nagios
#format = "table"

# Add the resolved IP as a column. Same effect as --show-ip.
#show_ip = false

# -- Default scan source ------------------------------------------------
#
# What checkssl reads when neither -d nor -f is given. Both paths are
# relative to this config file's directory, so moving the config dir
# just works.

#domains_file = "domains"
#domains_glob = "domains.d/*.list"

# Hardcoded smoke-test domain when nothing else is configured.
#fallback_domain = "google.com"

# -- HTML output --------------------------------------------------------

[html]
# Visual style: monitor (dashboard) or terminal (phosphor mock).
#style = "monitor"

# -- Nagios output ------------------------------------------------------

[nagios]
# Thresholds in days. Only consulted when --format nagios is active.
#warning_days = 30
#critical_days = 14

# -- TUI ----------------------------------------------------------------

[tui]
# Default sort column on launch: days | domain | ip | status | issuer
#sort = "days"
# Default direction: false = ascending (fewest days first).
#sort_desc = false

# Watch config files for external edits and re-scan automatically.
#hot_reload = true

# Keep edit backups for this long before "checkssl config gc" prunes
# them. Set to "0" to keep forever.
#backup_retention = "720h"  # 30 days

# -- Editor -------------------------------------------------------------

[editor]
# Command used when e opens the source file or , opens this config.
# {file} and {line} get substituted; {line} is 1 for new files.
#
# Common presets:
#   vim/nvim/nano:   cmd = "nvim +{line} {file}"
#   VSCode:          cmd = "code --wait --goto {file}:{line}"
#   Sublime Text:    cmd = "subl --wait {file}:{line}"
#   Zed:             cmd = "zed --wait {file}:{line}"
#   JetBrains IDEs:  cmd = "idea --line {line} {file}"
#   Helix:           cmd = "hx {file}:{line}"
#
# Empty / unset: fall back to $VISUAL / $EDITOR invoked as "+{line} {file}"
# (the vi convention), then "vi" as last resort.
#
# IMPORTANT: GUI editors (code, subl, zed, ...) need the --wait flag or
# the editor exits immediately and checkssl reloads too early.
#cmd = ""
`
