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
	ShowIP       bool
}

// Table renders results as an ASCII table with an "expires in" column.
func Table(w io.Writer, results []cert.Result, opts TableOptions) error {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	width := MaxDomainWidth(results)
	ipWidth := 0
	if opts.ShowIP {
		for _, r := range results {
			if l := len(r.ResolvedIP); l > ipWidth {
				ipWidth = l
			}
		}
		if ipWidth < 7 { // room for the "—" placeholder, which is 3 bytes in UTF-8
			ipWidth = 7
		}
	}

	sepLen := width + daysColWidth + 20
	if opts.ShowIP {
		sepLen += ipWidth + 3 // " | " plus the column itself
	}
	sep := strings.Repeat("=", sepLen)

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
		var row string
		if opts.ShowIP {
			ip := r.ResolvedIP
			if ip == "" {
				ip = "—"
			}
			row = fmt.Sprintf("| %-*s | %-*s | %s | %s |",
				width, r.Domain, ipWidth, ip, date, padded)
		} else {
			row = fmt.Sprintf("| %-*s | %s | %s |", width, r.Domain, date, padded)
		}
		if _, err := fmt.Fprintln(w, row); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, sep); err != nil {
		return err
	}
	return nil
}

// CSVOptions controls CSV rendering.
type CSVOptions struct {
	Now    time.Time
	ShowIP bool
}

// CSV renders results as CSV with a header row.
func CSV(w io.Writer, results []cert.Result, opts CSVOptions) error {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	cw := csv.NewWriter(w)
	header := []string{"Domain", "Expiration", "DaysUntilExpiry"}
	if opts.ShowIP {
		header = append([]string{"Domain", "IP"}, header[1:]...)
	}
	if err := cw.Write(header); err != nil {
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
		row := []string{r.Domain, expiration, days}
		if opts.ShowIP {
			row = []string{r.Domain, r.ResolvedIP, expiration, days}
		}
		if err := cw.Write(row); err != nil {
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
	ResolvedIP         string `json:"resolvedIP,omitempty"`
	Status             string `json:"status,omitempty"`
	AuthorizationError string `json:"authorizationError,omitempty"`
	Error              string `json:"error,omitempty"`
}

// JSONOptions controls JSON rendering.
type JSONOptions struct {
	Now    time.Time
	ShowIP bool
}

// JSON renders results as a pretty-printed JSON array.
func JSON(w io.Writer, results []cert.Result, opts JSONOptions) error {
	now := opts.Now
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
		rec := JSONRecord{
			Domain:             r.Domain,
			Expiration:         expiration,
			DaysUntilExpiry:    days,
			Status:             string(r.Status),
			AuthorizationError: r.AuthError,
			Error:              r.Err,
		}
		if opts.ShowIP {
			rec.ResolvedIP = r.ResolvedIP
		}
		records = append(records, rec)
	}
	buf, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(buf, '\n'))
	return err
}

// NagiosStatus is the plugin exit-code contract Nagios / Icinga / CheckMK /
// Sensu all speak: 0 OK, 1 WARNING, 2 CRITICAL, 3 UNKNOWN.
type NagiosStatus int

const (
	NagiosOK       NagiosStatus = 0
	NagiosWarning  NagiosStatus = 1
	NagiosCritical NagiosStatus = 2
	NagiosUnknown  NagiosStatus = 3
)

// NagiosOptions tunes the thresholds used by Nagios(); zero values fall back
// to sensible defaults (14d critical, 30d warning).
type NagiosOptions struct {
	Now          time.Time
	WarningDays  int
	CriticalDays int
}

// Nagios renders a single-line Nagios-compatible plugin output and returns
// the status code the caller should exit with:
//
//	STATE - human message | perfdata_key=value;warn;crit ...
//
// State is derived from the worst result in `results`: any error/expired ⇒
// CRITICAL, any invalid ⇒ CRITICAL, any expiry within critical window ⇒
// CRITICAL, any within warning window ⇒ WARNING, else OK.
func Nagios(w io.Writer, results []cert.Result, opts NagiosOptions) (NagiosStatus, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	warnDays := opts.WarningDays
	if warnDays <= 0 {
		warnDays = 30
	}
	critDays := opts.CriticalDays
	if critDays <= 0 {
		critDays = 14
	}

	if len(results) == 0 {
		_, err := fmt.Fprintln(w, "UNKNOWN - no domains checked")
		return NagiosUnknown, err
	}

	// Aggregate.
	var (
		state           = NagiosOK
		nErr, nExp      int
		nInvalid        int
		nCrit, nWarn    int
		nOK             int
		minDaysSet      bool
		minDays         int
		worstDomain     string
		worstDomainDays int
	)
	for _, r := range results {
		switch r.Status {
		case cert.StatusError:
			nErr++
			bumpWorst(&state, NagiosCritical)
		case cert.StatusExpired:
			nExp++
			bumpWorst(&state, NagiosCritical)
		case cert.StatusInvalid:
			nInvalid++
			bumpWorst(&state, NagiosCritical)
		default:
			// valid / expiring_soon — decide by days remaining vs. thresholds.
			d := daysBetween(r.ExpiresAt, now)
			switch {
			case d <= critDays:
				nCrit++
				bumpWorst(&state, NagiosCritical)
			case d <= warnDays:
				nWarn++
				bumpWorst(&state, NagiosWarning)
			default:
				nOK++
			}
			if !minDaysSet || d < minDays {
				minDays = d
				minDaysSet = true
				worstDomain = r.Domain
				worstDomainDays = d
			}
		}
	}

	// Human message.
	var msg string
	switch state {
	case NagiosOK:
		msg = fmt.Sprintf("OK - %d cert(s) valid, next expiry %s in %d days",
			len(results), worstDomain, worstDomainDays)
	case NagiosWarning:
		msg = fmt.Sprintf("WARNING - %d expiring within %dd (next: %s in %d days)",
			nWarn+nCrit, warnDays, worstDomain, worstDomainDays)
	case NagiosCritical:
		parts := []string{}
		if nErr > 0 {
			parts = append(parts, fmt.Sprintf("%d error", nErr))
		}
		if nExp > 0 {
			parts = append(parts, fmt.Sprintf("%d expired", nExp))
		}
		if nInvalid > 0 {
			parts = append(parts, fmt.Sprintf("%d invalid", nInvalid))
		}
		if nCrit > 0 {
			parts = append(parts, fmt.Sprintf("%d expiring within %dd", nCrit, critDays))
		}
		msg = fmt.Sprintf("CRITICAL - %s", strings.Join(parts, ", "))
		if worstDomain != "" {
			msg += fmt.Sprintf(" (next: %s in %d days)", worstDomain, worstDomainDays)
		}
	default:
		msg = "UNKNOWN"
	}

	// Perfdata.
	perf := fmt.Sprintf(
		"total=%d valid=%d warning=%d critical=%d expired=%d invalid=%d error=%d",
		len(results), nOK, nWarn, nCrit, nExp, nInvalid, nErr,
	)
	if minDaysSet {
		perf += fmt.Sprintf(" min_days=%d;%d;%d", minDays, warnDays, critDays)
	}

	_, err := fmt.Fprintf(w, "%s | %s\n", msg, perf)
	return state, err
}

func bumpWorst(current *NagiosStatus, candidate NagiosStatus) {
	if candidate > *current {
		*current = candidate
	}
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
