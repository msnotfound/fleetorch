package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newDashCmdReal() *cobra.Command {
	var plain bool
	cmd := &cobra.Command{
		Use:   "dash",
		Short: "Interactive TUI dashboard (use --plain for an auto-refresh table)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if plain {
				return doDashPlain()
			}
			return doDashTUI()
		},
	}
	cmd.Flags().BoolVar(&plain, "plain", false, "Simple auto-refreshing table (no TUI)")
	return cmd
}

// ---- constants & colours -------------------------------------------------

const (
	refreshInterval = 1 * time.Second
	logTailBytes    = 8 << 10

	paneList = 0
	paneLog  = 1
	paneDiff = 2
)

var (
	colorAccent  = lipgloss.Color("#7D53DE")
	colorSuccess = lipgloss.Color("#2AF598")
	colorWarning = lipgloss.Color("#FF9F43")
	colorDanger  = lipgloss.Color("#FF4757")
	colorMuted   = lipgloss.Color("#6272A4")
	colorFG      = lipgloss.Color("#F8F8F2")
	colorBG      = lipgloss.Color("#282A36")
	colorSel     = lipgloss.Color("#44475A")
)

var (
	styleAccent      = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleMuted       = lipgloss.NewStyle().Foreground(colorMuted)
	styleSuccess     = lipgloss.NewStyle().Foreground(colorSuccess)
	styleWarn        = lipgloss.NewStyle().Foreground(colorWarning)
	styleDanger      = lipgloss.NewStyle().Foreground(colorDanger)
	styleSel         = lipgloss.NewStyle().Background(colorSel).Foreground(colorFG).Bold(true)
	styleBorder      = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colorMuted)
	styleFocusBorder = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colorAccent)
)

// ---- messages ------------------------------------------------------------

type tickMsg  struct{}
type tasksMsg []*types.Task
type errMsg   struct{ err error }
type diffMsg  struct{ content string }

// ---- model ---------------------------------------------------------------

type dashModel struct {
	store      *store.Store
	cputimeDir string

	allTasks     []*types.Task
	visible      []*types.Task
	liveStatuses map[string]types.Status
	selectedIdx  int

	logLines     []string
	logScrollOff int

	diffVisible   bool
	diffContent   string
	diffLastFetch time.Time
	diffScrollOff int

	activePane int

	width  int
	height int

	err      error
	flashMsg string

	// inline footer kill confirm
	killConfirm bool

	// inline list filter
	filterActive bool
	filterInput  textinput.Model
	filterStr    string

	helpOpen     bool
	hideFinished bool
}

func newDashModel(st *store.Store, cputimeDir string) dashModel {
	ti := textinput.New()
	ti.Placeholder = "filter by task ID…"
	ti.CharLimit = 40
	return dashModel{
		store:        st,
		cputimeDir:   cputimeDir,
		logScrollOff: 99999,
		filterInput:  ti,
		liveStatuses: make(map[string]types.Status),
		hideFinished: true,
	}
}

// ---- tea lifecycle -------------------------------------------------------

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m dashModel) Init() tea.Cmd {
	return tea.Batch(m.doRefresh, tickCmd())
}

func (m dashModel) doRefresh() tea.Msg {
	tasks, err := m.store.ListTasks()
	if err != nil {
		return errMsg{err}
	}
	return tasksMsg(tasks)
}

func (m dashModel) fetchDiffCmd() tea.Cmd {
	if len(m.visible) == 0 || m.selectedIdx >= len(m.visible) {
		return func() tea.Msg { return diffMsg{"no task selected"} }
	}
	wt := m.visible[m.selectedIdx].Worktree
	return func() tea.Msg { return diffMsg{runGitDiff(wt)} }
}

// rebuildVisible recomputes the visible slice from allTasks applying hide and
// filter, preserving the currently-selected task ID when possible.
func (m *dashModel) rebuildVisible() {
	prevID := ""
	if m.selectedIdx >= 0 && m.selectedIdx < len(m.visible) {
		prevID = m.visible[m.selectedIdx].ID
	}

	m.visible = m.visible[:0]
	for _, t := range m.allTasks {
		live := m.liveStatuses[t.ID]
		if m.hideFinished && isFinished(live) {
			continue
		}
		if m.filterStr != "" && fuzzyScore(m.filterStr, t.ID) == 0 {
			continue
		}
		m.visible = append(m.visible, t)
	}

	m.selectedIdx = 0
	for i, t := range m.visible {
		if t.ID == prevID {
			m.selectedIdx = i
			break
		}
	}
	if m.selectedIdx >= len(m.visible) {
		m.selectedIdx = len(m.visible) - 1
	}
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
}

func (m dashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Kill confirm intercepts all keys.
		if m.killConfirm {
			switch msg.String() {
			case "y", "Y":
				m.killConfirm = false
				if len(m.visible) > 0 && m.selectedIdx < len(m.visible) {
					id := m.visible[m.selectedIdx].ID
					if err := doKill(id, false); err != nil {
						m.flashMsg = "kill failed: " + err.Error()
					} else {
						m.flashMsg = "killed " + id
						m.visible = append(m.visible[:m.selectedIdx], m.visible[m.selectedIdx+1:]...)
						if m.selectedIdx >= len(m.visible) && m.selectedIdx > 0 {
							m.selectedIdx--
						}
						m.logLines = nil
						m.logScrollOff = 99999
					}
				}
			default:
				m.killConfirm = false
			}
			return m, nil
		}

		// Filter mode intercepts typing.
		if m.filterActive {
			switch msg.String() {
			case "esc":
				m.filterActive = false
				m.filterInput.Blur()
				m.filterStr = ""
				m.filterInput.SetValue("")
				m.rebuildVisible()
				m.logLines = nil
				m.logScrollOff = 99999
			case "enter":
				m.filterActive = false
				m.filterInput.Blur()
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				m.filterStr = m.filterInput.Value()
				m.rebuildVisible()
				m.logLines = nil
				m.logScrollOff = 99999
				return m, cmd
			}
			return m, nil
		}

		// Help overlay — any key closes it.
		if m.helpOpen {
			m.helpOpen = false
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "r":
			return m, m.doRefresh

		case "?":
			m.helpOpen = true
			return m, nil

		case "esc":
			if m.filterStr != "" {
				m.filterStr = ""
				m.filterInput.SetValue("")
				m.rebuildVisible()
				m.logLines = nil
				m.logScrollOff = 99999
				m.flashMsg = ""
			}
			return m, nil

		case "/":
			m.filterActive = true
			m.filterInput.Focus()
			m.filterInput.SetValue(m.filterStr)
			return m, nil

		case "tab":
			if m.diffVisible {
				m.activePane = (m.activePane + 1) % 3
			} else {
				if m.activePane == paneList {
					m.activePane = paneLog
				} else {
					m.activePane = paneList
				}
			}
			return m, nil

		case "d":
			m.diffVisible = !m.diffVisible
			if !m.diffVisible && m.activePane == paneDiff {
				m.activePane = paneLog
			}
			if m.diffVisible {
				return m, m.fetchDiffCmd()
			}
			return m, nil

		case "a":
			m.hideFinished = !m.hideFinished
			m.rebuildVisible()
			m.logLines = nil
			m.logScrollOff = 99999
			m.flashMsg = ""
			return m, nil

		case "K":
			if len(m.visible) > 0 {
				m.killConfirm = true
				m.flashMsg = ""
			}
			return m, nil

		case "j", "down":
			switch m.activePane {
			case paneList:
				if m.selectedIdx < len(m.visible)-1 {
					m.selectedIdx++
					m.logLines = nil
					m.logScrollOff = 99999
					m.diffScrollOff = 0
					cmds := []tea.Cmd{m.doRefresh}
					if m.diffVisible {
						cmds = append(cmds, m.fetchDiffCmd())
					}
					return m, tea.Batch(cmds...)
				}
			case paneLog:
				m.logScrollOff++
			case paneDiff:
				m.diffScrollOff++
			}
			return m, nil

		case "k", "up":
			switch m.activePane {
			case paneList:
				if m.selectedIdx > 0 {
					m.selectedIdx--
					m.logLines = nil
					m.logScrollOff = 99999
					m.diffScrollOff = 0
					cmds := []tea.Cmd{m.doRefresh}
					if m.diffVisible {
						cmds = append(cmds, m.fetchDiffCmd())
					}
					return m, tea.Batch(cmds...)
				}
			case paneLog:
				if m.logScrollOff > 0 {
					m.logScrollOff--
				}
			case paneDiff:
				if m.diffScrollOff > 0 {
					m.diffScrollOff--
				}
			}
			return m, nil

		case "g":
			switch m.activePane {
			case paneList:
				if m.selectedIdx != 0 {
					m.selectedIdx = 0
					m.logLines = nil
					m.logScrollOff = 99999
					return m, m.doRefresh
				}
			case paneLog:
				m.logScrollOff = 0
			case paneDiff:
				m.diffScrollOff = 0
			}
			return m, nil

		case "G":
			switch m.activePane {
			case paneList:
				if last := len(m.visible) - 1; last >= 0 && m.selectedIdx != last {
					m.selectedIdx = last
					m.logLines = nil
					m.logScrollOff = 99999
					return m, m.doRefresh
				}
			case paneLog:
				m.logScrollOff = 99999
			case paneDiff:
				m.diffScrollOff = 99999
			}
			return m, nil
		}

	case tickMsg:
		cmds := []tea.Cmd{m.doRefresh, tickCmd()}
		if m.diffVisible && time.Since(m.diffLastFetch) >= 2*time.Second {
			cmds = append(cmds, m.fetchDiffCmd())
		}
		return m, tea.Batch(cmds...)

	case diffMsg:
		m.diffContent = msg.content
		m.diffLastFetch = time.Now()
		return m, nil

	case tasksMsg:
		m.allTasks = []*types.Task(msg)
		newStatuses := make(map[string]types.Status, len(m.allTasks))
		for _, t := range m.allTasks {
			newStatuses[t.ID] = liveStatus(t, m.cputimeDir)
		}
		m.liveStatuses = newStatuses
		m.rebuildVisible()
		m.reloadLog()
		m.err = nil
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil
	}
	return m, nil
}

// reloadLog reads the tail of the selected task's log file.
func (m *dashModel) reloadLog() {
	if m.selectedIdx < 0 || m.selectedIdx >= len(m.visible) {
		m.logLines = nil
		return
	}
	raw := readLogTail(m.visible[m.selectedIdx].Log, logTailBytes)
	trimmed := strings.TrimRight(raw, "\n")
	if trimmed == "" {
		m.logLines = nil
		return
	}
	rawLines := strings.Split(trimmed, "\n")
	sanitized := make([]string, len(rawLines))
	for i, l := range rawLines {
		sanitized[i] = sanitizeForViewport(l)
	}
	m.logLines = sanitized
}

// ---- view ----------------------------------------------------------------

func (m dashModel) View() string {
	if m.width == 0 {
		return "loading…"
	}

	if m.helpOpen {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			m.renderHelp(), lipgloss.WithWhitespaceBackground(colorBG))
	}

	// header(1) + border-overhead(2) + footer(2) = 5 fixed rows
	innerH := m.height - 5
	if innerH < 2 {
		innerH = 2
	}

	header := styleAccent.Render("fleetorch") + styleMuted.Render("  "+time.Now().Format("15:04:05"))

	var body string
	if m.diffVisible {
		avail := m.width - 6
		if avail < 3 {
			avail = 3
		}
		leftW := avail / 4
		centerW := avail / 2
		rightW := avail - leftW - centerW
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.paneStyle(paneList).Width(leftW).Height(innerH).Render(m.renderList(leftW, innerH)),
			m.paneStyle(paneLog).Width(centerW).Height(innerH).Render(m.renderLog(centerW, innerH)),
			m.paneStyle(paneDiff).Width(rightW).Height(innerH).Render(m.renderDiff(rightW, innerH)),
		)
	} else {
		avail := m.width - 4
		if avail < 2 {
			avail = 2
		}
		leftW := avail * 2 / 5
		rightW := avail - leftW
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.paneStyle(paneList).Width(leftW).Height(innerH).Render(m.renderList(leftW, innerH)),
			m.paneStyle(paneLog).Width(rightW).Height(innerH).Render(m.renderLog(rightW, innerH)),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, m.renderFooter())
}

func (m dashModel) paneStyle(pane int) lipgloss.Style {
	if m.activePane == pane {
		return styleFocusBorder
	}
	return styleBorder
}

// ---- pane renderers ------------------------------------------------------

func (m dashModel) renderList(w, h int) string {
	var lines []string

	// Filter bar at top.
	if m.filterActive {
		lines = append(lines, m.filterInput.View())
		h--
	} else if m.filterStr != "" {
		lines = append(lines, styleMuted.Render("/ ")+styleAccent.Render(m.filterStr)+styleMuted.Render("  [esc: clear]"))
		h--
	}

	if len(m.visible) == 0 {
		if m.filterStr != "" {
			lines = append(lines, "", styleMuted.Render("No matches."))
		} else {
			lines = append(lines, "", styleMuted.Render("No tasks. Try fleetorch spawn …"))
		}
		return strings.Join(lines, "\n")
	}

	// Scroll window keeping selected centered.
	start := m.selectedIdx - h/2
	if start < 0 {
		start = 0
	}
	if start+h > len(m.visible) {
		start = len(m.visible) - h
		if start < 0 {
			start = 0
		}
	}
	end := start + h
	if end > len(m.visible) {
		end = len(m.visible)
	}

	// Adaptive column widths.
	idW, agentW := 14, 10
	if w < 32 {
		idW, agentW = 10, 6
	} else if w > 52 {
		idW, agentW = 18, 12
	}

	for i := start; i < end; i++ {
		t := m.visible[i]
		live := m.liveStatuses[t.ID]

		glyph := statusGlyph(live)
		idPart := truncate(t.ID, idW)
		agPart := truncate(t.Agent, agentW)

		var idStyle, agStyle lipgloss.Style
		if isFinished(live) {
			idStyle = styleMuted
			agStyle = styleMuted
		} else if live == types.StatusActive || live == types.StatusRunning {
			idStyle = lipgloss.NewStyle().Foreground(colorFG)
			agStyle = styleMuted
		} else {
			idStyle = lipgloss.NewStyle().Foreground(colorFG)
			agStyle = styleMuted
		}

		rowContent := glyph + " " +
			idStyle.Render(fmt.Sprintf("%-*s", idW, idPart)) + "  " +
			agStyle.Render(fmt.Sprintf("%-*s", agentW, agPart)) + "  " +
			styleMuted.Render(fmt.Sprintf("%-7s", string(live))) + "  " +
			styleMuted.Render(fmt.Sprintf("%-4s", age(t.StartedAt))) + "  " +
			styleMuted.Render(fmt.Sprintf("$%.2f", t.BudgetUSD))

		if i == m.selectedIdx {
			plain := fmt.Sprintf("▸ %s %-*s  %-*s  %-7s  %-4s  $%.2f",
				statusGlyphChar(live),
				idW, truncate(t.ID, idW),
				agentW, truncate(t.Agent, agentW),
				string(live), age(t.StartedAt), t.BudgetUSD)
			lines = append(lines, styleSel.Width(w).Render(plain))
		} else {
			lines = append(lines, "  "+rowContent)
		}
	}

	// Scroll count indicator when list overflows.
	if len(m.visible) > h {
		lines = append(lines, styleMuted.Render(fmt.Sprintf(" %d/%d", m.selectedIdx+1, len(m.visible))))
	}

	return strings.Join(lines, "\n")
}

func (m dashModel) renderLog(w, h int) string {
	if len(m.visible) == 0 {
		return styleMuted.Render("(no task selected)")
	}
	t := m.visible[m.selectedIdx]
	header := styleAccent.Render(truncate(t.ID, w-10)) + styleMuted.Render(" — log")

	visibleH := h - 2
	if visibleH < 1 {
		visibleH = 1
	}

	if len(m.logLines) == 0 {
		return header + "\n\n" + styleMuted.Render("(no output yet)")
	}

	maxOff := len(m.logLines) - visibleH
	if maxOff < 0 {
		maxOff = 0
	}
	off := m.logScrollOff
	if off > maxOff {
		off = maxOff
	}

	end := off + visibleH
	if end > len(m.logLines) {
		end = len(m.logLines)
	}

	padded := make([]string, visibleH)
	copy(padded, m.logLines[off:end])
	body := strings.Join(padded, "\n")

	posText := scrollPos(off, maxOff, len(m.logLines), visibleH)
	scrollLine := lipgloss.NewStyle().Width(w).Align(lipgloss.Right).Foreground(colorMuted).Render(posText)

	return header + "\n" + body + "\n" + scrollLine
}

func (m dashModel) renderDiff(w, h int) string {
	if len(m.visible) == 0 {
		return styleMuted.Render("(no task selected)")
	}
	t := m.visible[m.selectedIdx]
	header := styleAccent.Render("diff") + styleMuted.Render(" — "+truncate(t.Worktree, w-8))

	visibleH := h - 2
	if visibleH < 1 {
		visibleH = 1
	}

	if m.diffContent == "" {
		return header + "\n\n" + styleMuted.Render("loading…")
	}

	rawLines := strings.Split(m.diffContent, "\n")
	colored := make([]string, len(rawLines))
	for i, l := range rawLines {
		colored[i] = colorDiffLine(l)
	}

	maxOff := len(colored) - visibleH
	if maxOff < 0 {
		maxOff = 0
	}
	off := m.diffScrollOff
	if off > maxOff {
		off = maxOff
	}

	end := off + visibleH
	if end > len(colored) {
		end = len(colored)
	}

	padded := make([]string, visibleH)
	copy(padded, colored[off:end])
	body := strings.Join(padded, "\n")

	posText := scrollPos(off, maxOff, len(colored), visibleH)
	scrollLine := lipgloss.NewStyle().Width(w).Align(lipgloss.Right).Foreground(colorMuted).Render(posText)

	return header + "\n" + body + "\n" + scrollLine
}

func (m dashModel) renderHelp() string {
	entries := [][2]string{
		{"j / k  ↓↑", "navigate tasks (or scroll log/diff)"},
		{"g / G", "jump to top / bottom"},
		{"K", "kill selected task (inline confirm)"},
		{"a", "toggle show finished tasks"},
		{"/", "filter task list by ID"},
		{"esc", "clear filter"},
		{"d", "toggle diff pane"},
		{"tab", "cycle pane focus"},
		{"r", "force refresh"},
		{"?", "toggle this help"},
		{"q / ctrl+c", "quit"},
	}
	var b strings.Builder
	b.WriteString(styleAccent.Render("  KEYBINDINGS") + "\n\n")
	for _, e := range entries {
		b.WriteString("  " + styleAccent.Render(fmt.Sprintf("%-14s", e[0])) +
			styleMuted.Render(e[1]) + "\n")
	}
	b.WriteString("\n  " + styleMuted.Render("press any key to close"))

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Render(b.String())
}

func (m dashModel) renderFooter() string {
	// Line 1: context / flash message.
	var infoStr string
	switch {
	case m.err != nil:
		infoStr = styleDanger.Render("error: " + m.err.Error())
	case m.flashMsg != "":
		infoStr = styleWarn.Render(m.flashMsg)
	default:
		pane := [3]string{"tasks", "log", "diff"}[m.activePane]
		n := len(m.visible)
		hidden := len(m.allTasks) - n
		suffix := ""
		if m.hideFinished && hidden > 0 {
			suffix = styleMuted.Render(fmt.Sprintf("  (%d finished hidden)", hidden))
		}
		infoStr = styleAccent.Render(pane) + styleMuted.Render(fmt.Sprintf("  %d task(s)", n)) + suffix
	}

	// Line 2: kill confirm or keymap.
	var actionStr string
	if m.killConfirm && len(m.visible) > 0 && m.selectedIdx < len(m.visible) {
		id := m.visible[m.selectedIdx].ID
		actionStr = styleDanger.Bold(true).Render("kill "+id+"?") + styleMuted.Render("  [y/N]")
	} else {
		actionStr = m.keymapStr()
	}

	line1 := lipgloss.NewStyle().Width(m.width).Render(infoStr)
	line2 := lipgloss.NewStyle().Width(m.width).Render(actionStr)
	return line1 + "\n" + line2
}

func (m dashModel) keymapStr() string {
	switch {
	case m.activePane != paneList:
		return styleMuted.Render("j/k: scroll  •  g/G: top/bot  •  tab: switch  •  q: quit")
	case m.width >= 100:
		show := "a: show all"
		if !m.hideFinished {
			show = "a: hide done"
		}
		return styleMuted.Render("j/k: select  •  K: kill  •  /: filter  •  d: diff  •  " + show + "  •  r: refresh  •  ?: help  •  q: quit")
	default:
		show := "a:show"
		if !m.hideFinished {
			show = "a:hide"
		}
		return styleMuted.Render("j/k:↑↓  K:kill  /:filter  d:diff  " + show + "  r:↻  ?:help  q:quit")
	}
}

// ---- helpers -------------------------------------------------------------

// statusGlyphChar returns the plain Unicode glyph for a status (no ANSI).
func statusGlyphChar(s types.Status) string {
	switch s {
	case types.StatusActive, types.StatusRunning:
		return "●"
	case types.StatusIdle:
		return "◐"
	case types.StatusDone:
		return "✓"
	default:
		return "✗"
	}
}

// statusGlyph returns the ANSI-coloured glyph for a status.
func statusGlyph(s types.Status) string {
	ch := statusGlyphChar(s)
	switch s {
	case types.StatusActive, types.StatusRunning:
		return styleSuccess.Render(ch)
	case types.StatusIdle:
		return styleWarn.Render(ch)
	case types.StatusDone:
		return styleMuted.Render(ch)
	default:
		return styleDanger.Render(ch)
	}
}

// isFinished returns true for terminal task states.
func isFinished(s types.Status) bool {
	return s == types.StatusDone || s == types.StatusFailed || s == types.StatusDead
}

// fuzzyScore returns a subsequence match score > 0 if pattern matches s.
func fuzzyScore(pattern, s string) int {
	pattern = strings.ToLower(pattern)
	s = strings.ToLower(s)
	if pattern == "" {
		return 1
	}
	pi, score, consec := 0, 0, 0
	for si := 0; si < len(s) && pi < len(pattern); si++ {
		if s[si] == pattern[pi] {
			pi++
			consec++
			score += consec * 2
			if si == 0 {
				score += 5
			}
		} else {
			consec = 0
		}
	}
	if pi < len(pattern) {
		return 0
	}
	return score
}

func scrollPos(off, maxOff, total, visH int) string {
	switch {
	case total <= visH:
		return "[ ALL ]"
	case off == 0:
		return "[ TOP ]"
	case off >= maxOff:
		return "[ BOT ]"
	default:
		return fmt.Sprintf("[ %d%% ]", 100*off/maxOff)
	}
}

func colorDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return styleSuccess.Render(line)
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return styleDanger.Render(line)
	default:
		return styleMuted.Render(line)
	}
}

// sanitizeForViewport strips ANSI cursor-positioning and control sequences that
// would corrupt the TUI layout, while preserving SGR color/style codes (ESC[…m).
func sanitizeForViewport(s string) string {
	if !strings.ContainsRune(s, '\x1b') {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		b := s[i]
		if b != '\x1b' {
			out.WriteByte(b)
			i++
			continue
		}
		if i+1 >= len(s) {
			i++
			continue
		}
		switch s[i+1] {
		case '[': // CSI sequence
			j := i + 2
			for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3F {
				j++
			}
			for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2F {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				out.WriteString(s[i : j+1]) // keep SGR
			}
			if j < len(s) {
				j++
			}
			i = j
		case ']': // OSC — consume until BEL or ST
			j := i + 2
			for j < len(s) {
				if s[j] == '\x07' {
					j++
					break
				}
				if s[j] == '\x1b' && j+1 < len(s) && s[j+1] == '\\' {
					j += 2
					break
				}
				j++
			}
			i = j
		case '(':
			i += 3
		case 'c':
			i += 2
		default:
			i++
		}
	}
	return out.String()
}

func readLogTail(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	size := info.Size()
	off := int64(0)
	if size > int64(n) {
		off = size - int64(n)
	}
	buf := make([]byte, size-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return ""
	}
	if off > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 && i < len(buf)-1 {
			buf = buf[i+1:]
		}
	}
	return string(buf)
}

func runGitDiff(worktree string) string {
	if worktree == "" {
		return "no diff available"
	}
	stat, err := runGitCmd(worktree, "diff", "--stat", "HEAD")
	if err != nil {
		stat, _ = runGitCmd(worktree, "diff")
		if stat == "" {
			return "no diff available"
		}
	}
	diff, _ := runGitCmd(worktree, "diff", "HEAD")
	if diff == "" {
		diff, _ = runGitCmd(worktree, "diff")
	}
	lines := strings.Split(diff, "\n")
	truncated := false
	if len(lines) > 200 {
		lines = lines[:200]
		truncated = true
	}
	result := strings.TrimRight(stat, "\n")
	if body := strings.Join(lines, "\n"); body != "" {
		result += "\n" + body
	}
	if truncated {
		result += "\n" + styleMuted.Render("… (truncated at 200 lines)")
	}
	return result
}

func runGitCmd(dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// ---- entry points --------------------------------------------------------

func doDashTUI() error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		msg := "dash requires a terminal. Use --plain for a non-interactive table."
		fmt.Fprintln(os.Stderr, msg)
		return fmt.Errorf("%s", msg)
	}
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	cputimeDir := filepath.Join(paths.DataDir, "cputime")
	_ = os.MkdirAll(cputimeDir, 0o755)
	st := store.New(paths.StateFile)
	p := tea.NewProgram(newDashModel(st, cputimeDir), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func doDashPlain() error {
	clear := func() { fmt.Print("\033[H\033[2J") }
	for {
		clear()
		fmt.Println("fleetorch dash --plain — Ctrl-C to exit")
		fmt.Println()
		if err := doList(); err != nil {
			fmt.Fprintln(os.Stderr, "list:", err)
		}
		time.Sleep(2 * time.Second)
	}
}
