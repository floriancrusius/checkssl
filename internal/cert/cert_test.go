package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		expiresAt  time.Time
		authorized bool
		want       Status
	}{
		{"valid", now.Add(90 * 24 * time.Hour), true, StatusValid},
		{"expiring exactly at 30d", now.Add(30 * 24 * time.Hour), true, StatusExpiringSoon},
		{"expiring in 5d", now.Add(5 * 24 * time.Hour), true, StatusExpiringSoon},
		{"expired 1 second ago", now.Add(-1 * time.Second), true, StatusExpired},
		{"expired long ago", now.Add(-100 * 24 * time.Hour), false, StatusExpired},
		{"authorized false, not expired", now.Add(90 * 24 * time.Hour), false, StatusInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.expiresAt, tc.authorized, now); got != tc.want {
				t.Fatalf("Classify() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheck_EmptyDomain(t *testing.T) {
	r := Check(context.Background(), "", Options{})
	if r.Status != StatusError {
		t.Fatalf("status = %q, want %q", r.Status, StatusError)
	}
	if r.Err == "" {
		t.Fatal("expected Err to be populated")
	}
}

func TestCheck_TrimsDomain(t *testing.T) {
	var seenHost string
	dial := func(_ context.Context, host, _ string, _ *tls.Config) (*tls.ConnectionState, string, error) {
		seenHost = host
		return nil, "", errors.New("stop")
	}
	Check(context.Background(), "  example.com\t", Options{Dialer: dial})
	if seenHost != "example.com" {
		t.Fatalf("host = %q, want %q", seenHost, "example.com")
	}
}

func TestCheck_DialError(t *testing.T) {
	dial := func(_ context.Context, _, _ string, _ *tls.Config) (*tls.ConnectionState, string, error) {
		return nil, "", errors.New("connection refused")
	}
	r := Check(context.Background(), "example.com", Options{Dialer: dial})
	if r.Status != StatusError {
		t.Fatalf("status = %q, want %q", r.Status, StatusError)
	}
	if r.Err != "connection refused" {
		t.Fatalf("Err = %q", r.Err)
	}
}

func TestCheck_NoPeerCerts(t *testing.T) {
	dial := func(_ context.Context, _, _ string, _ *tls.Config) (*tls.ConnectionState, string, error) {
		return &tls.ConnectionState{}, "", nil
	}
	r := Check(context.Background(), "example.com", Options{Dialer: dial})
	if r.Status != StatusError {
		t.Fatalf("status = %q", r.Status)
	}
	if r.Err == "" {
		t.Fatal("expected Err")
	}
}

func TestCheck_CopiesCertFields(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	leaf := &x509.Certificate{
		NotAfter: now.Add(90 * 24 * time.Hour),
		Issuer:   pkix.Name{CommonName: "Test CA"},
		DNSNames: []string{"example.com", "www.example.com"},
	}
	dial := func(_ context.Context, _, _ string, _ *tls.Config) (*tls.ConnectionState, string, error) {
		return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}, "198.51.100.42", nil
	}
	r := Check(context.Background(), "example.com", Options{Now: now, Dialer: dial})

	if !r.ExpiresAt.Equal(leaf.NotAfter) {
		t.Errorf("ExpiresAt = %v, want %v", r.ExpiresAt, leaf.NotAfter)
	}
	if r.Issuer != "Test CA" {
		t.Errorf("Issuer = %q", r.Issuer)
	}
	if len(r.SubjectAlt) != 2 || r.SubjectAlt[0] != "example.com" {
		t.Errorf("SubjectAlt = %v", r.SubjectAlt)
	}
	// Verify will fail (no CA), so status is Invalid — that's fine, the
	// point of this test is the field copying.
	if r.Status != StatusInvalid {
		t.Errorf("Status = %q, want %q (unauthorized leaf)", r.Status, StatusInvalid)
	}
	if r.AuthError == "" {
		t.Error("expected AuthError to be populated for unauthorized leaf")
	}
	if r.ResolvedIP != "198.51.100.42" {
		t.Errorf("ResolvedIP = %q, want 198.51.100.42", r.ResolvedIP)
	}
}

func TestPreferIPv4_OrdersIPv4First(t *testing.T) {
	in := []net.IPAddr{
		{IP: net.ParseIP("2a01:4f8:c01e:ae8::1")},
		{IP: net.ParseIP("138.199.133.112")},
		{IP: net.ParseIP("2001:db8::2")},
		{IP: net.ParseIP("10.0.0.1")},
	}
	got := preferIPv4(in)

	wantOrder := []string{"138.199.133.112", "10.0.0.1", "2a01:4f8:c01e:ae8::1", "2001:db8::2"}
	if len(got) != len(wantOrder) {
		t.Fatalf("len = %d, want %d", len(got), len(wantOrder))
	}
	for i, want := range wantOrder {
		if got[i].IP.String() != want {
			t.Errorf("position %d: got %s, want %s (full order: %v)", i, got[i].IP, want, got)
		}
	}
}

func TestPreferIPv4_LeavesOrderWhenAllSameFamily(t *testing.T) {
	in := []net.IPAddr{
		{IP: net.ParseIP("10.0.0.1")},
		{IP: net.ParseIP("10.0.0.2")},
		{IP: net.ParseIP("10.0.0.3")},
	}
	got := preferIPv4(in)
	for i := range in {
		if got[i].IP.String() != in[i].IP.String() {
			t.Errorf("position %d changed: got %s, want %s", i, got[i].IP, in[i].IP)
		}
	}
}

func TestPreferIPv4_DoesNotMutateInput(t *testing.T) {
	in := []net.IPAddr{
		{IP: net.ParseIP("::1")},
		{IP: net.ParseIP("127.0.0.1")},
	}
	first := in[0].IP.String()
	preferIPv4(in)
	if in[0].IP.String() != first {
		t.Errorf("input was mutated: in[0] = %s, originally %s", in[0].IP, first)
	}
}

func TestDialPreferIPv4_FallsBackWhenIPv4Fails(t *testing.T) {
	// Simulate the user's scenario: resolver returns both families but we
	// cannot reach the host at all. The error we see should come from
	// the LAST tried IP, i.e. an IPv6 address — proving we actually tried
	// IPv4 first and then fell through.
	resolve := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{
			{IP: net.ParseIP("2001:db8::1")}, // RFC 3849 documentation prefix, unreachable
			{IP: net.ParseIP("192.0.2.1")},   // TEST-NET-1, unreachable
		}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, _, err := dialPreferIPv4(ctx, "example.invalid", "443",
		&tls.Config{ServerName: "example.invalid", MinVersion: tls.VersionTLS12}, resolve)
	if err == nil {
		t.Fatal("expected an error when every address is unreachable")
	}
	// Last tried IP should have been the IPv6 one — the error should mention it.
	if !strings.Contains(err.Error(), "2001:db8::1") {
		t.Errorf("final error should come from the last tried IP (IPv6), got: %v", err)
	}
}

func TestDialPreferIPv4_ResolverError(t *testing.T) {
	resolve := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return nil, errors.New("nxdomain")
	}
	_, _, err := dialPreferIPv4(context.Background(), "nope.example", "443",
		&tls.Config{}, resolve)
	if err == nil || !strings.Contains(err.Error(), "nxdomain") {
		t.Errorf("expected resolver error to propagate, got: %v", err)
	}
}

func TestDialPreferIPv4_IPLiteralSkipsResolver(t *testing.T) {
	resolve := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		t.Fatal("resolver should not be called for IP literals")
		return nil, nil
	}
	// Dialing TEST-NET-1 will fail at the TCP layer, but we only care that
	// the resolver wasn't invoked.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, _, _ = dialPreferIPv4(ctx, "192.0.2.1", "443",
		//nolint:gosec // test-only; the connection won't complete anyway.
		&tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, resolve)
}
