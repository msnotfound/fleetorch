package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
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

// ---- bubbletea TUI -------------------------------------------------------

const (
	refreshInterval = 1 * time.Second
	logTailBytes    = 8 << 10 // 8 KiB

	paneTaskList = 0
	paneLog      = 1
)

// ---- hex color palette ---------------------------------------------------

var (
	colorAccent    = lipgloss.Color("#7D53DE")
	colorAccentDim = lipgloss.Color("#3C3C3C")
	colorSuccess   = lipgloss.Color("#2AF598")
	colorWarning   = lipgloss.Color("#FF9F43")
	colorDanger    = lipgloss.Color("#FF4757")
	colorMuted     = lipgloss.Color("#6272A4")
	colorFG        = lipgloss.Color("#F8F8F2")
	colorBG        = lipgloss.Color("#282A36")
)

// ---- styles --------------------------------------------------------------

var (
	styleTitle  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleSel    = lipgloss.NewStyle().Bold(true).Foreground(colorFG).Background(lipgloss.Color("#44475A"))
	styleMuted  = lipgloss.NewStyle().Foreground(colorMuted)
	styleActive = lipgloss.NewStyle().Foreground(colorSuccess)
	styleIdle   = lipgloss.NewStyle().Foreground(colorWarning)
	styleDone   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9FD"))
	styleFailed = lipgloss.NewStyle().Foreground(colorDanger)
	styleWarn   = lipgloss.NewStyle().Foreground(colorWarning)

	styleFocusBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent)
	styleUnfocusBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccentDim)

	styleFooter = lipgloss.NewStyle().Background(colorBG).Foreground(colorMuted)

	styleKillModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDanger).
			Padding(1, 3)
	styleKillTitle = lipgloss.NewStyle().Bold(true).Foreground(colorDanger)

	stylePaletteModal = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(0, 1).
				Width(52)
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// paletteMatch holds a task index and its fuzzy score for the command palette.
type paletteMatch struct {
	taskIdx int
	score   int
}

type dashModel struct {
	store *store.Store

	tasks    []*types.Task
	selected int

	logLines     []string
	logScrollOff int // 99999 = stay at bottom (clamped on render)
	logFoldMode  bool

	activePane int // paneTaskList or paneLog

	width  int
	height int

	err          error
	flashMessage string

	// kill confirmation modal
	killModalOpen bool

	// command palette
	paletteOpen    bool
	paletteInput   textinput.Model
	paletteMatches []paletteMatch
	paletteSelIdx  int
}

func newDashModel(st *store.Store) dashModel {
	ti := textinput.New()
	ti.Placeholder = "type task ID…"
	ti.CharLimit = 40
	return dashModel{store: st, logScrollOff: 99999, paletteInput: ti}
}

func (m dashModel) Init() tea.Cmd {
	return tea.Batch(m.refresh, tickCmd())
}

func (m dashModel) refresh() tea.Msg {
	tasks, err := m.store.ListTasks()
	if err != nil {
		return errMsg{err}
	}
	return tasksMsg(tasks)
}

type tasksMsg []*types.Task
type errMsg struct{ err error }

func (m dashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Kill modal has highest priority — intercepts all keys.
		if m.killModalOpen {
			switch msg.String() {
			case "y", "Y", "enter":
				m.killModalOpen = false
				if len(m.tasks) > 0 {
					id := m.tasks[m.selected].ID
					m.flashMessage = "killed " + id
					if err := doKill(id, false); err != nil {
						m.flashMessage = "kill failed: " + err.Error()
					}
					return m, m.refresh
				}
			default:
				m.killModalOpen = false
			}
			return m, nil
		}

		// Command palette — second priority.
		if m.paletteOpen {
			switch msg.String() {
			case "esc":
				m.paletteOpen = false
				m.paletteInput.SetValue("")
				m.paletteSelIdx = 0
			case "enter":
				if len(m.paletteMatches) > 0 {
					m.selected = m.paletteMatches[m.paletteSelIdx].taskIdx
					m.logLines = nil
					m.logScrollOff = 99999
				}
				m.paletteOpen = false
				m.paletteInput.SetValue("")
				m.paletteSelIdx = 0
				return m, m.refresh
			case "up":
				if m.paletteSelIdx > 0 {
					m.paletteSelIdx--
				}
			case "down":
				if m.paletteSelIdx < len(m.paletteMatches)-1 {
					m.paletteSelIdx++
				}
			default:
				var cmd tea.Cmd
				m.paletteInput, cmd = m.paletteInput.Update(msg)
				m.paletteSelIdx = 0
				m.updatePaletteMatches()
				return m, cmd
			}
			return m, nil
		}

		// Normal key handling.
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "ctrl+k", "/":
			m.paletteOpen = true
			m.paletteInput.Focus()
			m.paletteInput.SetValue("")
			m.paletteSelIdx = 0
			m.updatePaletteMatches()
			return m, nil

		case "tab":
			m.activePane = 1 - m.activePane
			return m, nil

		case "j", "down":
			if m.activePane == paneTaskList {
				if m.selected < len(m.tasks)-1 {
					m.selected++
					m.logLines = nil
					m.logScrollOff = 99999
					return m, m.refresh
				}
			} else {
				m.logScrollOff++
			}
			return m, nil

		case "k", "up":
			if m.activePane == paneTaskList {
				if m.selected > 0 {
					m.selected--
					m.logLines = nil
					m.logScrollOff = 99999
					return m, m.refresh
				}
			} else {
				if m.logScrollOff > 0 {
					m.logScrollOff--
				}
			}
			return m, nil

		case "g":
			if m.activePane == paneTaskList {
				m.selected = 0
				m.logLines = nil
				m.logScrollOff = 99999
				return m, m.refresh
			}
			m.logScrollOff = 0
			return m, nil

		case "G":
			if m.activePane == paneTaskList {
				if len(m.tasks) > 0 {
					m.selected = len(m.tasks) - 1
					m.logLines = nil
					m.logScrollOff = 99999
					return m, m.refresh
				}
			} else {
				m.logScrollOff = 99999
			}
			return m, nil

		case "r":
			return m, m.refresh

		case "K":
			if len(m.tasks) > 0 {
				m.killModalOpen = true
			}
			return m, nil

		case "f":
			if m.activePane == paneLog {
				m.logFoldMode = !m.logFoldMode
			}
			return m, nil
		}

	case tickMsg:
		return m, tea.Batch(m.refresh, tickCmd())

	case tasksMsg:
		m.tasks = []*types.Task(msg)
		if m.selected >= len(m.tasks) {
			m.selected = len(m.tasks) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
		if len(m.tasks) > 0 {
			raw := readLogTail(m.tasks[m.selected].Log, logTailBytes)
			trimmed := strings.TrimRight(raw, "\n")
			if trimmed != "" {
				m.logLines = strings.Split(trimmed, "\n")
			} else {
				m.logLines = nil
			}
		}
		// Keep palette matches in sync after task list refresh.
		if m.paletteOpen {
			m.updatePaletteMatches()
		}
		m.err = nil
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil
	}
	return m, nil
}

func (m dashModel) View() string {
	if m.width == 0 {
		return "loading…"
	}

	// Layout geometry: header(1) + border-top(1) + content + border-bot(1) + footer(2)
	leftW := m.width * 2 / 5
	rightW := m.width - leftW - 4 // 4 = two borders × 2 cols each
	innerH := m.height - 5
	if innerH < 3 {
		innerH = 3
	}

	left := m.renderTaskList(leftW, innerH)
	right := m.renderLog(rightW, innerH)

	// Focus-aware borders
	leftBorder := styleUnfocusBorder.Width(leftW).Height(innerH)
	rightBorder := styleUnfocusBorder.Width(rightW).Height(innerH)
	if m.activePane == paneTaskList {
		leftBorder = styleFocusBorder.Width(leftW).Height(innerH)
	} else {
		rightBorder = styleFocusBorder.Width(rightW).Height(innerH)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		leftBorder.Render(left),
		rightBorder.Render(right),
	)

	header := styleTitle.Render("fleetorch") + styleMuted.Render("  "+time.Now().Format("15:04:05"))

	// Two-line sticky footer
	paneName := "tasks"
	if m.activePane == paneLog {
		paneName = "log"
	}

	foldIndicator := ""
	if m.activePane == paneLog {
		if m.logFoldMode {
			foldIndicator = "  " + styleTitle.Render("fold: ON")
		} else {
			foldIndicator = "  " + styleMuted.Render("fold: OFF")
		}
	}
	infoLine := styleTitle.Render(paneName) +
		styleMuted.Render(fmt.Sprintf("  %d task(s)", len(m.tasks))) +
		foldIndicator

	var keymapStr string
	if m.activePane == paneTaskList {
		keymapStr = "tab: switch  •  j/k: select  •  K: kill  •  ctrl+k: find  •  r: refresh  •  q: quit"
	} else {
		keymapStr = "tab: switch  •  j/k: scroll  •  f: fold  •  g/G: top/bottom  •  q: quit"
	}
	keymapLine := styleMuted.Render(keymapStr)
	if m.flashMessage != "" {
		keymapLine = styleWarn.Render(m.flashMessage) + styleMuted.Render("  •  "+keymapStr)
	}

	footer := styleFooter.Width(m.width).Render(infoLine + "\n" + keymapLine)

	// Kill confirmation modal — full-screen overlay on dark background.
	if m.killModalOpen {
		modal := m.renderKillModal()
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			modal,
			lipgloss.WithWhitespaceBackground(colorBG),
		)
	}

	// Command palette — full-screen overlay anchored to top-center.
	if m.paletteOpen {
		modal := m.renderPalette()
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Top,
			modal,
			lipgloss.WithWhitespaceBackground(colorBG),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// renderKillModal renders the floating kill-confirmation modal box.
func (m dashModel) renderKillModal() string {
	if len(m.tasks) == 0 {
		return styleKillModal.Render(styleKillTitle.Render("no tasks selected"))
	}
	t := m.tasks[m.selected]
	title := styleKillTitle.Render("Kill task " + t.ID + "?")
	info := styleMuted.Render(fmt.Sprintf(
		"agent: %-12s  age: %-8s  burned: $%.2f",
		t.Agent, age(t.StartedAt), t.BudgetUSD,
	))
	prompt := lipgloss.NewStyle().Bold(true).Foreground(colorFG).Render("[y/N]")
	content := lipgloss.JoinVertical(lipgloss.Center, title, info, "", prompt)
	return styleKillModal.Render(content)
}

// renderPalette renders the command palette modal box.
func (m dashModel) renderPalette() string {
	var b strings.Builder
	b.WriteString(m.paletteInput.View())
	b.WriteString("\n")
	if len(m.paletteMatches) == 0 {
		b.WriteString(styleMuted.Render("  no matches"))
	} else {
		for i, pm := range m.paletteMatches {
			t := m.tasks[pm.taskIdx]
			live := liveStatus(t)
			statusStr := styleForStatus(live).Render(string(live))
			line := truncate(t.ID, 18) + "  " +
				styleMuted.Render(truncate(t.Agent, 12)) + "  " +
				statusStr
			if i == m.paletteSelIdx {
				b.WriteString(styleSel.Width(46).Render("▸ " + line))
			} else {
				b.WriteString("  " + line)
			}
			if i < len(m.paletteMatches)-1 {
				b.WriteString("\n")
			}
		}
	}
	return stylePaletteModal.Render(b.String())
}

// fuzzyScore returns a subsequence-match score > 0 if pattern matches s.
// Higher scores indicate better matches (consecutive runs, prefix matches).
func fuzzyScore(pattern, s string) int {
	pattern = strings.ToLower(pattern)
	s = strings.ToLower(s)
	if pattern == "" {
		return 1
	}
	pi := 0
	score := 0
	consecutive := 0
	for si := 0; si < len(s) && pi < len(pattern); si++ {
		if s[si] == pattern[pi] {
			pi++
			consecutive++
			score += consecutive * 2
			if si == 0 {
				score += 5 // bonus for matching at start
			}
		} else {
			consecutive = 0
		}
	}
	if pi < len(pattern) {
		return 0 // pattern was not fully matched
	}
	return score
}

// updatePaletteMatches refreshes the top-5 fuzzy matches for the current input.
func (m *dashModel) updatePaletteMatches() {
	query := m.paletteInput.Value()
	type scored struct {
		idx   int
		score int
	}
	var matches []scored
	for i, t := range m.tasks {
		var s int
		if query == "" {
			s = 1 // show all tasks when query is empty
		} else {
			s = fuzzyScore(query, t.ID)
		}
		if s > 0 {
			matches = append(matches, scored{i, s})
		}
	}
	sort.Slice(matches, func(a, b int) bool {
		return matches[a].score > matches[b].score
	})
	const maxMatches = 5
	if len(matches) > maxMatches {
		matches = matches[:maxMatches]
	}
	m.paletteMatches = make([]paletteMatch, len(matches))
	for i, sm := range matches {
		m.paletteMatches[i] = paletteMatch{taskIdx: sm.idx, score: sm.score}
	}
}

// foldLogLines replaces reasoning/tool-call noise with compact markers.
func foldLogLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	inThinking := false
	hiddenCount := 0

	flush := func() {
		if hiddenCount > 0 {
			marker := lipgloss.NewStyle().Foreground(colorMuted).
				Render(fmt.Sprintf("[+ %d hidden reasoning lines]", hiddenCount))
			out = append(out, marker)
			hiddenCount = 0
		}
	}

	for _, line := range lines {
		if inThinking {
			hiddenCount++
			if strings.Contains(line, "</thinking>") {
				inThinking = false
				flush()
			}
			continue
		}

		if strings.Contains(line, "<thinking>") {
			hiddenCount++
			if strings.Contains(line, "</thinking>") {
				// single-line block
				flush()
			} else {
				inThinking = true
			}
			continue
		}

		if strings.HasPrefix(line, "tool_use:") || strings.HasPrefix(line, "mcp_tool:") {
			hiddenCount++
			flush()
			continue
		}

		if strings.HasPrefix(line, "{") &&
			(strings.Contains(line, `"tool_use_id"`) || strings.Contains(line, `"input"`)) {
			hiddenCount++
			flush()
			continue
		}

		out = append(out, line)
	}

	// Flush any unclosed thinking block.
	if hiddenCount > 0 {
		flush()
	}

	return out
}

func (m dashModel) renderTaskList(w, h int) string {
	if len(m.tasks) == 0 {
		return styleMuted.Render("no tasks") + "\n\n" +
			styleMuted.Render("spawn one: fleetorch spawn <agent> <id> \"<prompt>\"")
	}

	var b strings.Builder
	for i, t := range m.tasks {
		live := liveStatus(t)
		statusStr := styleForStatus(live).Render(fmt.Sprintf("%-8s", string(live)))
		bar := budgetBar(t.BudgetUSD)
		line := truncate(t.ID, 12) + "  " +
			truncate(t.Agent, 10) + "  " +
			statusStr + "  " +
			fmt.Sprintf("%-6s", age(t.StartedAt)) + "  " +
			bar
		if i == m.selected {
			b.WriteString(styleSel.Width(w).Render("▸ " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	if m.err != nil {
		b.WriteString("\n" + styleFailed.Render("error: "+m.err.Error()))
	}
	_ = h
	return b.String()
}

func (m dashModel) renderLog(w, h int) string {
	if len(m.tasks) == 0 {
		return ""
	}
	t := m.tasks[m.selected]
	header := styleTitle.Render(t.ID) + styleMuted.Render(" — "+t.Log)

	// h lines available: 1 header + (h-2) content + 1 scroll indicator
	visibleH := h - 2
	if visibleH < 1 {
		visibleH = 1
	}

	lines := m.logLines
	if m.logFoldMode {
		lines = foldLogLines(lines)
	}

	if len(lines) == 0 {
		return header + "\n\n" + styleMuted.Render("(no log output yet)")
	}

	// Clamp scroll offset
	maxOff := len(lines) - visibleH
	if maxOff < 0 {
		maxOff = 0
	}
	off := m.logScrollOff
	if off > maxOff {
		off = maxOff
	}

	// Slice visible window
	end := off + visibleH
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[off:end]

	// Pad to exactly visibleH lines so scroll indicator stays at bottom
	padded := make([]string, visibleH)
	copy(padded, visible)
	body := strings.Join(padded, "\n")

	// Scroll position indicator
	var posText string
	switch {
	case len(lines) <= visibleH:
		posText = "[ ALL ]"
	case off == 0:
		posText = "[ TOP ]"
	case off >= maxOff:
		posText = "[ BOT ]"
	default:
		pct := 100 * off / maxOff
		posText = fmt.Sprintf("[ %d%% ]", pct)
	}

	scrollLine := lipgloss.NewStyle().Width(w).Align(lipgloss.Right).Foreground(colorMuted).Render(posText)

	return header + "\n" + body + "\n" + scrollLine
}

func styleForStatus(s types.Status) lipgloss.Style {
	switch s {
	case types.StatusActive, types.StatusRunning:
		return styleActive
	case types.StatusIdle:
		return styleIdle
	case types.StatusDone:
		return styleDone
	case types.StatusFailed, types.StatusDead:
		return styleFailed
	default:
		return styleMuted
	}
}

// budgetBar renders a coloured unicode block bar + dollar amount.
// Bar width is 8 chars; reference max is $20 for colour thresholds.
func budgetBar(budgetUSD float64) string {
	const (
		barW   = 8
		maxRef = 20.0
	)
	frac := budgetUSD / maxRef
	if frac > 1 {
		frac = 1
	}
	if frac < 0 {
		frac = 0
	}
	filled := int(frac * barW)

	var sb strings.Builder
	for i := 0; i < barW; i++ {
		if i < filled {
			sb.WriteRune('█')
		} else {
			sb.WriteRune('░')
		}
	}
	fmt.Fprintf(&sb, " $%.2f", budgetUSD)

	var color lipgloss.Color
	switch {
	case frac < 0.5:
		color = colorSuccess
	case frac < 0.75:
		color = colorWarning
	default:
		color = colorDanger
	}
	return lipgloss.NewStyle().Foreground(color).Render(sb.String())
}

// readLogTail returns the last n bytes of path (or less if smaller).
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
	// Drop the first (possibly partial) line if we seeked into the middle.
	if off > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 && i < len(buf)-1 {
			buf = buf[i+1:]
		}
	}
	return string(buf)
}

func doDashTUI() error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		msg := "dash requires a terminal. Use --plain for a non-interactive table."
		fmt.Fprintln(os.Stderr, msg)
		return errors.New(msg)
	}
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	st := store.New(paths.StateFile)
	p := tea.NewProgram(newDashModel(st), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// ---- plain fallback (kept for ssh / dumb terminals) ----------------------

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
