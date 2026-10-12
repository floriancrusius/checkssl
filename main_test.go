package main

import (
	"strings"
	"testing"

	"github.com/floriancrusius/checkssl/internal/cert"
)

func TestParseOnly_RecognisesEveryStatus(t *testing.T) {
	spec := "valid,expiring_soon,expired,invalid,error"
	got, err := parseOnly(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Errorf("got %d statuses, want 5: %v", len(got), got)
	}
}

func TestParseOnly_AcceptsHyphenAndWhitespace(t *testing.T) {
	got, err := parseOnly(" ERROR , expiring-soon ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %v", len(got), got)
	}
}

func TestParseOnly_RejectsUnknown(t *testing.T) {
	_, err := parseOnly("ok,unknown")
	if err == nil || !strings.Contains(err.Error(), "unknown status") {
		t.Errorf("expected 'unknown status' error, got %v", err)
	}
}

func TestParseOnly_EmptyReturnsNil(t *testing.T) {
	got, err := parseOnly("")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil for empty spec, got %v", got)
	}
}

func TestFilterByStatus_NilAllowReturnsInput(t *testing.T) {
	in := []cert.Result{
		{Domain: "a.example", Status: cert.StatusValid},
		{Domain: "b.example", Status: cert.StatusError},
	}
	got := filterByStatus(in, nil)
	if len(got) != 2 {
		t.Errorf("nil filter should keep everything, got %d", len(got))
	}
}

func TestFilterByStatus_KeepsOnlyMatching(t *testing.T) {
	in := []cert.Result{
		{Domain: "ok.example", Status: cert.StatusValid},
		{Domain: "warn.example", Status: cert.StatusExpiringSoon},
		{Domain: "dead.example", Status: cert.StatusExpired},
		{Domain: "self.example", Status: cert.StatusInvalid},
		{Domain: "down.example", Status: cert.StatusError},
	}
	allow := map[cert.Status]struct{}{
		cert.StatusExpired: {},
		cert.StatusInvalid: {},
		cert.StatusError:   {},
	}
	got := filterByStatus(in, allow)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3: %v", len(got), got)
	}
	for _, r := range got {
		switch r.Status {
		case cert.StatusExpired, cert.StatusInvalid, cert.StatusError:
			// ok
		default:
			t.Errorf("unexpected status %q in filtered output", r.Status)
		}
	}
}

func TestFilterByStatus_EmptyAllowDropsAll(t *testing.T) {
	in := []cert.Result{{Domain: "x.example", Status: cert.StatusValid}}
	got := filterByStatus(in, map[cert.Status]struct{}{})
	if len(got) != 0 {
		t.Errorf("empty allow set should drop everything, got %d", len(got))
	}
}
