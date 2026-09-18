// Package render turns cert.Result slices into human- or machine-readable
// output. All rendering functions are pure — they take a fixed "now" so
// tests can assert exact strings.
package render

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/floriancrusius/checkssl/internal/cert"
)

const (
	minDomainWidth = 10
	daysColWidth   = 17
	dateFormat     = "02.01.2006" // Go's magic reference date → dd.mm.yyyy
	errorLabel     = "   Error  "
)

// ANSI escape sequences. Callers decide whether to emit them via ColorEnabled.
const (
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiGreen  = "\x1b[32m"
	ansiDim    = "\x1b[2m"
	ansiReset  = "\x1b[0m"
)

// SortByExpiry returns results sorted by ExpiresAt ascending. Errors (zero
// ExpiresAt) always go to the end, whatever the direction.
func SortByExpiry(results []cert.Result) []cert.Result {
	out := make([]cert.Result, len(results))
	copy(out, results)
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := out[i].ExpiresAt.IsZero(), out[j].ExpiresAt.IsZero()
		if ai && aj {
			return false
		}
		if ai {
			return false
		}
		if aj {
			return true
		}
		return out[i].ExpiresAt.Before(out[j].ExpiresAt)
	})
	return out
}

// MaxDomainWidth is a helper for rendering — the widest domain in the slice,
// clamped to minDomainWidth.
func MaxDomainWidth(results []cert.Result) int {
	max := minDomainWidth
	for _, r := range results {
		if len(r.Domain) > max {
			max = len(r.Domain)
		}
	}
	return max
}

// TableOptions controls table rendering.
type TableOptions struct {
	Now          time.Time
	ColorEnabled bool
}

// Table renders results as an ASCII table with an "expires in" column.
func Table(w io.Writer, results []cert.Result, opts TableOptions) error {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	width := MaxDomainWidth(results)
	sep := strings.Repeat("=", width+daysColWidth+20)

	if _, err := fmt.Fprintln(w, sep); err != nil {
		return err
	}
	for _, r := range results {
		date := formatDate(r.ExpiresAt)
		if date == "" {
			date = errorLabel
		}
		days := formatDays(r.ExpiresAt, now)
		padded := padStart(days, daysColWidth)
		if opts.ColorEnabled {
			padded = colorFor(r.Status) + padded + ansiReset
		}
		row := fmt.Sprintf("| %-*s | %s | %s |", width, r.Domain, date, padded)
		if _, err := fmt.Fprintln(w, row); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, sep); err != nil {
		return err
	}
	return nil
}

// CSV renders results as CSV with a header row.
func CSV(w io.Writer, results []cert.Result, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"Domain", "Expiration", "DaysUntilExpiry"}); err != nil {
		return err
	}
	for _, r := range results {
		expiration := formatDate(r.ExpiresAt)
		if expiration == "" {
			expiration = "Error"
		}
		days := ""
		if !r.ExpiresAt.IsZero() {
			days = fmt.Sprintf("%d", daysBetween(r.ExpiresAt, now))
		}
		if err := cw.Write([]string{r.Domain, expiration, days}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// JSONRecord is the on-the-wire shape of a JSON output entry.
type JSONRecord struct {
	Domain             string `json:"domain"`
	Expiration         string `json:"expiration"`
	DaysUntilExpiry    *int   `json:"daysUntilExpiry"`
	Status             string `json:"status,omitempty"`
	AuthorizationError string `json:"authorizationError,omitempty"`
	Error              string `json:"error,omitempty"`
}

// JSON renders results as a pretty-printed JSON array.
func JSON(w io.Writer, results []cert.Result, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	records := make([]JSONRecord, 0, len(results))
	for _, r := range results {
		expiration := formatDate(r.ExpiresAt)
		if expiration == "" {
			expiration = "Error"
		}
		var days *int
		if !r.ExpiresAt.IsZero() {
			d := daysBetween(r.ExpiresAt, now)
			days = &d
		}
		records = append(records, JSONRecord{
			Domain:             r.Domain,
			Expiration:         expiration,
			DaysUntilExpiry:    days,
			Status:             string(r.Status),
			AuthorizationError: r.AuthError,
			Error:              r.Err,
		})
	}
	buf, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(buf, '\n'))
	return err
}

// PrintErrors writes an error summary to w. No-op for an empty slice.
func PrintErrors(w io.Writer, errs []string) {
	if len(errs) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "❌ Errors encountered:")
	fmt.Fprintln(w)
	for _, e := range errs {
		fmt.Fprintf(w, "   %s\n", e)
	}
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dateFormat)
}

func daysBetween(expiresAt, now time.Time) int {
	// Match the JS implementation: Math.ceil((expiry - now) / day).
	diff := expiresAt.Sub(now).Hours() / 24
	return int(math.Ceil(diff))
}

func formatDays(expiresAt, now time.Time) string {
	if expiresAt.IsZero() {
		return "N/A"
	}
	days := daysBetween(expiresAt, now)
	switch {
	case days < 0:
		return fmt.Sprintf("expired %dd ago", -days)
	case days == 0:
		return "today"
	case days == 1:
		return "in 1 day"
	default:
		return fmt.Sprintf("in %d days", days)
	}
}

func colorFor(status cert.Status) string {
	switch status {
	case cert.StatusExpired, cert.StatusInvalid:
		return ansiRed
	case cert.StatusExpiringSoon:
		return ansiYellow
	case cert.StatusValid:
		return ansiGreen
	default:
		return ansiDim
	}
}

func padStart(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

// Buffer is a tiny convenience so callers can render into a string.
func Buffer(fn func(w io.Writer) error) (string, error) {
	var buf bytes.Buffer
	if err := fn(&buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
