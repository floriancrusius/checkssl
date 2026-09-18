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
	StatusValid         Status = "valid"
	StatusExpiringSoon  Status = "expiring_soon"
	StatusExpired       Status = "expired"
	StatusInvalid       Status = "invalid"
	StatusError         Status = "error"
	ExpiringSoonWindow         = 30 * 24 * time.Hour
	defaultTimeout             = 5 * time.Second
	defaultPort                = "443"
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
}

// Options configures a Check call. Zero values fall back to sensible defaults.
type Options struct {
	Timeout time.Duration
	Port    string
	// Now overrides time.Now for classification; leave zero in production.
	Now time.Time
	// Dialer lets tests substitute the network layer.
	Dialer Dialer
}

// Dialer abstracts a TLS dialer so tests can inject a fake.
type Dialer func(ctx context.Context, host, port string, cfg *tls.Config) (*tls.ConnectionState, error)

func defaultDialer(ctx context.Context, host, port string, cfg *tls.Config) (*tls.ConnectionState, error) {
	d := &net.Dialer{}
	conn, err := tls.DialWithDialer(d, "tcp", net.JoinHostPort(host, port), cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	state := conn.ConnectionState()
	return &state, nil
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
		ServerName:         domain,
		InsecureSkipVerify: true, // we verify manually below to keep the cert
		MinVersion:         tls.VersionTLS12,
	}

	state, err := dial(dialCtx, domain, port, cfg)
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

	// Independent verification using intermediates the server sent.
	intermediates := x509.NewCertPool()
	for _, c := range state.PeerCertificates[1:] {
		intermediates.AddCert(c)
	}
	_, verifyErr := leaf.Verify(x509.VerifyOptions{
		DNSName:       domain,
		Intermediates: intermediates,
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
