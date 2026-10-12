package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/floriancrusius/checkssl/internal/cert"
)

// View renders the whole TUI. It composes:
//
//	header       title + scan progress + elapsed
//	summary      counts per status, highlighted when a chip is toggled off
//	columns      sortable header row (arrow on active column)
//	rows         scrollable list, selected row highlighted
//	footer       keyboard hint
//	overlay      help panel (when `?` is active) or filter prompt (`/`)
func (m model) View() tea.View {
	v := tea.NewView(m.renderBody())
	v.AltScreen = true
	return v
}

func (m model) renderBody() string {
	if m.width == 0 {
		return "loading…"
	}

	contentWidth := m.width - 2 // one char margin on each side

	// --- Header
	title := m.st.header.Render("checkssl")
	meta := m.renderMeta()
	headerLine := lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		strings.Repeat(" ", maxInt(0, contentWidth-lipgloss.Width(title)-lipgloss.Width(meta))),
		meta,
	)

	// --- Summary chips
	chips := m.renderChips()

	// --- Column header + rows
	colHeader, rowView, visibleRows := m.renderTable(contentWidth, m.tableHeight())

	// --- Status line (count + sort)
	status := m.renderStatusLine(visibleRows)

	// --- Footer hint
	hint := m.st.hint.Render(
		"j/k nav · l details · e edit · # comment · D del · N new · , conf · / filter · r reload · ? help · q quit",
	)

	main := strings.Join([]string{
		headerLine,
		"",
		chips,
		"",
		colHeader,
		rowView,
		status,
		"",
		hint,
	}, "\n")

	// Pad to full height so nothing jumps during scans.
	main = padToHeight(main, m.height-1)

	// --- Overlays
	switch m.mode {
	case modeFilter:
		return m.renderWithOverlay(main, m.renderFilterOverlay())
	case modeHelp:
		return m.renderWithOverlay(main, m.renderHelpOverlay())
	case modeDetails:
		return m.renderWithOverlay(main, m.renderDetailsOverlay())
	case modeConfirmDelete:
		return m.renderWithOverlay(main, m.renderConfirmDeleteOverlay())
	case modeNewFile:
		return m.renderWithOverlay(main, m.renderNewFileOverlay())
	}
	return main
}

func (m model) tableHeight() int {
	// Reserve rows for: header (1) + blank (1) + chips (1) + blank (1)
	//                 + col header (1) + status (1) + blank (1) + hint (1).
	// That's 8 lines of chrome plus two blank lines around the table.
	// Leave at least 5 rows for data.
	h := m.height - 10
	if h < 5 {
		return 5
	}
	return h
}

func (m model) renderMeta() string {
	if m.scanning {
		elapsed := time.Since(m.startedAt).Truncate(100 * time.Millisecond)
		return m.st.meta.Render(fmt.Sprintf("scanning %d of %d · %s", len(m.results), m.total, elapsed))
	}
	return m.st.meta.Render(fmt.Sprintf("%d hosts · scan %s", m.total, m.elapsed.Truncate(10*time.Millisecond)))
}

func (m model) renderChips() string {
	counts := map[cert.Status]int{}
	for _, r := range m.results {
		counts[r.Status]++
	}
	chip := func(label string, s cert.Status, tone lipgloss.Style) string {
		text := fmt.Sprintf("%d %s", counts[s], label)
		style := m.st.chipOff
		if m.chips[s] {
			style = m.st.chipOn.Foreground(tone.GetForeground())
		}
		return style.Render(text)
	}
	parts := []string{
		chip("valid", cert.StatusValid, m.st.colOK),
		chip("expiring", cert.StatusExpiringSoon, m.st.colWarn),
		chip("expired", cert.StatusExpired, m.st.colCrit),
		chip("invalid", cert.StatusInvalid, m.st.colCrit),
		chip("error", cert.StatusError, m.st.colErr),
	}
	return strings.Join(parts, " ")
}

// columnWidths picks column widths based on the available content width.
// Returned slice is in display order: marker, domain, (ip?), days, expires, status, note.
func (m model) columnWidths(contentWidth int) []int {
	// Fixed widths.
	marker := 2
	days := 7
	expires := 10
	status := 7

	ipWidth := 0
	if m.opts.ShowIP {
		ipWidth = 15
		for _, r := range m.rows {
			if l := lipgloss.Width(r.ResolvedIP); l > ipWidth {
				ipWidth = l
			}
		}
		if ipWidth > 39 {
			ipWidth = 39
		}
	}

	// Everything else goes to domain + note (3:2 split).
	gaps := 6 // separators (" " spacing between 7 columns, up to 6 gaps)
	if m.opts.ShowIP {
		gaps = 7
	}
	remain := contentWidth - marker - days - expires - status - ipWidth - gaps
	if remain < 20 {
		remain = 20
	}
	domainWidth := (remain * 3) / 5
	noteWidth := remain - domainWidth

	if m.opts.ShowIP {
		return []int{marker, domainWidth, ipWidth, days, expires, status, noteWidth}
	}
	return []int{marker, domainWidth, days, expires, status, noteWidth}
}

func (m model) renderTable(contentWidth, maxRows int) (header, body string, visible int) {
	widths := m.columnWidths(contentWidth)

	headerCells := []string{"", "domain"}
	if m.opts.ShowIP {
		headerCells = append(headerCells, "ip")
	}
	headerCells = append(headerCells, "days", "expires", "status", "note")
	// Mark the active sort column.
	activeIdx := map[sortKey]int{
		sortByDomain: 1,
		sortByIP:     2,
		sortByDays:   indexOf(m.opts.ShowIP, 2, 3),
		sortByStatus: indexOf(m.opts.ShowIP, 4, 5),
		sortByIssuer: -1, // not a direct column
	}[m.sort]
	for i, cell := range headerCells {
		trimmed := truncate(cell, widths[i])
		cellStyle := m.st.tableHead
		if i == activeIdx {
			cellStyle = m.st.sortActive
			arrow := " ▲"
			if m.sortDesc {
				arrow = " ▼"
			}
			if lipgloss.Width(trimmed)+2 <= widths[i] {
				trimmed += arrow
			}
		}
		headerCells[i] = cellStyle.Render(padCell(trimmed, widths[i], i == 3 || (!m.opts.ShowIP && i == 2)))
	}
	// Prefix with the same gutter width used by rows, so the columns line up.
	header = "  " + strings.Join(headerCells, " ")

	// Rows window
	start, end := scrollWindow(m.cursor, len(m.rows), maxRows)
	bodyLines := []string{}
	for i := start; i < end; i++ {
		r := m.rows[i]
		line := m.renderRow(r, widths)
		// Prepend a two-char gutter: a bright accent arrow on the selected
		// row, two spaces otherwise. Separate from the row's own ANSI so
		// per-cell colouring survives.
		if i == m.cursor {
			line = m.st.tableSelMark.Render("▸ ") + line
		} else {
			line = "  " + line
		}
		bodyLines = append(bodyLines, line)
	}
	// Pad body so a short list doesn't push the status line up.
	for len(bodyLines) < maxRows {
		bodyLines = append(bodyLines, "")
	}
	body = strings.Join(bodyLines, "\n")
	visible = end - start
	return
}

func (m model) renderRow(r cert.Result, widths []int) string {
	now := time.Now()
	days := daysOrInf(r, now)
	vc := visualClass(r.Status, days, !r.ExpiresAt.IsZero())
	marker, label, color := markerFor(vc, m.st)

	daysStr := "—"
	if !r.ExpiresAt.IsZero() {
		if days < 0 {
			daysStr = fmt.Sprintf("-%dd", -days)
		} else {
			daysStr = fmt.Sprintf("%dd", days)
		}
	}
	expires := "—"
	if !r.ExpiresAt.IsZero() {
		expires = r.ExpiresAt.Format("02.01.2006")
	}
	note := r.Issuer
	if r.AuthError != "" {
		note = r.AuthError
	} else if r.Err != "" {
		note = r.Err
	}

	cells := []string{color.Render(marker), padCell(truncate(r.Domain, widths[1]), widths[1], false)}
	if m.opts.ShowIP {
		cells = append(cells, m.st.colMuted.Render(padCell(truncate(r.ResolvedIP, widths[2]), widths[2], false)))
	}
	daysIdx := 2
	if m.opts.ShowIP {
		daysIdx = 3
	}
	cells = append(cells, color.Render(padCell(daysStr, widths[daysIdx], true)))
	cells = append(cells, m.st.colMuted.Render(padCell(expires, widths[daysIdx+1], false)))
	cells = append(cells, color.Render(padCell(label, widths[daysIdx+2], false)))
	cells = append(cells, m.st.colMuted.Render(truncate(note, widths[daysIdx+3])))

	return strings.Join(cells, " ")
}

func (m model) renderStatusLine(visible int) string {
	parts := []string{
		fmt.Sprintf("%d of %d shown", visible, len(m.results)),
	}
	if m.filter != "" {
		parts = append(parts, "filter: "+m.filter)
	}
	parts = append(parts, "sort: "+m.sort.String()+directionArrow(m.sortDesc))
	// Always show provenance of the cursor row so you can see at a
	// glance whether an edit key would have something to act on.
	if m.cursor < len(m.rows) {
		r := m.rows[m.cursor]
		if r.Source != "" {
			parts = append(parts, fmt.Sprintf("cursor: %s:%d", displayPath(r.Source), r.SourceLine))
		} else {
			parts = append(parts, "cursor: "+r.Domain+" [no source]")
		}
	}
	line := m.st.hint.Render(strings.Join(parts, "   "))
	if m.lastError != "" {
		line += "   " + m.st.colCrit.Render("⚠ "+m.lastError)
	}
	return line
}

func directionArrow(desc bool) string {
	if desc {
		return " ▼"
	}
	return " ▲"
}

func (m model) renderFilterOverlay() string {
	prompt := m.st.filterPrompt.Render("/")
	buf := m.st.filterInput.Render(m.filterBuf + "▊")
	hint := m.st.hint.Render("  Enter=apply  Esc=cancel")
	line := prompt + " " + buf + "   " + hint
	// Position bottom-left-ish; we just stick it at the end of the main view.
	return line
}

func (m model) renderHelpOverlay() string {
	rows := [][2]string{
		{"j / k", "next / previous host"},
		{"g / G", "first / last host"},
		{"ctrl+d / ctrl+u", "scroll ten rows"},
		{"l · enter", "details for the selected host (source file, issuer, error)"},
		{"e", "open the configured editor on the source file at the right line"},
		{"#", "comment out the selected line (writes to the source file)"},
		{"D", "delete the selected line from the source file (confirm with y)"},
		{"N", "create a new include-list file and open it in the editor"},
		{",", "open the checkssl config file in the editor"},
		{"s / S", "cycle sort column forward / backward"},
		{"d", "toggle sort direction (asc ↔ desc)"},
		{"1 … 5", "toggle valid · expiring · expired · invalid · error"},
		{"/", "filter (any cell, case-insensitive)"},
		{"r", "re-scan from scratch"},
		{"?", "this help"},
		{"q · ctrl+c", "quit"},
	}
	keyWidth := 0
	for _, r := range rows {
		if l := lipgloss.Width(r[0]); l > keyWidth {
			keyWidth = l
		}
	}
	lines := []string{m.st.header.Render("Keyboard shortcuts"), ""}
	for _, r := range rows {
		key := m.st.helpKey.Render(padCell(r[0], keyWidth, false))
		desc := m.st.helpDesc.Render(r[1])
		lines = append(lines, key+"  "+desc)
	}
	lines = append(lines, "", m.st.hint.Render("Esc / ? / q to close"))
	return m.st.helpPanel.Render(strings.Join(lines, "\n"))
}

// renderDetailsOverlay builds the panel shown when the user presses `l`
// or Enter on a row. It surfaces the per-domain information that the
// one-line table has to crop: full source file + line, issuer, errors,
// SANs. Rendered as a bottom-aligned panel over the main view.
// renderConfirmDeleteOverlay replaces the hint line with a yes/no
// prompt naming the host and source file. `y` confirms, anything else
// cancels. Kept on a single line so no modal panel is needed.
// renderNewFileOverlay replaces the hint line with a filename prompt
// for the modeNewFile input. Mirrors the filter prompt's styling.
func (m model) renderNewFileOverlay() string {
	prompt := m.st.filterPrompt.Render("new file:")
	buf := m.st.filterInput.Render(m.newBuf + "▊")
	suffix := ""
	if m.opts.NewFileSuffix != "" && !strings.HasSuffix(m.newBuf, m.opts.NewFileSuffix) {
		suffix = m.st.hint.Render(" (" + m.opts.NewFileSuffix + " will be appended)")
	}
	hint := m.st.hint.Render("  Enter=create  Esc=cancel")
	return prompt + " " + buf + suffix + "   " + hint
}

func (m model) renderConfirmDeleteOverlay() string {
	if m.cursor >= len(m.rows) {
		return m.st.hint.Render("")
	}
	r := m.rows[m.cursor]
	loc := fmt.Sprintf("%s:%d", displayPath(r.Source), r.SourceLine)
	warn := m.st.colCrit.Render("delete")
	host := m.st.helpKey.Render(r.Domain)
	return m.st.hint.Render(warn+" "+host+" from "+loc+"? ") +
		m.st.helpKey.Render("y") +
		m.st.hint.Render(" confirm · any other key cancels")
}

func (m model) renderDetailsOverlay() string {
	if m.cursor >= len(m.rows) {
		return ""
	}
	r := m.rows[m.cursor]
	label := func(k, v string) string {
		if v == "" {
			v = "—"
		}
		return m.st.helpKey.Render(padCell(k, 9, false)) + "  " + m.st.helpDesc.Render(v)
	}

	source := "—"
	if r.Source != "" {
		source = fmt.Sprintf("%s:%d", displayPath(r.Source), r.SourceLine)
	}

	expires := "—"
	if !r.ExpiresAt.IsZero() {
		expires = r.ExpiresAt.Format("02.01.2006 15:04 MST")
	}

	lines := []string{
		m.st.header.Render("Details"),
		"",
		label("domain", r.Domain),
		label("source", source),
		label("ip", r.ResolvedIP),
		label("status", string(r.Status)),
		label("expires", expires),
		label("issuer", r.Issuer),
	}
	if r.AuthError != "" {
		lines = append(lines, label("auth err", r.AuthError))
	}
	if r.Err != "" {
		lines = append(lines, label("error", r.Err))
	}
	if len(r.SubjectAlt) > 0 {
		sans := strings.Join(r.SubjectAlt, ", ")
		if len(sans) > 120 {
			sans = sans[:117] + "…"
		}
		lines = append(lines, label("SANs", sans))
	}
	lines = append(lines, "", m.st.hint.Render("Esc / l / enter to close · j/k still move the cursor"))
	return m.st.helpPanel.Render(strings.Join(lines, "\n"))
}

// displayPath shortens an absolute path by substituting $HOME with ~
// so the overlay stays readable even for deep home-directory paths.
func displayPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	if p == home {
		return "~"
	}
	return p
}

func (m model) renderWithOverlay(main, overlay string) string {
	if m.mode == modeFilter || m.mode == modeConfirmDelete || m.mode == modeNewFile {
		// Hint-line prompts: filename, delete-confirm, filter.
		lines := strings.Split(main, "\n")
		if len(lines) > 0 {
			lines[len(lines)-1] = overlay
		}
		return strings.Join(lines, "\n")
	}
	// Help & details: overlay a centred panel on top of the main view.
	return lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, overlay,
		lipgloss.WithWhitespaceChars(""))
}

// -------- helpers ----------------------------------------------------------

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, w int) string {
	if w <= 1 {
		return "…"
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	// Simple rune-based truncation; cert data is ASCII-ish in practice.
	rs := []rune(s)
	for len(rs) > 0 && lipgloss.Width(string(rs)+"…") > w {
		rs = rs[:len(rs)-1]
	}
	return string(rs) + "…"
}

func padCell(s string, w int, rightAlign bool) string {
	diff := w - lipgloss.Width(s)
	if diff <= 0 {
		return s
	}
	pad := strings.Repeat(" ", diff)
	if rightAlign {
		return pad + s
	}
	return s + pad
}

func padToHeight(s string, h int) string {
	lines := strings.Split(s, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func scrollWindow(cursor, total, maxRows int) (start, end int) {
	if total <= maxRows {
		return 0, total
	}
	start = cursor - maxRows/2
	if start < 0 {
		start = 0
	}
	end = start + maxRows
	if end > total {
		end = total
		start = end - maxRows
	}
	return
}

func indexOf(showIP bool, withoutIP, withIP int) int {
	if showIP {
		return withIP
	}
	return withoutIP
}

func markerFor(vc string, st styles) (marker, label string, color lipgloss.Style) {
	switch vc {
	case "valid":
		return "●", "VALID", st.colOK
	case "expiring_soon":
		return "●", "WARN", st.colWarn
	case "hot":
		return "●", "WARN", st.colHot
	case "expired":
		return "✖", "EXP", st.colCrit
	case "invalid":
		return "✖", "INV", st.colCrit
	case "error":
		return "○", "DOWN", st.colErr
	}
	return "·", vc, st.colMuted
}
