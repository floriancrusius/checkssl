package render

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"sort"
	"time"

	"github.com/floriancrusius/checkssl/internal/cert"
)

// HTML output styles.
const (
	HTMLStyleMonitor  = "monitor"  // dashboard-style default
	HTMLStyleTerminal = "terminal" // monospace terminal mock
)

// HTMLOptions configures the HTML rendering. Zero values fall back to sane
// defaults; expose them so tests can pin the timestamp.
type HTMLOptions struct {
	Now         time.Time
	GeneratedAt time.Time
	Title       string
	ShowIP      bool
	// Style selects the visual direction. Empty string == HTMLStyleMonitor.
	Style string
}

// htmlRow is what the template iterates over.
type htmlRow struct {
	Domain      string
	IP          string
	Status      string // raw cert.Status value, drives filter chips
	StatusText  string // human label used by the monitor style
	StatusLabel string // short label used by the terminal style ("OK", "WARN", …)
	StatusCls   string // CSS class the terminal style attaches to the label
	Marker      string // leading glyph used by the terminal style
	Expiration  string
	Days        string
	DaysInt     int
	HasDays     bool
	BarPct      int
	Issuer      string
	Note        string
	VisualClass string // row class; splits expiring_soon into "hot" when ≤14d
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
	NextDomain   string // longest-running cert most at risk (first positive-days row)
	NextDays     int
	HasNext      bool
	AvgDays      string // average positive days, used by the terminal footer
	ShowIP       bool
	Rows         []htmlRow
}

// HTML renders results as a self-contained HTML report. The output is a
// single standalone file with inline CSS/JS, safe to attach to an email
// or drop into a static file server. The visual direction is picked by
// opts.Style.
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
	style := opts.Style
	if style == "" {
		style = HTMLStyleMonitor
	}

	page := htmlPage{
		Title:       title,
		GeneratedAt: generated.Format("2006-01-02 15:04:05 MST"),
		Total:       len(results),
		ShowIP:      opts.ShowIP,
	}
	for _, r := range results {
		row := htmlRow{
			Domain:     r.Domain,
			IP:         r.ResolvedIP,
			Status:     string(r.Status),
			StatusText: statusText(r.Status),
			Issuer:     r.Issuer,
		}
		if r.ExpiresAt.IsZero() {
			row.Expiration = "—"
			row.Days = "—"
		} else {
			row.Expiration = formatDate(r.ExpiresAt)
			d := daysBetween(r.ExpiresAt, now)
			row.Days = fmt.Sprintf("%d", d)
			row.DaysInt = d
			row.HasDays = true
			row.BarPct = computeBarPct(d)
		}
		switch {
		case r.Err != "":
			row.Note = r.Err
		case r.AuthError != "":
			row.Note = r.AuthError
		}

		row.VisualClass = visualClass(r.Status, row.DaysInt, row.HasDays)
		row.Marker, row.StatusLabel, row.StatusCls = terminalLabel(r.Status, row.VisualClass)

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

	// Next-expiry: smallest positive days value in the set. Used by the
	// monitor's fourth tile and (indirectly) by the terminal summary.
	pos := make([]int, 0, len(results))
	for _, r := range page.Rows {
		if r.HasDays && r.DaysInt > 0 {
			pos = append(pos, r.DaysInt)
		}
	}
	if len(pos) > 0 {
		sort.Ints(pos)
		page.NextDays = pos[0]
		page.HasNext = true
		for _, r := range page.Rows {
			if r.HasDays && r.DaysInt == pos[0] {
				page.NextDomain = r.Domain
				break
			}
		}
		sum := 0
		for _, d := range pos {
			sum += d
		}
		page.AvgDays = fmt.Sprintf("%dd", int(math.Round(float64(sum)/float64(len(pos)))))
	} else {
		page.AvgDays = "—"
	}

	var tmplText string
	switch style {
	case HTMLStyleTerminal:
		tmplText = htmlTerminalTemplate
	case HTMLStyleMonitor:
		tmplText = htmlMonitorTemplate
	default:
		return fmt.Errorf("unknown html style %q (expected %q or %q)",
			style, HTMLStyleMonitor, HTMLStyleTerminal)
	}

	funcs := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"neg": func(a int) int { return -a },
	}
	tmpl, err := template.New("report").Funcs(funcs).Parse(tmplText)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, page)
}

// computeBarPct maps a day count to a 0-100% bar width. Expired/invalid
// rows render as a full-width red bar; valid rows top out at 180 days.
func computeBarPct(days int) int {
	if days <= 0 {
		return 100
	}
	pct := int(math.Round(float64(days) / 180.0 * 100.0))
	switch {
	case pct > 100:
		return 100
	case pct < 6:
		return 6
	}
	return pct
}

// visualClass escalates expiring_soon to "hot" at ≤14 days so the row can
// be coloured more urgently without changing its filter bucket.
func visualClass(status cert.Status, days int, hasDays bool) string {
	if status == cert.StatusExpiringSoon && hasDays && days <= 14 {
		return "hot"
	}
	return string(status)
}

func terminalLabel(status cert.Status, visual string) (marker, label, cls string) {
	switch status {
	case cert.StatusValid:
		return "●", "OK", "ok"
	case cert.StatusExpiringSoon:
		if visual == "hot" {
			return "●", "WARN", "hot"
		}
		return "●", "WARN", "warn"
	case cert.StatusExpired:
		return "✖", "FAIL", "crit"
	case cert.StatusInvalid:
		return "✖", "FAIL", "crit"
	case cert.StatusError:
		return "○", "DOWN", "err"
	default:
		return "·", string(status), ""
	}
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

// ===========================================================================
//  Monitor template — dense dashboard with metric tiles, severity stripes
//  and a visual bar for days-until-expiry.
// ===========================================================================

const htmlMonitorTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>{{.Title}}</title>
<style>
  :root {
    --bg:        #f6f7f9;
    --surface:   #ffffff;
    --surface-2: #f1f3f6;
    --fg:        #0f1419;
    --muted:     #5a6472;
    --border:    #e2e5ea;
    --accent:    #2563eb;
    --ok:        #16a34a;
    --warn:      #b4810c;
    --warn-hot:  #d05825;
    --crit:      #c92a2a;
    --err:       #6b7280;
    color-scheme: light;
  }
  @media (prefers-color-scheme: dark) {
    :root:not([data-theme="light"]) {
      --bg:        #0b0f14;
      --surface:   #141922;
      --surface-2: #1b222d;
      --fg:        #d8dee6;
      --muted:     #7a8290;
      --border:    #232a35;
      --accent:    #58a6ff;
      --ok:        #3fb950;
      --warn:      #d2a85b;
      --warn-hot:  #e8805c;
      --crit:      #e8524c;
      --err:       #7b8390;
      color-scheme: dark;
    }
  }
  :root[data-theme="dark"] {
    --bg:        #0b0f14;
    --surface:   #141922;
    --surface-2: #1b222d;
    --fg:        #d8dee6;
    --muted:     #7a8290;
    --border:    #232a35;
    --accent:    #58a6ff;
    --ok:        #3fb950;
    --warn:      #d2a85b;
    --warn-hot:  #e8805c;
    --crit:      #e8524c;
    --err:       #7b8390;
    color-scheme: dark;
  }
  * { box-sizing: border-box; min-width: 0; }
  body {
    margin: 0;
    background: var(--bg);
    color: var(--fg);
    font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Inter, sans-serif;
    -webkit-font-smoothing: antialiased;
    padding: 24px clamp(12px, 2vw, 32px);
  }
  main { max-width: 1200px; margin: 0 auto; }
  .m-head {
    display: flex; flex-wrap: wrap; gap: 10px 20px;
    justify-content: space-between; align-items: baseline;
    padding: 0 2px 14px;
  }
  .m-head h1 {
    margin: 0; font-size: 16px; font-weight: 600; letter-spacing: -0.01em;
  }
  .m-head h1 .dot {
    display: inline-block; width: 8px; height: 8px; border-radius: 50%;
    background: var(--ok); margin-right: 8px; vertical-align: middle;
    box-shadow: 0 0 0 3px color-mix(in oklab, var(--ok), transparent 75%);
  }
  .m-head .m-meta {
    font: 500 12px/1 ui-monospace, "JetBrains Mono", "SF Mono", Menlo, monospace;
    color: var(--muted); letter-spacing: 0.02em;
  }

  .m-card {
    background: var(--surface); border: 1px solid var(--border);
    border-radius: 12px; overflow: hidden;
  }
  .m-tiles {
    display: grid; grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 1px; background: var(--border);
  }
  .m-tile {
    background: var(--surface); padding: 18px 20px 16px;
    display: flex; flex-direction: column; gap: 4px; cursor: pointer;
    user-select: none;
    transition: background .1s ease;
  }
  .m-tile:hover { background: var(--surface-2); }
  .m-tile .n {
    font: 600 28px/1 inherit; letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
  }
  .m-tile .n .delta {
    margin-left: 6px; font-size: 12px; font-weight: 500; color: var(--muted);
  }
  .m-tile .l {
    font-size: 11px; color: var(--muted); text-transform: uppercase;
    letter-spacing: 0.08em; font-weight: 500;
  }
  .m-tile[data-tone="warn"] .n { color: var(--warn); }
  .m-tile[data-tone="crit"] .n { color: var(--crit); }

  .m-toolbar {
    margin-top: 1px;
    padding: 12px 20px; display: flex; flex-wrap: wrap; gap: 10px;
    align-items: center; border-top: 1px solid var(--border);
    background: var(--surface);
  }
  .m-toolbar input[type="search"] {
    flex: 1 1 220px; min-width: 0; background: var(--bg); color: var(--fg);
    border: 1px solid var(--border); border-radius: 6px; padding: 7px 10px;
    font: 500 13px/1.4 inherit;
  }
  .m-toolbar input[type="search"]::placeholder { color: var(--muted); }
  .m-toolbar input[type="search"]:focus-visible {
    outline: none; border-color: var(--accent);
    box-shadow: 0 0 0 2px color-mix(in oklab, var(--accent), transparent 70%);
  }
  .m-filters { display: flex; flex-wrap: wrap; gap: 6px; }
  .m-filter {
    appearance: none; background: transparent; color: var(--muted);
    border: 1px solid var(--border); border-radius: 999px;
    padding: 5px 11px; font: 500 11px/1 inherit;
    text-transform: uppercase; letter-spacing: 0.06em; cursor: pointer;
    transition: color .1s ease, background .1s ease, border-color .1s ease;
  }
  .m-filter.on, .m-filter:hover { color: var(--fg); background: var(--surface-2); }
  .m-filter.off { opacity: 0.5; }
  .m-filter.valid.on    { border-color: var(--ok); }
  .m-filter.warn.on     { border-color: var(--warn); }
  .m-filter.crit.on     { border-color: var(--crit); }
  .m-filter.err.on      { border-color: var(--err); }
  .m-toolbar .right { margin-left: auto; display: flex; gap: 10px; align-items: center; }
  .m-toolbar .count { color: var(--muted); font-size: 12px; white-space: nowrap; }
  .m-toolbar button.reset {
    appearance: none; background: transparent; color: var(--fg);
    border: 1px solid var(--border); border-radius: 6px;
    padding: 6px 10px; font: 500 12px/1 inherit; cursor: pointer;
  }

  .m-list { border-top: 1px solid var(--border); }
  .m-row {
    display: grid;
    grid-template-columns: 4px minmax(0, 2.6fr){{if .ShowIP}} minmax(130px, 1.2fr){{end}} minmax(0, 1.1fr) 70px minmax(140px, 1.6fr) minmax(0, 1.2fr);
    gap: 16px; padding-right: 20px; align-items: center;
    border-bottom: 1px solid var(--border); min-height: 56px;
    background: var(--surface);
  }
  .m-row:last-child { border-bottom: 0; }
  .m-row:hover { background: color-mix(in oklab, var(--surface), var(--surface-2) 50%); }
  .m-stripe { align-self: stretch; background: var(--err); }
  .m-row[data-s="valid"]         .m-stripe { background: var(--ok); }
  .m-row[data-s="expiring_soon"] .m-stripe { background: var(--warn); }
  .m-row[data-s="hot"]           .m-stripe { background: var(--warn-hot); }
  .m-row[data-s="expired"]       .m-stripe,
  .m-row[data-s="invalid"]       .m-stripe { background: var(--crit); }
  .m-row[data-s="error"]         .m-stripe { background: var(--err); }

  .m-domain {
    display: flex; flex-direction: column; gap: 2px; padding: 12px 0 12px 0;
  }
  .m-domain .d {
    font-weight: 500; color: var(--fg); font-size: 14px;
    overflow-wrap: anywhere;
  }
  .m-domain .i {
    font: 500 11px/1 ui-monospace, "JetBrains Mono", "SF Mono", Menlo, monospace;
    color: var(--muted); letter-spacing: 0.02em;
  }
  .m-ip, .m-expires {
    font: 500 12px/1 ui-monospace, "JetBrains Mono", "SF Mono", Menlo, monospace;
    color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .m-days {
    font: 600 15px/1 ui-monospace, "JetBrains Mono", "SF Mono", Menlo, monospace;
    font-variant-numeric: tabular-nums; text-align: right;
  }
  .m-row[data-s="valid"]         .m-days { color: var(--ok); }
  .m-row[data-s="expiring_soon"] .m-days { color: var(--warn); }
  .m-row[data-s="hot"]           .m-days { color: var(--warn-hot); }
  .m-row[data-s="expired"]       .m-days,
  .m-row[data-s="invalid"]       .m-days { color: var(--crit); }
  .m-row[data-s="error"]         .m-days { color: var(--err); }

  .m-bar {
    position: relative; height: 6px;
    background: color-mix(in oklab, var(--border), transparent 20%);
    border-radius: 3px; overflow: hidden;
  }
  .m-bar > span {
    position: absolute; top: 0; bottom: 0; left: 0;
    background: currentColor; border-radius: 3px;
  }
  .m-row[data-s="valid"]         .m-bar { color: var(--ok); }
  .m-row[data-s="expiring_soon"] .m-bar { color: var(--warn); }
  .m-row[data-s="hot"]           .m-bar { color: var(--warn-hot); }
  .m-row[data-s="expired"]       .m-bar,
  .m-row[data-s="invalid"]       .m-bar,
  .m-row[data-s="error"]         .m-bar { color: var(--crit); }

  .m-issuer {
    font-size: 12px; color: var(--muted);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .m-issuer .note {
    display: block; color: var(--crit);
    font: 500 11px/1.3 ui-monospace, "JetBrains Mono", "SF Mono", Menlo, monospace;
    margin-top: 2px; white-space: normal; overflow-wrap: anywhere;
  }

  @media (max-width: 820px) {
    .m-tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .m-row {
      grid-template-columns: 4px 1fr auto;
      grid-template-areas:
        "s dom days"
        "s ip  bar"
        "s exp exp"
        "s iss iss";
      row-gap: 2px; padding: 10px 16px 12px 0;
    }
    .m-stripe   { grid-area: s; }
    .m-domain   { grid-area: dom; padding: 0; }
    {{if .ShowIP}}.m-ip      { grid-area: ip; }{{end}}
    .m-expires  { grid-area: exp; }
    .m-days     { grid-area: days; align-self: start; }
    .m-bar      { grid-area: bar; width: 100px; justify-self: end; }
    .m-issuer   { grid-area: iss; font-size: 11px; margin-top: 4px; }
  }

  @media print {
    body { background: white; color: black; }
    .m-toolbar { display: none; }
    .m-row:hover { background: transparent; }
  }
</style>
</head>
<body>
<main>
  <div class="m-head">
    <h1><span class="dot" id="statusDot"></span>{{.Title}}</h1>
    <span class="m-meta">{{.GeneratedAt}} · {{.Total}} host(s)</span>
  </div>

  <div class="m-card">
    <div class="m-tiles">
      <button type="button" class="m-tile" data-status="valid" title="Toggle">
        <span class="n">{{.Valid}}<span class="delta">/ {{.Total}}</span></span>
        <span class="l">{{.Valid}} valid</span>
      </button>
      <button type="button" class="m-tile" data-tone="warn" data-status="expiring_soon" title="Toggle">
        <span class="n">{{.ExpiringSoon}}</span>
        <span class="l">{{.ExpiringSoon}} expiring soon</span>
      </button>
      <button type="button" class="m-tile" data-tone="crit" data-status="expired invalid" title="Toggle">
        <span class="n">{{add .Expired .Invalid}}</span>
        <span class="l">{{.Expired}} expired · {{.Invalid}} invalid · {{.Errored}} error</span>
      </button>
      <button type="button" class="m-tile" data-status="" title="Next expiry">
        <span class="n">{{if .HasNext}}{{.NextDays}}d{{else}}—{{end}}</span>
        <span class="l">Next expiry · {{if .HasNext}}{{.NextDomain}}{{else}}—{{end}}</span>
      </button>
    </div>
    <div class="m-toolbar">
      <input type="search" id="filter" placeholder="Filter domain, IP, issuer, note…" autocomplete="off">
      <div class="m-filters">
        <button type="button" class="m-filter on valid" data-status="valid">valid</button>
        <button type="button" class="m-filter on warn"  data-status="expiring_soon">warning</button>
        <button type="button" class="m-filter on crit"  data-status="expired">expired</button>
        <button type="button" class="m-filter on crit"  data-status="invalid">invalid</button>
        <button type="button" class="m-filter on err"   data-status="error">error</button>
      </div>
      <div class="right">
        <span class="count" id="count"></span>
        <button type="button" class="reset" id="reset">Reset</button>
      </div>
    </div>

    <div class="m-list" id="report">
      {{$showIP := .ShowIP}}
      {{range .Rows}}
      <div class="m-row" data-s="{{.VisualClass}}" data-status="{{.Status}}">
        <div class="m-stripe"></div>
        <div class="m-domain">
          <span class="d">{{.Domain}}</span>
          <span class="i">{{.Expiration}}</span>
        </div>
        {{if $showIP}}<div class="m-ip">{{if .IP}}{{.IP}}{{else}}—{{end}}</div>{{end}}
        <div class="m-expires"></div>
        <div class="m-days">{{if .HasDays}}{{if lt .DaysInt 0}}{{neg .DaysInt}}d ago{{else}}{{.DaysInt}}d{{end}}{{else}}—{{end}}</div>
        <div class="m-bar"><span style="width:{{.BarPct}}%"></span></div>
        <div class="m-issuer">{{if .Issuer}}{{.Issuer}}{{else}}—{{end}}{{if .Note}}<span class="note">{{.Note}}</span>{{end}}</div>
      </div>
      {{end}}
    </div>
  </div>
</main>
<script>
  (function () {
    const rows    = Array.from(document.querySelectorAll(".m-row"));
    const chips   = document.querySelectorAll(".m-filter[data-status]");
    const tiles   = document.querySelectorAll(".m-tile[data-status]");
    const search  = document.getElementById("filter");
    const reset   = document.getElementById("reset");
    const count   = document.getElementById("count");
    const dot     = document.getElementById("statusDot");
    const total   = rows.length;

    const active = new Set();
    chips.forEach(c => active.add(c.dataset.status));

    function cellsText(row) {
      return Array.from(row.children).map(c => c.textContent).join(" ").toLowerCase();
    }

    function applyFilter() {
      const needle = search.value.trim().toLowerCase();
      let shown = 0;
      for (const row of rows) {
        const statusOK = active.has(row.dataset.status);
        const textOK = !needle || cellsText(row).includes(needle);
        const visible = statusOK && textOK;
        row.hidden = !visible;
        if (visible) shown++;
      }
      count.textContent = shown === total
        ? shown + " / " + total + " shown"
        : shown + " / " + total + " shown (filtered)";

      // Dot colour tracks worst visible status.
      let worst = "ok";
      for (const row of rows) {
        if (row.hidden) continue;
        const s = row.dataset.status;
        if (s === "error" || s === "expired" || s === "invalid") { worst = "crit"; break; }
        if (s === "expiring_soon") worst = "warn";
      }
      const map = { ok: "var(--ok)", warn: "var(--warn)", crit: "var(--crit)" };
      dot.style.background = map[worst];
      dot.style.boxShadow = "0 0 0 3px color-mix(in oklab, " + map[worst] + ", transparent 75%)";
    }

    chips.forEach(chip => {
      chip.addEventListener("click", () => {
        const s = chip.dataset.status;
        if (active.has(s)) { active.delete(s); chip.classList.add("off"); chip.classList.remove("on"); }
        else               { active.add(s);    chip.classList.remove("off"); chip.classList.add("on"); }
        applyFilter();
      });
    });

    // Metric tiles double as quick filters: click to isolate that bucket,
    // click again to restore everything.
    tiles.forEach(tile => {
      const statuses = (tile.dataset.status || "").trim().split(/\s+/).filter(Boolean);
      if (statuses.length === 0) return;
      tile.addEventListener("click", () => {
        const isolated = statuses.length === active.size &&
          statuses.every(s => active.has(s)) &&
          Array.from(active).every(s => statuses.includes(s));
        if (isolated) {
          chips.forEach(c => { active.add(c.dataset.status); c.classList.remove("off"); c.classList.add("on"); });
        } else {
          active.clear();
          statuses.forEach(s => active.add(s));
          chips.forEach(c => {
            if (statuses.includes(c.dataset.status)) { c.classList.add("on");  c.classList.remove("off"); }
            else                                      { c.classList.add("off"); c.classList.remove("on"); }
          });
        }
        applyFilter();
      });
    });

    search.addEventListener("input", applyFilter);
    reset.addEventListener("click", () => {
      search.value = "";
      chips.forEach(c => { active.add(c.dataset.status); c.classList.remove("off"); c.classList.add("on"); });
      applyFilter();
    });

    applyFilter();
  })();
</script>
</body>
</html>
`

// ===========================================================================
//  Terminal template — monospace monoculture, box-drawing frames, phosphor
//  status badges. Dark-committed; the page itself doesn't try to light-mode.
// ===========================================================================

const htmlTerminalTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>{{.Title}}</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;700&display=swap">
<style>
  :root {
    --bg:        #0d0f0c;
    --panel:     #10140f;
    --fg:        #d7d8c7;
    --muted:     #6a7465;
    --rule:      #2a2e24;
    --prompt:    #67e089;
    --ok:        #67e089;
    --warn:      #e3c674;
    --warn-hot:  #e59a4a;
    --crit:      #e05454;
    --err:       #868d79;
    --accent:    #67e089;
    color-scheme: dark;
  }
  * { box-sizing: border-box; min-width: 0; }
  body {
    margin: 0; background: var(--bg); color: var(--fg);
    font: 13px/1.55 "JetBrains Mono", ui-monospace, "SF Mono", Menlo, monospace;
    padding: 24px clamp(16px, 4vw, 32px);
    -webkit-font-smoothing: antialiased;
  }
  main {
    max-width: 1200px; margin: 0 auto;
    background: var(--bg); border: 1px solid var(--rule);
    border-radius: 10px; padding: 24px clamp(16px, 3vw, 28px);
    overflow-x: auto;
  }
  .t-prompt, .t-cursor { color: var(--muted); user-select: none; }
  .t-prompt .p-user { color: var(--prompt); font-weight: 700; }
  .t-prompt .p-caret { color: var(--prompt); }
  .t-prompt .p-arg { color: var(--fg); }

  .t-frame { margin: 16px 0 16px; }
  .t-frame .top, .t-frame .bot { color: var(--rule); white-space: pre; margin: 0; }

  .t-summary {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: 2px 20px;
    padding: 8px 16px 10px;
    border-left: 1px solid var(--rule);
    border-right: 1px solid var(--rule);
  }
  .t-summary .l {
    font-size: 11px; color: var(--muted);
    text-transform: lowercase; letter-spacing: 0.02em;
  }
  .t-summary .n {
    font-size: 16px; font-weight: 600; color: var(--fg);
    font-variant-numeric: tabular-nums;
  }
  .t-summary .n.ok   { color: var(--ok); }
  .t-summary .n.warn { color: var(--warn); }
  .t-summary .n.crit { color: var(--crit); }

  .t-list {
    border-left: 1px solid var(--rule);
    border-right: 1px solid var(--rule);
  }
  .t-toolbar {
    display: flex; flex-wrap: wrap; gap: 10px; align-items: center;
    padding: 6px 16px;
    border-bottom: 1px solid var(--rule);
    color: var(--muted);
  }
  .t-toolbar .p { color: var(--prompt); }
  .t-toolbar input {
    flex: 1 1 200px; min-width: 0;
    background: transparent; color: var(--fg); border: 0; outline: 0;
    font: inherit; padding: 0;
  }
  .t-toolbar input::placeholder { color: var(--muted); }
  .t-toolbar .chips { display: flex; gap: 8px; flex-wrap: wrap; }
  .t-toolbar .chip {
    appearance: none; background: transparent; color: var(--muted);
    border: 1px solid var(--rule); border-radius: 3px;
    padding: 1px 7px; font: inherit; font-size: 11px;
    text-transform: uppercase; letter-spacing: 0.04em; cursor: pointer;
  }
  .t-toolbar .chip.on   { color: var(--fg); border-color: var(--fg); }
  .t-toolbar .chip.off  { opacity: 0.4; }
  .t-toolbar .chip.ok.on   { color: var(--ok);   border-color: var(--ok); }
  .t-toolbar .chip.warn.on { color: var(--warn); border-color: var(--warn); }
  .t-toolbar .chip.crit.on { color: var(--crit); border-color: var(--crit); }
  .t-toolbar .chip.err.on  { color: var(--err);  border-color: var(--err); }

  .t-list-head, .t-list-row, .t-list-foot {
    display: grid;
    grid-template-columns: 24px minmax(0, 2.4fr){{if .ShowIP}} minmax(130px, 1fr){{end}} 72px 110px minmax(0, 1.6fr);
    gap: 12px; padding: 2px 16px; align-items: baseline;
  }
  .t-list-head {
    color: var(--muted); text-transform: lowercase;
    border-bottom: 1px solid var(--rule);
    padding-top: 6px; padding-bottom: 6px;
  }
  .t-list-row:hover { background: color-mix(in oklab, var(--panel), transparent 30%); }
  .t-list-row .marker { color: var(--muted); user-select: none; }
  .t-list-row[data-s="valid"]         .marker { color: var(--ok); }
  .t-list-row[data-s="expiring_soon"] .marker { color: var(--warn); }
  .t-list-row[data-s="hot"]           .marker { color: var(--warn-hot); }
  .t-list-row[data-s="expired"]       .marker,
  .t-list-row[data-s="invalid"]       .marker { color: var(--crit); }
  .t-list-row[data-s="error"]         .marker { color: var(--err); }

  .t-domain { color: var(--fg); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .t-ip, .t-exp {
    color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .t-days { font-variant-numeric: tabular-nums; text-align: right; }
  .t-list-row[data-s="valid"]         .t-days { color: var(--ok); }
  .t-list-row[data-s="expiring_soon"] .t-days { color: var(--warn); }
  .t-list-row[data-s="hot"]           .t-days { color: var(--warn-hot); }
  .t-list-row[data-s="expired"]       .t-days,
  .t-list-row[data-s="invalid"]       .t-days { color: var(--crit); }
  .t-list-row[data-s="error"]         .t-days { color: var(--err); }

  .t-status { font-weight: 700; letter-spacing: 0.04em; font-size: 11px; margin-right: 6px; }
  .t-status.ok   { color: var(--ok); }
  .t-status.warn { color: var(--warn); }
  .t-status.hot  { color: var(--warn-hot); }
  .t-status.crit { color: var(--crit); }
  .t-status.err  { color: var(--err); }
  .t-note {
    color: var(--muted); overflow: hidden; text-overflow: ellipsis;
    white-space: nowrap; font-size: 12px;
  }

  .t-list-foot {
    color: var(--muted); border-top: 1px solid var(--rule);
    padding-top: 6px; padding-bottom: 6px;
    grid-template-columns: 1fr auto;
  }
  .t-cursor { margin-top: 14px; }
  .t-cursor .blink {
    display: inline-block; width: 7px; height: 14px; background: var(--prompt);
    vertical-align: -2px; margin-left: 2px;
    animation: tblink 1s steps(1) infinite;
  }
  @keyframes tblink { 50% { opacity: 0; } }
  @media (prefers-reduced-motion: reduce) {
    .t-cursor .blink { animation: none; }
  }
  @media (max-width: 760px) {
    body { font-size: 12px; }
    main { padding: 20px 16px; }
    .t-summary { grid-template-columns: repeat(3, minmax(0, 1fr)); }
    .t-list-head, .t-list-row, .t-list-foot {
      grid-template-columns: 16px minmax(0, 1fr) 60px minmax(60px, auto);
    }
    .t-list-head .h-ip, .t-list-head .h-exp,
    .t-list-row .t-ip,  .t-list-row .t-exp { display: none; }
  }
</style>
</head>
<body>
<main>
  <div class="t-prompt">
    <span class="p-user">checkssl</span> <span>❯</span>
    <span class="p-arg">report --generated={{.GeneratedAt}}{{if .ShowIP}} --show-ip{{end}}</span>
  </div>

  <div class="t-frame" aria-hidden="true">
    <pre class="top">┌─ summary ──────────────────────────────────────────────────────────────────┐</pre>
    <div class="t-summary">
      <div><div class="l">total</div><div class="n">{{.Total}}</div></div>
      <div><div class="l">valid</div><div class="n ok">{{.Valid}}</div></div>
      <div><div class="l">warning</div><div class="n warn">{{.ExpiringSoon}}</div></div>
      <div><div class="l">critical</div><div class="n crit">{{add .Expired .Invalid}}</div></div>
      <div><div class="l">errors</div><div class="n{{if gt .Errored 0}} crit{{end}}">{{.Errored}}</div></div>
    </div>
    <pre class="bot">└────────────────────────────────────────────────────────────────────────────┘</pre>
  </div>

  <div class="t-frame">
    <pre class="top">┌─ hosts ────────────────────────────────────────────────────────────────────┐</pre>
    <div class="t-list" id="report">
      <div class="t-toolbar">
        <span class="p">/</span>
        <input type="search" id="filter" placeholder="filter domain, ip, issuer…" autocomplete="off">
        <div class="chips">
          <button type="button" class="chip on ok"   data-status="valid">valid</button>
          <button type="button" class="chip on warn" data-status="expiring_soon">warn</button>
          <button type="button" class="chip on crit" data-status="expired">expired</button>
          <button type="button" class="chip on crit" data-status="invalid">invalid</button>
          <button type="button" class="chip on err"  data-status="error">down</button>
        </div>
        <button type="button" class="chip" id="reset" title="Reset">reset</button>
      </div>

      <div class="t-list-head">
        <span>  </span>
        <span>domain</span>
        {{if .ShowIP}}<span class="h-ip">ip</span>{{end}}
        <span style="text-align:right">days</span>
        <span class="h-exp">expires</span>
        <span>status · note</span>
      </div>
      {{$showIP := .ShowIP}}
      {{range .Rows}}
      <div class="t-list-row" data-s="{{.VisualClass}}" data-status="{{.Status}}">
        <span class="marker">{{.Marker}}</span>
        <span class="t-domain">{{.Domain}}</span>
        {{if $showIP}}<span class="t-ip">{{if .IP}}{{.IP}}{{else}}—{{end}}</span>{{end}}
        <span class="t-days">{{if .HasDays}}{{if lt .DaysInt 0}}-{{neg .DaysInt}}d{{else}}{{.DaysInt}}d{{end}}{{else}}   —{{end}}</span>
        <span class="t-exp">{{.Expiration}}</span>
        <span class="t-note"><span class="t-status {{.StatusCls}}">{{.StatusLabel}}</span>{{if .Note}}{{.Note}}{{else if .Issuer}}{{.Issuer}}{{end}}</span>
      </div>
      {{end}}
      <div class="t-list-foot">
        <span id="count">{{.Total}} rows · sorted by days asc</span>
        <span>filter: everything shown</span>
      </div>
    </div>
    <pre class="bot">└────────────────────────────────────────────────────────────────────────────┘</pre>
  </div>

  <div class="t-cursor"><span class="p-user">checkssl</span> <span>❯</span><span class="blink"></span></div>
</main>
<script>
  (function () {
    const rows   = Array.from(document.querySelectorAll(".t-list-row"));
    const chips  = document.querySelectorAll(".chip[data-status]");
    const search = document.getElementById("filter");
    const reset  = document.getElementById("reset");
    const count  = document.getElementById("count");
    const total  = rows.length;

    const active = new Set();
    chips.forEach(c => active.add(c.dataset.status));

    function cellsText(row) {
      return Array.from(row.children).map(c => c.textContent).join(" ").toLowerCase();
    }

    function applyFilter() {
      const needle = search.value.trim().toLowerCase();
      let shown = 0;
      for (const row of rows) {
        const statusOK = active.has(row.dataset.status);
        const textOK = !needle || cellsText(row).includes(needle);
        const visible = statusOK && textOK;
        row.hidden = !visible;
        if (visible) shown++;
      }
      count.textContent = shown === total
        ? shown + " rows · sorted by days asc"
        : shown + " of " + total + " rows · filtered";
    }

    chips.forEach(chip => {
      chip.addEventListener("click", () => {
        const s = chip.dataset.status;
        if (active.has(s)) { active.delete(s); chip.classList.add("off"); chip.classList.remove("on"); }
        else               { active.add(s);    chip.classList.remove("off"); chip.classList.add("on"); }
        applyFilter();
      });
    });

    search.addEventListener("input", applyFilter);
    reset.addEventListener("click", () => {
      search.value = "";
      chips.forEach(c => { active.add(c.dataset.status); c.classList.remove("off"); c.classList.add("on"); });
      applyFilter();
    });

    // Press "/" anywhere to focus the filter input, vim-style.
    document.addEventListener("keydown", (e) => {
      if (e.key === "/" && e.target !== search) {
        e.preventDefault();
        search.focus();
      } else if (e.key === "Escape" && e.target === search) {
        search.value = "";
        applyFilter();
        search.blur();
      }
    });

    applyFilter();
  })();
</script>
</body>
</html>
`
