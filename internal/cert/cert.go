// Package cert performs the TLS handshake for a hostname and reports the
// peer certificate together with an independent verification result.
package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"time"
)

// Status classifies the result of a certificate check.
type Status string

const (
	StatusValid        Status = "valid"
	StatusExpiringSoon Status = "expiring_soon"
	StatusExpired      Status = "expired"
	StatusInvalid      Status = "invalid"
	StatusError        Status = "error"
	ExpiringSoonWindow        = 30 * 24 * time.Hour
	defaultTimeout            = 5 * time.Second
	defaultPort               = "443"
)

// Result is what a single Check call returns.
type Result struct {
	Domain     string
	ExpiresAt  time.Time
	Status     Status
	AuthError  string // non-empty when verification failed but a cert was still returned
	Err        string // non-empty when no certificate could be obtained
	Issuer     string
	SubjectAlt []string
	ResolvedIP string // the IP address the TLS handshake actually used
}

// Options configures a Check call. Zero values fall back to sensible defaults.
type Options struct {
	Timeout time.Duration
	Port    string
	// Now overrides time.Now for classification; leave zero in production.
	Now time.Time
	// Dialer lets tests substitute the network layer.
	Dialer Dialer
	// RootCAs overrides the system trust store for the manual verification
	// step. Leave nil to use the system pool.
	RootCAs *x509.CertPool
}

// Dialer abstracts a TLS dialer so tests can inject a fake. The second
// return value is the IP the handshake actually ran against (empty when the
// dialer cannot determine it, e.g. in tests).
type Dialer func(ctx context.Context, host, port string, cfg *tls.Config) (state *tls.ConnectionState, ip string, err error)

// Resolver abstracts host-to-IP resolution so tests can inject a fake.
type Resolver func(ctx context.Context, host string) ([]net.IPAddr, error)

func defaultResolver(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// defaultDialer resolves `host`, prefers IPv4 addresses over IPv6, and tries
// each in turn until one connects. This keeps `checkssl` working on machines
// where IPv6 is disabled or unreachable but the server also publishes an
// A record — the common case. If the host is a literal IP, no resolution
// happens.
func defaultDialer(ctx context.Context, host, port string, cfg *tls.Config) (*tls.ConnectionState, string, error) {
	return dialPreferIPv4(ctx, host, port, cfg, defaultResolver)
}

func dialPreferIPv4(ctx context.Context, host, port string, cfg *tls.Config, resolve Resolver) (*tls.ConnectionState, string, error) {
	// IP literal → dial directly, no resolution.
	if net.ParseIP(host) != nil {
		return tlsDial(ctx, host, port, cfg)
	}

	ips, err := resolve(ctx, host)
	if err != nil {
		return nil, "", err
	}
	if len(ips) == 0 {
		return nil, "", fmt.Errorf("no addresses for %s", host)
	}

	var lastErr error
	for _, ip := range preferIPv4(ips) {
		state, resolvedIP, err := tlsDial(ctx, ip.String(), port, cfg)
		if err == nil {
			return state, resolvedIP, nil
		}
		lastErr = err
	}
	return nil, "", lastErr
}

// preferIPv4 returns a copy of `ips` with IPv4 addresses in front, preserving
// relative order inside each family.
func preferIPv4(ips []net.IPAddr) []net.IPAddr {
	out := make([]net.IPAddr, 0, len(ips))
	// Two passes keep this stable without pulling in sort.
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			out = append(out, ip)
		}
	}
	for _, ip := range ips {
		if ip.IP.To4() == nil {
			out = append(out, ip)
		}
	}
	return out
}

func tlsDial(ctx context.Context, host, port string, cfg *tls.Config) (*tls.ConnectionState, string, error) {
	d := &tls.Dialer{NetDialer: &net.Dialer{}, Config: cfg}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = conn.Close() }()
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil, "", fmt.Errorf("dialer did not return a *tls.Conn")
	}
	state := tlsConn.ConnectionState()
	ip := ""
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		ip = addr.IP.String()
	}
	return &state, ip, nil
}

// Check performs the handshake for a single domain and returns the classified
// result. A returned Result always has Domain set; Err is populated iff no
// certificate could be obtained.
func Check(ctx context.Context, domain string, opts Options) Result {
	domain = strings.TrimSpace(domain)
	res := Result{Domain: domain}
	if domain == "" {
		res.Err = "domain must be a non-empty string"
		res.Status = StatusError
		return res
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	port := opts.Port
	if port == "" {
		port = defaultPort
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	dial := opts.Dialer
	if dial == nil {
		dial = defaultDialer
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cfg := &tls.Config{
		ServerName: domain,
		// We skip Go's built-in verification and run leaf.Verify ourselves so
		// that expired / self-signed / hostname-mismatched certs come back
		// as data (Status + AuthError) instead of connection errors.
		InsecureSkipVerify: true, //nolint:gosec // manual verification below
		MinVersion:         tls.VersionTLS12,
	}

	state, ip, err := dial(dialCtx, domain, port, cfg)
	if err != nil {
		res.Err = err.Error()
		res.Status = StatusError
		return res
	}
	if len(state.PeerCertificates) == 0 {
		res.Err = fmt.Sprintf("no certificate returned for %s", domain)
		res.Status = StatusError
		return res
	}

	leaf := state.PeerCertificates[0]
	res.ExpiresAt = leaf.NotAfter
	res.Issuer = leaf.Issuer.CommonName
	res.SubjectAlt = leaf.DNSNames
	res.ResolvedIP = ip

	// Independent verification using intermediates the server sent.
	intermediates := x509.NewCertPool()
	for _, c := range state.PeerCertificates[1:] {
		intermediates.AddCert(c)
	}
	_, verifyErr := leaf.Verify(x509.VerifyOptions{
		DNSName:       domain,
		Intermediates: intermediates,
		Roots:         opts.RootCAs, // nil → system pool
		CurrentTime:   now,
	})
	if verifyErr != nil {
		res.AuthError = verifyErr.Error()
	}

	res.Status = Classify(res.ExpiresAt, verifyErr == nil, now)
	return res
}

// Classify buckets a certificate by its remaining lifetime and authorization
// state. Exported so callers that already have a certificate on hand can reuse
// the same rules used by Check.
func Classify(expiresAt time.Time, authorized bool, now time.Time) Status {
	remaining := expiresAt.Sub(now)
	if remaining <= 0 {
		return StatusExpired
	}
	if !authorized {
		return StatusInvalid
	}
	if remaining <= ExpiringSoonWindow {
		return StatusExpiringSoon
	}
	return StatusValid
}
