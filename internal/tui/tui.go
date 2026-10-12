// Package tui is checkssl's interactive terminal UI. It wraps the same
// cert.Check pipeline as the non-interactive modes but renders results as a
// live-updating table — rows appear as handshakes complete, keyboard
// shortcuts drive navigation, filter, sort, and reload.
package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/floriancrusius/checkssl/internal/cert"
	"github.com/floriancrusius/checkssl/internal/config"
)

// Options is what the CLI hands over to Run.
type Options struct {
	Entries     []config.Entry
	Timeout     time.Duration
	Concurrency int
	ShowIP      bool
}

// Run starts the TUI and blocks until the user quits.
func Run(opts Options) error {
	if len(opts.Entries) == 0 {
		return fmt.Errorf("tui: no domains to check")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 100
	}

	m := initialModel(opts)
	p := tea.NewProgram(m)
	progRef = p

	// Kick off the first scan in the background. All subsequent re-scans
	// are started from the Update loop via a dedicated command.
	go runScan(p, opts, 1)

	_, err := p.Run()
	return err
}

// -------- Messages ---------------------------------------------------------

type (
	scanStartedMsg struct {
		generation int
		total      int
		startedAt  time.Time
	}
	scanResultMsg struct {
		generation int
		result     cert.Result
	}
	scanDoneMsg struct {
		generation int
		elapsed    time.Duration
	}
	// editorDoneMsg lands after $EDITOR exits. On a clean exit we kick off
	// a reload so edits are reflected immediately.
	editorDoneMsg struct {
		err  error
		file string
	}
	tickMsg struct{}
)

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// runScan is the background goroutine that performs cert.Check on every
// domain and pushes incremental results back to the program.
func runScan(p *tea.Program, opts Options, generation int) {
	p.Send(scanStartedMsg{generation: generation, total: len(opts.Entries), startedAt: time.Now()})

	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup
	ctx := context.Background()
	start := time.Now()

	for _, e := range opts.Entries {
		wg.Add(1)
		sem <- struct{}{}
		go func(e config.Entry) {
			defer wg.Done()
			defer func() { <-sem }()
			r := cert.Check(ctx, e.Domain, cert.Options{Timeout: opts.Timeout})
			if !r.ExpiresAt.IsZero() {
				r.ExpiresAt = r.ExpiresAt.In(time.Local)
			}
			r.Source = e.File
			r.SourceLine = e.Line
			p.Send(scanResultMsg{generation: generation, result: r})
		}(e)
	}
	wg.Wait()
	p.Send(scanDoneMsg{generation: generation, elapsed: time.Since(start)})
}

// -------- Model ------------------------------------------------------------

type sortKey int

const (
	sortByDays sortKey = iota
	sortByDomain
	sortByIP
	sortByStatus
	sortByIssuer
)

func (k sortKey) String() string {
	switch k {
	case sortByDays:
		return "days"
	case sortByDomain:
		return "domain"
	case sortByIP:
		return "ip"
	case sortByStatus:
		return "status"
	case sortByIssuer:
		return "issuer"
	}
	return ""
}

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeHelp
	modeDetails
)

type model struct {
	opts Options

	// Scan state
	generation int
	total      int
	startedAt  time.Time
	elapsed    time.Duration
	scanning   bool
	results    []cert.Result

	// View state
	cursor    int
	rows      []cert.Result // filtered + sorted view
	filter    string
	filterBuf string
	chips     map[cert.Status]bool
	sort      sortKey
	sortDesc  bool
	mode      mode
	width     int
	height    int
	lastError string // transient status-line note (editor failures etc.)

	// Styles
	st styles
}

func initialModel(opts Options) model {
	return model{
		opts:  opts,
		chips: defaultChipState(),
		sort:  sortByDays,
		st:    newStyles(),
	}
}

func defaultChipState() map[cert.Status]bool {
	return map[cert.Status]bool{
		cert.StatusValid:        true,
		cert.StatusExpiringSoon: true,
		cert.StatusExpired:      true,
		cert.StatusInvalid:      true,
		cert.StatusError:        true,
	}
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

// -------- Styles -----------------------------------------------------------

type styles struct {
	header   lipgloss.Style
	meta     lipgloss.Style
	chip     lipgloss.Style
	chipOn   lipgloss.Style
	chipOff  lipgloss.Style
	colOK    lipgloss.Style
	colWarn  lipgloss.Style
	colHot   lipgloss.Style
	colCrit  lipgloss.Style
	colErr   lipgloss.Style
	colMuted lipgloss.Style

	tableHead    lipgloss.Style
	tableRow     lipgloss.Style
	tableSelMark lipgloss.Style
	tableSelPad  lipgloss.Style
	sortActive   lipgloss.Style

	hint      lipgloss.Style
	helpPanel lipgloss.Style
	helpKey   lipgloss.Style
	helpDesc  lipgloss.Style

	filterPrompt lipgloss.Style
	filterInput  lipgloss.Style
}

func newStyles() styles {
	ok := lipgloss.Color("#3fb950")
	warn := lipgloss.Color("#d2a85b")
	hot := lipgloss.Color("#e59a4a")
	crit := lipgloss.Color("#e05454")
	errc := lipgloss.Color("#868d79")
	muted := lipgloss.Color("240")
	fg := lipgloss.Color("252")
	accent := lipgloss.Color("#67e089")

	return styles{
		header:   lipgloss.NewStyle().Bold(true).Foreground(fg),
		meta:     lipgloss.NewStyle().Foreground(muted),
		chip:     lipgloss.NewStyle().Padding(0, 1),
		chipOn:   lipgloss.NewStyle().Padding(0, 1).Bold(true),
		chipOff:  lipgloss.NewStyle().Padding(0, 1).Foreground(muted).Faint(true),
		colOK:    lipgloss.NewStyle().Foreground(ok),
		colWarn:  lipgloss.NewStyle().Foreground(warn),
		colHot:   lipgloss.NewStyle().Foreground(hot),
		colCrit:  lipgloss.NewStyle().Foreground(crit),
		colErr:   lipgloss.NewStyle().Foreground(errc),
		colMuted: lipgloss.NewStyle().Foreground(muted),

		tableHead:    lipgloss.NewStyle().Foreground(muted).Underline(true),
		tableRow:     lipgloss.NewStyle(),
		tableSelMark: lipgloss.NewStyle().Foreground(accent).Bold(true),
		tableSelPad:  lipgloss.NewStyle().Foreground(muted),
		sortActive:   lipgloss.NewStyle().Foreground(accent).Bold(true),

		hint:         lipgloss.NewStyle().Foreground(muted),
		helpPanel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(1, 2),
		helpKey:      lipgloss.NewStyle().Foreground(accent).Bold(true),
		helpDesc:     lipgloss.NewStyle().Foreground(fg),
		filterPrompt: lipgloss.NewStyle().Foreground(accent).Bold(true),
		filterInput:  lipgloss.NewStyle().Foreground(fg),
	}
}

// -------- Update -----------------------------------------------------------

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		// Only used to keep the elapsed timer ticking while scanning.
		if m.scanning {
			return m, tickCmd()
		}
		return m, nil

	case scanStartedMsg:
		m.generation = msg.generation
		m.total = msg.total
		m.startedAt = msg.startedAt
		m.scanning = true
		m.results = make([]cert.Result, 0, msg.total)
		m.rebuildRows()
		return m, tickCmd()

	case scanResultMsg:
		if msg.generation != m.generation {
			return m, nil // ignore stragglers from an aborted scan
		}
		m.results = append(m.results, msg.result)
		m.rebuildRows()
		return m, nil

	case scanDoneMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.scanning = false
		m.elapsed = msg.elapsed
		return m, nil

	case editorDoneMsg:
		// Re-scan on a clean editor exit so edits appear immediately.
		// On error we just surface nothing and keep the current data.
		if msg.err != nil {
			m.lastError = fmt.Sprintf("editor: %v", msg.err)
			return m, nil
		}
		return m, m.reload()

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Filter mode: input editing + Esc/Enter to leave.
	if m.mode == modeFilter {
		switch key {
		case "esc":
			m.mode = modeNormal
			m.filter = ""
			m.filterBuf = ""
			m.rebuildRows()
		case "enter":
			m.filter = m.filterBuf
			m.mode = modeNormal
			m.rebuildRows()
		case "backspace":
			if len(m.filterBuf) > 0 {
				m.filterBuf = m.filterBuf[:len(m.filterBuf)-1]
				m.filter = m.filterBuf
				m.rebuildRows()
			}
		default:
			if len(key) == 1 { // ignore multi-char escapes
				m.filterBuf += key
				m.filter = m.filterBuf
				m.rebuildRows()
			}
		}
		return m, nil
	}

	// Help mode: Esc, ?, q leave; everything else is swallowed.
	if m.mode == modeHelp {
		switch key {
		case "esc", "?", "q":
			m.mode = modeNormal
		}
		return m, nil
	}

	// Details mode: Esc, l, enter, q close; j/k still move the cursor so the
	// panel updates as the user browses.
	if m.mode == modeDetails {
		switch key {
		case "esc", "l", "enter", "q":
			m.mode = modeNormal
		case "j", "down":
			m.moveCursor(+1)
		case "k", "up":
			m.moveCursor(-1)
		}
		return m, nil
	}

	// Normal mode
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.moveCursor(+1)
	case "k", "up":
		m.moveCursor(-1)
	case "g", "home":
		m.moveCursor(-len(m.rows))
	case "G", "end":
		m.moveCursor(+len(m.rows))
	case "ctrl+d", "pgdown":
		m.moveCursor(+10)
	case "ctrl+u", "pgup":
		m.moveCursor(-10)
	case "l", "enter":
		if len(m.rows) > 0 {
			m.mode = modeDetails
		}
	case "e":
		if cmd := m.editSelected(); cmd != nil {
			return m, cmd
		}
	case "/":
		m.mode = modeFilter
		m.filterBuf = m.filter
	case "r":
		return m, m.reload()
	case "s":
		m.sort = (m.sort + 1) % 5
		m.rebuildRows()
	case "S":
		m.sort = (m.sort + 4) % 5 // -1 mod 5
		m.rebuildRows()
	case "d":
		m.sortDesc = !m.sortDesc
		m.rebuildRows()
	case "?":
		m.mode = modeHelp
	case "1":
		m.toggleChip(cert.StatusValid)
	case "2":
		m.toggleChip(cert.StatusExpiringSoon)
	case "3":
		m.toggleChip(cert.StatusExpired)
	case "4":
		m.toggleChip(cert.StatusInvalid)
	case "5":
		m.toggleChip(cert.StatusError)
	}
	return m, nil
}

func (m *model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

func (m *model) toggleChip(s cert.Status) {
	m.chips[s] = !m.chips[s]
	m.rebuildRows()
}

// editSelected opens $EDITOR on the source file of the currently-selected
// row, jumping to its line. Returns nil (no command) when there is no
// row selected or the row has no file provenance (e.g. a -d command-line
// domain). Falls back to `vi` when $EDITOR is unset.
func (m *model) editSelected() tea.Cmd {
	if m.cursor >= len(m.rows) {
		return nil
	}
	r := m.rows[m.cursor]
	if r.Source == "" || r.SourceLine == 0 {
		m.lastError = "no source file for this row (added via -d or built-in default)"
		return nil
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	// `+<line>` is the near-universal convention understood by vi, vim,
	// nvim, nano, and most emacs frontends. VSCode's `code` wants
	// `--goto file:line` instead — users of code set $EDITOR to a
	// wrapper that translates, we don't try to detect it here.
	//nolint:gosec // editor+file paths come from user config; invoking them is the whole point.
	cmd := exec.Command(editor, fmt.Sprintf("+%d", r.SourceLine), r.Source)
	m.lastError = ""
	src := r.Source
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{err: err, file: src}
	})
}

func (m *model) reload() tea.Cmd {
	m.generation++
	m.results = nil
	m.rows = nil
	m.cursor = 0
	m.scanning = true
	gen := m.generation
	return func() tea.Msg {
		go func() {
			// NB: we cannot call p.Send from inside a Cmd easily, so start a
			// goroutine that triggers via a package-level program handle.
			// Use the stored reference set at Run().
			if progRef != nil {
				runScan(progRef, m.opts, gen)
			}
		}()
		return nil
	}
}

// -------- Rebuild rows (filter + sort) -------------------------------------

func visualClass(status cert.Status, days int, hasDays bool) string {
	if status == cert.StatusExpiringSoon && hasDays && days <= 14 {
		return "hot"
	}
	return string(status)
}

func (m *model) rebuildRows() {
	needle := strings.ToLower(strings.TrimSpace(m.filter))
	out := make([]cert.Result, 0, len(m.results))
	for _, r := range m.results {
		if !m.chips[r.Status] {
			continue
		}
		if needle != "" && !rowMatches(r, needle) {
			continue
		}
		out = append(out, r)
	}
	now := time.Now()
	sort.SliceStable(out, func(i, j int) bool {
		less := rowLess(out[i], out[j], m.sort, now)
		if m.sortDesc {
			return !less
		}
		return less
	})
	m.rows = out
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func rowMatches(r cert.Result, needle string) bool {
	hay := strings.ToLower(r.Domain + " " + r.ResolvedIP + " " + r.Issuer + " " + r.AuthError + " " + r.Err)
	return strings.Contains(hay, needle)
}

func rowLess(a, b cert.Result, by sortKey, now time.Time) bool {
	switch by {
	case sortByDomain:
		return strings.ToLower(a.Domain) < strings.ToLower(b.Domain)
	case sortByIP:
		return strings.ToLower(a.ResolvedIP) < strings.ToLower(b.ResolvedIP)
	case sortByIssuer:
		return strings.ToLower(a.Issuer) < strings.ToLower(b.Issuer)
	case sortByStatus:
		return statusOrdinal(a.Status) < statusOrdinal(b.Status)
	default: // sortByDays
		da := daysOrInf(a, now)
		db := daysOrInf(b, now)
		return da < db
	}
}

func daysOrInf(r cert.Result, now time.Time) int {
	if r.ExpiresAt.IsZero() {
		return 999999
	}
	return int(r.ExpiresAt.Sub(now).Hours() / 24)
}

func statusOrdinal(s cert.Status) int {
	switch s {
	case cert.StatusValid:
		return 0
	case cert.StatusExpiringSoon:
		return 1
	case cert.StatusExpired:
		return 2
	case cert.StatusInvalid:
		return 3
	case cert.StatusError:
		return 4
	}
	return 5
}

// progRef is a package-level reference to the running tea.Program, set by
// Run() so reload() can push messages back into the loop. Not ideal, but
// Bubble Tea's Cmd signature doesn't accept the program handle directly.
var progRef *tea.Program
