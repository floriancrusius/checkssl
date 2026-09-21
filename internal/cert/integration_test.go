package cert

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testTLSServer spins up a local TLS listener whose certificate is signed by
// a freshly generated CA. Callers get the CA cert pool (to pass into Options)
// and the address to hand to Check (host:port). t.Cleanup shuts it all down.
func testTLSServer(t *testing.T, notBefore, notAfter time.Time, dnsNames []string) (roots *x509.CertPool, host, port string) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen CA key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "checkssl-test-ca"},
		NotBefore:             notBefore.Add(-time.Hour),
		NotAfter:              notAfter.Add(24 * 365 * time.Hour), // outlive any leaf we test
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen leaf key: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf cert: %v", err)
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{leafDER, caDER},
			PrivateKey:  leafKey,
			Leaf:        caCert, // any parsed cert is fine; Go re-parses Certificate[0]
		}},
		MinVersion: tls.VersionTLS12,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	u := strings.TrimPrefix(srv.URL, "https://")
	h, p, err := net.SplitHostPort(u)
	if err != nil {
		t.Fatalf("split %q: %v", u, err)
	}

	roots = x509.NewCertPool()
	roots.AddCert(caCert)
	return roots, h, p
}

func TestCheck_Integration_ValidCert(t *testing.T) {
	now := time.Now()
	roots, host, port := testTLSServer(t, now.Add(-time.Hour), now.Add(60*24*time.Hour), []string{"localhost"})

	r := Check(context.Background(), "localhost", Options{
		Port:    port,
		RootCAs: roots,
		Now:     now,
	})

	if r.Status != StatusValid {
		t.Fatalf("Status = %q, want %q (host %s, err=%s, auth=%s)",
			r.Status, StatusValid, host, r.Err, r.AuthError)
	}
	if r.AuthError != "" {
		t.Errorf("AuthError = %q, want empty", r.AuthError)
	}
	if r.Err != "" {
		t.Errorf("Err = %q, want empty", r.Err)
	}
	if len(r.SubjectAlt) == 0 || r.SubjectAlt[0] != "localhost" {
		t.Errorf("SubjectAlt = %v", r.SubjectAlt)
	}
	if r.Issuer != "checkssl-test-ca" {
		t.Errorf("Issuer = %q", r.Issuer)
	}
}

func TestCheck_Integration_ExpiringSoon(t *testing.T) {
	now := time.Now()
	roots, _, port := testTLSServer(t, now.Add(-time.Hour), now.Add(5*24*time.Hour), []string{"localhost"})

	r := Check(context.Background(), "localhost", Options{Port: port, RootCAs: roots, Now: now})
	if r.Status != StatusExpiringSoon {
		t.Fatalf("Status = %q, want %q (err=%s auth=%s)", r.Status, StatusExpiringSoon, r.Err, r.AuthError)
	}
}

func TestCheck_Integration_Expired(t *testing.T) {
	now := time.Now()
	// Leaf expired 1 hour ago; CA still valid so the handshake completes.
	roots, _, port := testTLSServer(t, now.Add(-48*time.Hour), now.Add(-time.Hour), []string{"localhost"})

	r := Check(context.Background(), "localhost", Options{Port: port, RootCAs: roots, Now: now})
	if r.Status != StatusExpired {
		t.Fatalf("Status = %q, want %q", r.Status, StatusExpired)
	}
	if r.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should still be populated for expired certs")
	}
}

func TestCheck_Integration_UntrustedCA(t *testing.T) {
	now := time.Now()
	// Do NOT pass RootCAs — Check falls back to the system pool, which
	// does not know about our test CA.
	_, _, port := testTLSServer(t, now.Add(-time.Hour), now.Add(60*24*time.Hour), []string{"localhost"})

	r := Check(context.Background(), "localhost", Options{Port: port, Now: now})
	if r.Status != StatusInvalid {
		t.Fatalf("Status = %q, want %q", r.Status, StatusInvalid)
	}
	if r.AuthError == "" {
		t.Error("AuthError should be populated for untrusted CA")
	}
	if r.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be populated even when unauthorized")
	}
}

func TestCheck_Integration_HostnameMismatch(t *testing.T) {
	now := time.Now()
	// Cert is for "other.example" but we connect as "localhost".
	roots, _, port := testTLSServer(t, now.Add(-time.Hour), now.Add(60*24*time.Hour), []string{"other.example"})

	r := Check(context.Background(), "localhost", Options{Port: port, RootCAs: roots, Now: now})
	if r.Status != StatusInvalid {
		t.Fatalf("Status = %q, want %q", r.Status, StatusInvalid)
	}
	if !strings.Contains(r.AuthError, "certificate is valid for") &&
		!strings.Contains(r.AuthError, "not") {
		// Wording varies across Go versions; assert a substring both use.
		t.Errorf("AuthError should mention name mismatch, got: %s", r.AuthError)
	}
}

func TestCheck_Integration_ConnectionRefused(t *testing.T) {
	// Grab a port that nobody is listening on.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open listener: %v", err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := l.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	r := Check(context.Background(), "localhost", Options{Port: port, Timeout: 2 * time.Second})
	if r.Status != StatusError {
		t.Fatalf("Status = %q, want %q (err=%s)", r.Status, StatusError, r.Err)
	}
	if r.Err == "" {
		t.Error("Err should be populated on dial failure")
	}
}
