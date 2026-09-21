package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/floriancrusius/checkssl/internal/cert"
)

func TestNagios_OK(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "a.example", ExpiresAt: now.Add(60 * 24 * time.Hour), Status: cert.StatusValid},
		{Domain: "b.example", ExpiresAt: now.Add(90 * 24 * time.Hour), Status: cert.StatusValid},
	}
	var buf bytes.Buffer
	status, err := Nagios(&buf, results, NagiosOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if status != NagiosOK {
		t.Fatalf("status = %d, want %d", status, NagiosOK)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "OK - ") {
		t.Errorf("output should start with OK -\n%s", out)
	}
	if !strings.Contains(out, "| total=2 valid=2") {
		t.Errorf("missing perfdata\n%s", out)
	}
}

func TestNagios_Warning(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "healthy.example", ExpiresAt: now.Add(60 * 24 * time.Hour), Status: cert.StatusValid},
		{Domain: "soon.example", ExpiresAt: now.Add(20 * 24 * time.Hour), Status: cert.StatusExpiringSoon},
	}
	var buf bytes.Buffer
	status, _ := Nagios(&buf, results, NagiosOptions{Now: now})
	if status != NagiosWarning {
		t.Fatalf("status = %d, want %d", status, NagiosWarning)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "WARNING - ") {
		t.Errorf("output should start with WARNING -\n%s", out)
	}
	if !strings.Contains(out, "soon.example") {
		t.Errorf("output should name the worst domain\n%s", out)
	}
}

func TestNagios_CriticalExpired(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "ok.example", ExpiresAt: now.Add(60 * 24 * time.Hour), Status: cert.StatusValid},
		{Domain: "dead.example", ExpiresAt: now.Add(-24 * time.Hour), Status: cert.StatusExpired},
	}
	var buf bytes.Buffer
	status, _ := Nagios(&buf, results, NagiosOptions{Now: now})
	if status != NagiosCritical {
		t.Fatalf("status = %d, want %d", status, NagiosCritical)
	}
	if !strings.HasPrefix(buf.String(), "CRITICAL - ") {
		t.Errorf("output should start with CRITICAL -\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "1 expired") {
		t.Errorf("missing expired count\n%s", buf.String())
	}
}

func TestNagios_CriticalOnInvalid(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "self.example", ExpiresAt: now.Add(60 * 24 * time.Hour), Status: cert.StatusInvalid,
			AuthError: "self-signed"},
	}
	var buf bytes.Buffer
	status, _ := Nagios(&buf, results, NagiosOptions{Now: now})
	if status != NagiosCritical {
		t.Fatalf("status = %d, want %d", status, NagiosCritical)
	}
}

func TestNagios_CriticalOnErrorResult(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "broken.example", Status: cert.StatusError, Err: "timeout"},
	}
	var buf bytes.Buffer
	status, _ := Nagios(&buf, results, NagiosOptions{Now: now})
	if status != NagiosCritical {
		t.Fatalf("status = %d, want %d", status, NagiosCritical)
	}
	if !strings.Contains(buf.String(), "1 error") {
		t.Errorf("expected '1 error' in output\n%s", buf.String())
	}
}

func TestNagios_CustomThresholdsPromoteToCritical(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "borderline.example", ExpiresAt: now.Add(5 * 24 * time.Hour), Status: cert.StatusExpiringSoon},
	}
	var buf bytes.Buffer
	status, _ := Nagios(&buf, results, NagiosOptions{Now: now, WarningDays: 30, CriticalDays: 7})
	if status != NagiosCritical {
		t.Fatalf("status = %d, want %d (5d ≤ 7d critical)", status, NagiosCritical)
	}
}

func TestNagios_Empty(t *testing.T) {
	var buf bytes.Buffer
	status, _ := Nagios(&buf, nil, NagiosOptions{})
	if status != NagiosUnknown {
		t.Fatalf("status = %d, want %d", status, NagiosUnknown)
	}
	if !strings.HasPrefix(buf.String(), "UNKNOWN") {
		t.Errorf("output should start with UNKNOWN\n%s", buf.String())
	}
}
