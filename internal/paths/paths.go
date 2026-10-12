// Package paths resolves checkssl's on-disk locations. It wraps
// os.UserConfigDir / os.UserCacheDir so every call site uses the same
// platform-native directory, and it detects the legacy ~/.checkssl +
// ~/.domains/ layout so an upgrade keeps working without an immediate
// migration.
package paths

import (
	"os"
	"path/filepath"
)

// EnvConfigDir, when set, overrides the OS-default config directory.
// Useful for tests and for running two separate checkssl setups side
// by side.
const EnvConfigDir = "CHECKSSL_CONFIG_DIR"

// EnvCacheDir does the same for the cache directory.
const EnvCacheDir = "CHECKSSL_CACHE_DIR"

// appDir is the per-OS subdirectory name both config and cache paths
// end in — the single place that encodes the project's identity.
const appDir = "checkssl"

// ConfigDir returns the directory that holds config.toml, the primary
// domain list and the domains.d/ include folder. Resolves to:
//
//	Linux:   $XDG_CONFIG_HOME/checkssl or $HOME/.config/checkssl
//	macOS:   $HOME/Library/Application Support/checkssl
//	Windows: %AppData%\checkssl
//
// $CHECKSSL_CONFIG_DIR overrides unconditionally. Returns an error only
// when the OS cannot even determine a home directory — the caller
// should treat that as fatal and fall back to a reasonable default.
func ConfigDir() (string, error) {
	if v := os.Getenv(EnvConfigDir); v != "" {
		return v, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDir), nil
}

// CacheDir returns the directory for ephemeral data — session backups,
// anything checkssl can regenerate without losing user state. Resolves
// to:
//
//	Linux:   $XDG_CACHE_HOME/checkssl or $HOME/.cache/checkssl
//	macOS:   $HOME/Library/Caches/checkssl
//	Windows: %LocalAppData%\checkssl
//
// $CHECKSSL_CACHE_DIR overrides unconditionally.
func CacheDir() (string, error) {
	if v := os.Getenv(EnvCacheDir); v != "" {
		return v, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDir), nil
}

// ConfigFile returns the full path to config.toml inside ConfigDir().
func ConfigFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// DomainsFile returns the full path to the primary domain list file,
// which is the modern replacement for ~/.checkssl.
func DomainsFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "domains"), nil
}

// DomainsDir returns the full path to the per-group include directory,
// the modern replacement for ~/.domains/.
func DomainsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "domains.d"), nil
}

// BackupsDir returns the directory inside CacheDir where session-scoped
// edit backups land.
func BackupsDir() (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "backups"), nil
}

// Legacy describes the pre-v2 layout at ~/.checkssl + ~/.domains/.
// Fields are empty strings when the corresponding path does not exist.
type Legacy struct {
	// CheckSSLFile is ~/.checkssl if it exists, empty otherwise.
	CheckSSLFile string
	// DomainsDir is ~/.domains if it exists, empty otherwise.
	DomainsDir string
}

// DetectLegacy returns a Legacy describing which of the old-layout
// paths are present. Used by the soft-migration fallback so an existing
// install keeps working after an upgrade.
func DetectLegacy() Legacy {
	var out Legacy
	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	cfg := filepath.Join(home, ".checkssl")
	if st, err := os.Stat(cfg); err == nil && !st.IsDir() {
		out.CheckSSLFile = cfg
	}
	dom := filepath.Join(home, ".domains")
	if st, err := os.Stat(dom); err == nil && st.IsDir() {
		out.DomainsDir = dom
	}
	return out
}

// Any reports whether either legacy path is present.
func (l Legacy) Any() bool {
	return l.CheckSSLFile != "" || l.DomainsDir != ""
}
