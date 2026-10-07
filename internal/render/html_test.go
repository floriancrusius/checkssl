package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/floriancrusius/checkssl/internal/cert"
)

func TestHTML_ContainsExpectedSections(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "example.com", ExpiresAt: time.Date(2025, 2, 12, 0, 0, 0, 0, time.UTC),
			Status: cert.StatusValid, Issuer: "Test CA"},
		{Domain: "expiring.example", ExpiresAt: time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
			Status: cert.StatusExpiringSoon, Issuer: "Test CA"},
		{Domain: "gone.example", ExpiresAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			Status: cert.StatusExpired, Issuer: "Old CA"},
		{Domain: "broken.example", Status: cert.StatusError, Err: "timeout"},
	}
	var buf bytes.Buffer
	if err := HTML(&buf, results, HTMLOptions{Now: now, GeneratedAt: now}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	must := []string{
		"<!DOCTYPE html>",
		`<title>checkssl report</title>`,
		"example.com",
		"expiring.example",
		"gone.example",
		"broken.example",
		"12.02.2025",
		`data-status="valid"`,
		`data-status="expiring_soon"`,
		`data-status="expired"`,
		`data-status="invalid"`,
		`data-status="error"`,
		">1 valid</span>",
		">1 expiring soon</span>",
		"1 expired",
		"1 error",
		"timeout",
	}
	for _, s := range must {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q", s)
		}
	}
}

func TestHTML_HasFilterControls(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "example.com", ExpiresAt: now.Add(90 * 24 * time.Hour),
			Status: cert.StatusValid},
	}
	for _, style := range []string{HTMLStyleMonitor, HTMLStyleTerminal} {
		t.Run(style, func(t *testing.T) {
			var buf bytes.Buffer
			if err := HTML(&buf, results, HTMLOptions{Now: now, GeneratedAt: now, Style: style}); err != nil {
				t.Fatal(err)
			}
			out := buf.String()

			must := []string{
				`id="filter"`,
				`id="reset"`,
				`id="count"`,
				`applyFilter`,
				// Grid rows set their own `display`, which otherwise overrides
				// the hidden attribute toggled by applyFilter. The rule below
				// is what actually hides filtered rows.
				`[hidden]`,
				`display: none`,
			}
			for _, s := range must {
				if !strings.Contains(out, s) {
					t.Errorf("filter UI missing %q", s)
				}
			}
		})
	}
}

func TestHTML_EscapesUserInput(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "<script>alert(1)</script>", Status: cert.StatusError,
			Err: `<img src=x onerror="alert(1)">`},
	}
	var buf bytes.Buffer
	if err := HTML(&buf, results, HTMLOptions{Now: now, GeneratedAt: now}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Error("raw <script> tag survived escaping")
	}
	if strings.Contains(out, `onerror="alert(1)"`) {
		t.Error("raw onerror attribute survived escaping")
	}
	if !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("expected html-escaped domain")
	}
}

func TestHTML_CustomTitle(t *testing.T) {
	var buf bytes.Buffer
	if err := HTML(&buf, nil, HTMLOptions{Title: "Nightly SSL sweep"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<title>Nightly SSL sweep</title>") {
		t.Error("custom title not applied")
	}
}

func TestHTML_TerminalStyle(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []cert.Result{
		{Domain: "example.com", ExpiresAt: now.Add(90 * 24 * time.Hour),
			Status: cert.StatusValid, Issuer: "Test CA"},
		{Domain: "broken.example", Status: cert.StatusError, Err: "timeout"},
	}
	var buf bytes.Buffer
	if err := HTML(&buf, results, HTMLOptions{Now: now, GeneratedAt: now, Style: HTMLStyleTerminal}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	must := []string{
		`class="t-list-row"`,
		`class="t-summary"`,
		`JetBrains+Mono`,
		`class="t-status ok"`,
		`class="t-status err"`,
		`class="t-prompt"`,
		`applyFilter`,
		`id="filter"`,
	}
	for _, s := range must {
		if !strings.Contains(out, s) {
			t.Errorf("terminal output missing %q", s)
		}
	}
	// Monitor-only artefacts should not appear.
	if strings.Contains(out, "m-tile") {
		t.Error("terminal output should not contain monitor tiles")
	}
}

func TestHTML_UnknownStyleRejected(t *testing.T) {
	var buf bytes.Buffer
	err := HTML(&buf, nil, HTMLOptions{Style: "chartjs"})
	if err == nil || !strings.Contains(err.Error(), "unknown html style") {
		t.Fatalf("expected unknown-style error, got %v", err)
	}
}

func TestHTML_EmptyResults(t *testing.T) {
	var buf bytes.Buffer
	if err := HTML(&buf, nil, HTMLOptions{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "0 host(s)") {
		t.Errorf("expected '0 host(s)' in output")
	}
	if !strings.Contains(out, "</html>") {
		t.Error("template did not render completely")
	}
}
