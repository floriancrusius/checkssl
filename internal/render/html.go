package render

import (
	"fmt"
	"html/template"
	"io"
	"time"

	"github.com/floriancrusius/checkssl/internal/cert"
)

// HTMLOptions configures the HTML rendering. Zero values fall back to sane
// defaults; expose them so tests can pin the timestamp.
type HTMLOptions struct {
	Now         time.Time
	GeneratedAt time.Time
	Title       string
}

// htmlRow is what the template iterates over.
type htmlRow struct {
	Domain     string
	Status     string
	StatusText string
	Expiration string
	Days       string
	Issuer     string
	Note       string
}

type htmlPage struct {
	Title        string
	GeneratedAt  string
	Total        int
	Valid        int
	ExpiringSoon int
	Expired      int
	Invalid      int
	Errored      int
	Rows         []htmlRow
}

// HTML renders results as a self-contained HTML report with inline CSS and
// a small sortable-table script. Safe to attach to an email or drop into a
// static file server.
func HTML(w io.Writer, results []cert.Result, opts HTMLOptions) error {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	generated := opts.GeneratedAt
	if generated.IsZero() {
		generated = now
	}
	title := opts.Title
	if title == "" {
		title = "checkssl report"
	}

	page := htmlPage{
		Title:       title,
		GeneratedAt: generated.Format("2006-01-02 15:04:05 MST"),
		Total:       len(results),
	}
	for _, r := range results {
		row := htmlRow{
			Domain: r.Domain,
			Status: string(r.Status),
			Issuer: r.Issuer,
		}
		row.StatusText = statusText(r.Status)
		if r.ExpiresAt.IsZero() {
			row.Expiration = "—"
			row.Days = "—"
		} else {
			row.Expiration = formatDate(r.ExpiresAt)
			row.Days = fmt.Sprintf("%d", daysBetween(r.ExpiresAt, now))
		}
		switch {
		case r.Err != "":
			row.Note = r.Err
		case r.AuthError != "":
			row.Note = r.AuthError
		}

		switch r.Status {
		case cert.StatusValid:
			page.Valid++
		case cert.StatusExpiringSoon:
			page.ExpiringSoon++
		case cert.StatusExpired:
			page.Expired++
		case cert.StatusInvalid:
			page.Invalid++
		case cert.StatusError:
			page.Errored++
		}
		page.Rows = append(page.Rows, row)
	}

	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, page)
}

func statusText(s cert.Status) string {
	switch s {
	case cert.StatusValid:
		return "valid"
	case cert.StatusExpiringSoon:
		return "expiring soon"
	case cert.StatusExpired:
		return "expired"
	case cert.StatusInvalid:
		return "invalid"
	case cert.StatusError:
		return "error"
	default:
		return string(s)
	}
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
  :root {
    --bg: #fafafa; --fg: #222; --muted: #666; --border: #e2e2e2;
    --valid: #16a34a; --warn: #ca8a04; --crit: #dc2626; --err: #737373;
    --row-valid: #f0fdf4; --row-warn: #fefce8; --row-crit: #fef2f2; --row-err: #f5f5f5;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --bg: #0d1117; --fg: #e6edf3; --muted: #8b949e; --border: #30363d;
      --row-valid: #0f2417; --row-warn: #2a1f05; --row-crit: #2d1416; --row-err: #191c20;
    }
  }
  * { box-sizing: border-box; }
  body { margin: 0; padding: 2rem 1rem; background: var(--bg); color: var(--fg);
    font: 14px/1.5 -apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif; }
  main { max-width: 1100px; margin: 0 auto; }
  h1 { margin: 0 0 .25rem; font-size: 1.4rem; }
  .meta { color: var(--muted); margin: 0 0 1.5rem; font-size: .9rem; }
  .summary { display: flex; flex-wrap: wrap; gap: .5rem; margin: 0 0 1.5rem; }
  .chip { padding: .3rem .7rem; border-radius: 999px; font-size: .85rem;
    font-weight: 600; border: 1px solid var(--border); }
  .chip.valid    { color: var(--valid); }
  .chip.warn     { color: var(--warn);  }
  .chip.crit     { color: var(--crit);  }
  .chip.err      { color: var(--err);   }
  table { width: 100%; border-collapse: collapse; background: var(--bg);
    border: 1px solid var(--border); border-radius: 6px; overflow: hidden; }
  th, td { padding: .55rem .8rem; text-align: left; border-bottom: 1px solid var(--border);
    font-size: .92rem; vertical-align: top; }
  th { background: color-mix(in srgb, var(--bg) 60%, var(--fg) 8%);
    cursor: pointer; user-select: none; font-weight: 600; }
  th::after { content: ""; display: inline-block; width: 0; }
  th.sort-asc::after  { content: " ▲"; color: var(--muted); }
  th.sort-desc::after { content: " ▼"; color: var(--muted); }
  tbody tr.valid         { background: var(--row-valid); }
  tbody tr.expiring_soon { background: var(--row-warn);  }
  tbody tr.expired,
  tbody tr.invalid       { background: var(--row-crit);  }
  tbody tr.error         { background: var(--row-err);   }
  td.status { font-weight: 600; }
  td.status.valid         { color: var(--valid); }
  td.status.expiring_soon { color: var(--warn);  }
  td.status.expired,
  td.status.invalid       { color: var(--crit);  }
  td.status.error         { color: var(--err);   }
  td.days, td.expiration, th.days, th.expiration { font-variant-numeric: tabular-nums; white-space: nowrap; }
  td.note { color: var(--muted); font-size: .85rem; }
  @media print {
    body { background: white; color: black; padding: 0; }
    .summary .chip { border-color: #ccc; }
  }
</style>
</head>
<body>
<main>
  <h1>{{.Title}}</h1>
  <p class="meta">Generated {{.GeneratedAt}} · {{.Total}} domain(s)</p>
  <div class="summary">
    <span class="chip valid">{{.Valid}} valid</span>
    <span class="chip warn">{{.ExpiringSoon}} expiring soon</span>
    <span class="chip crit">{{.Expired}} expired</span>
    <span class="chip crit">{{.Invalid}} invalid</span>
    <span class="chip err">{{.Errored}} error</span>
  </div>
  <table id="report">
    <thead>
      <tr>
        <th data-type="text">Domain</th>
        <th data-type="text" class="status">Status</th>
        <th data-type="date" class="expiration">Expiration</th>
        <th data-type="num"  class="days">Days</th>
        <th data-type="text">Issuer</th>
        <th data-type="text">Note</th>
      </tr>
    </thead>
    <tbody>
    {{range .Rows}}
      <tr class="{{.Status}}">
        <td>{{.Domain}}</td>
        <td class="status {{.Status}}">{{.StatusText}}</td>
        <td class="expiration">{{.Expiration}}</td>
        <td class="days">{{.Days}}</td>
        <td>{{.Issuer}}</td>
        <td class="note">{{.Note}}</td>
      </tr>
    {{end}}
    </tbody>
  </table>
</main>
<script>
  // Minimal click-to-sort. Numeric columns use the raw number, date columns
  // parse dd.mm.yyyy, text uses locale compare.
  (function () {
    const table = document.getElementById('report');
    const tbody = table.tBodies[0];
    const headers = table.querySelectorAll('th');
    const parsers = {
      num:  v => v === '—' ? Number.POSITIVE_INFINITY : parseFloat(v),
      date: v => {
        if (v === '—') return Number.POSITIVE_INFINITY;
        const [d, m, y] = v.split('.').map(Number);
        return new Date(y, m - 1, d).getTime();
      },
      text: v => v.toLowerCase(),
    };
    headers.forEach((th, idx) => {
      th.addEventListener('click', () => {
        const type = th.dataset.type || 'text';
        const asc = !th.classList.contains('sort-asc');
        headers.forEach(h => h.classList.remove('sort-asc', 'sort-desc'));
        th.classList.add(asc ? 'sort-asc' : 'sort-desc');
        const rows = Array.from(tbody.rows);
        rows.sort((a, b) => {
          const av = parsers[type](a.cells[idx].textContent.trim());
          const bv = parsers[type](b.cells[idx].textContent.trim());
          if (av < bv) return asc ? -1 : 1;
          if (av > bv) return asc ? 1 : -1;
          return 0;
        });
        rows.forEach(r => tbody.appendChild(r));
      });
    });
  })();
</script>
</body>
</html>
`
