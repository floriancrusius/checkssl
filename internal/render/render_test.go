package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/floriancrusius/checkssl/internal/cert"
)

var testNow = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

func sample() []cert.Result {
	return []cert.Result{
		{Domain: "example.com", ExpiresAt: time.Date(2025, 2, 12, 0, 0, 0, 0, time.UTC), Status: cert.StatusValid},
		{Domain: "test.org", ExpiresAt: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC), Status: cert.StatusValid},
	}
}

func TestSortByExpiry_ErrorsLast(t *testing.T) {
	results := []cert.Result{
		{Domain: "later.com", ExpiresAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Domain: "earlier.com", ExpiresAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Domain: "error.com", Status: cert.StatusError},
	}
	sorted := SortByExpiry(results)
	got := []string{sorted[0].Domain, sorted[1].Domain, sorted[2].Domain}
	want := []string{"earlier.com", "later.com", "error.com"}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("sort order = %v, want %v", got, want)
		}
	}
}

func TestSortByExpiry_DoesNotMutate(t *testing.T) {
	results := []cert.Result{
		{Domain: "b.com", ExpiresAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Domain: "a.com", ExpiresAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	SortByExpiry(results)
	if results[0].Domain != "b.com" {
		t.Fatalf("original mutated: got %v", results)
	}
}

func TestTable_RendersDaysColumn(t *testing.T) {
	var buf bytes.Buffer
	if err := Table(&buf, sample(), TableOptions{Now: testNow}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	wantContains := []string{
		"| example.com |",
		"12.02.2025",
		"in 42 days",
		"| test.org    |",
		"in 397 days",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Errorf("output missing %q\ngot:\n%s", s, got)
		}
	}
}

func TestTable_ErrorRow(t *testing.T) {
	results := []cert.Result{{Domain: "broken.com", Status: cert.StatusError, Err: "boom"}}
	var buf bytes.Buffer
	if err := Table(&buf, results, TableOptions{Now: testNow}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, errorLabel) {
		t.Errorf("expected error label in output:\n%s", got)
	}
	if !strings.Contains(got, "N/A") {
		t.Errorf("expected N/A in output:\n%s", got)
	}
}

func TestTable_ColorWrapsWhenEnabled(t *testing.T) {
	results := []cert.Result{
		{Domain: "gone.com", ExpiresAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Status: cert.StatusExpired},
	}
	var buf bytes.Buffer
	if err := Table(&buf, results, TableOptions{Now: testNow, ColorEnabled: true}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, ansiRed) || !strings.Contains(got, ansiReset) {
		t.Errorf("expected ANSI red + reset:\n%s", got)
	}
}

func TestTable_ColorSuppressedByDefault(t *testing.T) {
	var buf bytes.Buffer
	if err := Table(&buf, sample(), TableOptions{Now: testNow}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("expected no ANSI escapes without ColorEnabled:\n%s", buf.String())
	}
}

func TestCSV_HeaderAndRows(t *testing.T) {
	var buf bytes.Buffer
	if err := CSV(&buf, sample(), testNow); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\r\n"), "\n")
	if lines[0] != "Domain,Expiration,DaysUntilExpiry" {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.Contains(lines[1], "example.com,12.02.2025,42") {
		t.Errorf("row 1 = %q", lines[1])
	}
	if !strings.Contains(lines[2], "test.org,02.02.2026,397") {
		t.Errorf("row 2 = %q", lines[2])
	}
}

func TestCSV_ErrorRow(t *testing.T) {
	results := []cert.Result{{Domain: "broken.com", Status: cert.StatusError}}
	var buf bytes.Buffer
	if err := CSV(&buf, results, testNow); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\r\n"), "\n")
	if !strings.Contains(lines[1], "broken.com,Error,") {
		t.Errorf("row = %q", lines[1])
	}
}

func TestJSON_HappyPath(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sample(), testNow); err != nil {
		t.Fatal(err)
	}
	var records []JSONRecord
	if err := json.Unmarshal(buf.Bytes(), &records); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(records) != 2 {
		t.Fatalf("record count = %d", len(records))
	}
	if records[0].Domain != "example.com" || records[0].Expiration != "12.02.2025" {
		t.Errorf("record 0 = %+v", records[0])
	}
	if records[0].DaysUntilExpiry == nil || *records[0].DaysUntilExpiry != 42 {
		t.Errorf("days = %v, want 42", records[0].DaysUntilExpiry)
	}
	if records[0].Status != string(cert.StatusValid) {
		t.Errorf("status = %q", records[0].Status)
	}
}

func TestJSON_ErrorFieldsAndNullDays(t *testing.T) {
	results := []cert.Result{{Domain: "broken.com", Status: cert.StatusError, Err: "boom"}}
	var buf bytes.Buffer
	if err := JSON(&buf, results, testNow); err != nil {
		t.Fatal(err)
	}
	// Assert on the raw JSON so we can verify null vs. missing.
	if !strings.Contains(buf.String(), `"daysUntilExpiry": null`) {
		t.Errorf("expected daysUntilExpiry: null\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"error": "boom"`) {
		t.Errorf("expected error field\n%s", buf.String())
	}
}

func TestPrintErrors_Noop(t *testing.T) {
	var buf bytes.Buffer
	PrintErrors(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("expected no output, got %q", buf.String())
	}
}

func TestPrintErrors_WritesHeaderAndItems(t *testing.T) {
	var buf bytes.Buffer
	PrintErrors(&buf, []string{"a", "b"})
	got := buf.String()
	if !strings.Contains(got, "Errors encountered") ||
		!strings.Contains(got, "   a") || !strings.Contains(got, "   b") {
		t.Errorf("unexpected output:\n%s", got)
	}
}
