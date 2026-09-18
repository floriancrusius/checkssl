package cert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
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
	dial := func(_ context.Context, host, _ string, _ *tls.Config) (*tls.ConnectionState, error) {
		seenHost = host
		return nil, errors.New("stop")
	}
	Check(context.Background(), "  example.com\t", Options{Dialer: dial})
	if seenHost != "example.com" {
		t.Fatalf("host = %q, want %q", seenHost, "example.com")
	}
}

func TestCheck_DialError(t *testing.T) {
	dial := func(_ context.Context, _, _ string, _ *tls.Config) (*tls.ConnectionState, error) {
		return nil, errors.New("connection refused")
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
	dial := func(_ context.Context, _, _ string, _ *tls.Config) (*tls.ConnectionState, error) {
		return &tls.ConnectionState{}, nil
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
	dial := func(_ context.Context, _, _ string, _ *tls.Config) (*tls.ConnectionState, error) {
		return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}, nil
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
}
