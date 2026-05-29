package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	pulseInterval   = 500 * time.Millisecond
	logTailBytes    = 8 << 10 // 8 KiB

	paneTaskList = 0
	paneLog      = 1
	paneDiff     = 2
)

// sparkRunes is the 8-level sparkline character set (low → high).
var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// pulseChars cycles for the live-indicator dot animation.
var pulseChars = []rune{'●', '◐', '○', '◑'}

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
				Width(56)
)

type tickMsg time.Time
type pulseTickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func pulseTickCmd() tea.Cmd {
	return tea.Tick(pulseInterval, func(t time.Time) tea.Msg { return pulseTickMsg(t) })
}

// diffMsg carries the result of a background git diff shell-out.
type diffMsg struct{ content string }

// paletteMatch holds a task index and its fuzzy score for the command palette.
type paletteMatch struct {
	taskIdx int
	score   int
}

type dashModel struct {
	store      *store.Store
	cputimeDir string

	tasks        []*types.Task
	selected     int
	liveStatuses map[string]types.Status // precomputed in Update, not View

	logLines     []string
	logScrollOff int // 99999 = stay at bottom (clamped on render)
	logFoldMode  bool

	activePane int // paneTaskList, paneLog, or paneDiff

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

	// live pulse animation (500ms tick)
	pulseFrame int

	// burn-rate sparkline history (keyed by task ID, up to 12 samples)
	burnHistory  map[string][]float64
	logSizeCache map[string]int64

	// split-pane diff viewer
	diffVisible   bool
	diffContent   string
	diffLastFetch time.Time
	diffScrollOff int
}

func newDashModel(st *store.Store, cputimeDir string) dashModel {
	ti := textinput.New()
	ti.Placeholder = "type task ID…"
	ti.CharLimit = 40
	return dashModel{
		store:        st,
		cputimeDir:   cputimeDir,
		logScrollOff: 99999,
		paletteInput: ti,
		liveStatuses: make(map[string]types.Status),
		burnHistory:  make(map[string][]float64),
		logSizeCache: make(map[string]int64),
	}
}

func (m dashModel) Init() tea.Cmd {
	return tea.Batch(m.refresh, tickCmd(), pulseTickCmd())
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

// fetchDiffCmd returns a Cmd that shells out git diff for the focused task's worktree.
func (m dashModel) fetchDiffCmd() tea.Cmd {
	if len(m.tasks) == 0 || m.selected >= len(m.tasks) {
		return func() tea.Msg { return diffMsg{"no task selected"} }
	}
	worktree := m.tasks[m.selected].Worktree
	return func() tea.Msg {
		return diffMsg{runGitDiff(worktree)}
	}
}

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
			if m.diffVisible {
				m.activePane = (m.activePane + 1) % 3
			} else {
				m.activePane = 1 - m.activePane
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

		case "j", "down":
			switch m.activePane {
			case paneTaskList:
				if m.selected < len(m.tasks)-1 {
					m.selected++
					m.logLines = nil
					m.logScrollOff = 99999
					m.diffScrollOff = 0
					cmds := []tea.Cmd{m.refresh}
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
			case paneTaskList:
				if m.selected > 0 {
					m.selected--
					m.logLines = nil
					m.logScrollOff = 99999
					m.diffScrollOff = 0
					cmds := []tea.Cmd{m.refresh}
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
			case paneTaskList:
				m.selected = 0
				m.logLines = nil
				m.logScrollOff = 99999
				m.diffScrollOff = 0
				return m, m.refresh
			case paneLog:
				m.logScrollOff = 0
			case paneDiff:
				m.diffScrollOff = 0
			}
			return m, nil

		case "G":
			switch m.activePane {
			case paneTaskList:
				if len(m.tasks) > 0 {
					m.selected = len(m.tasks) - 1
					m.logLines = nil
					m.logScrollOff = 99999
					m.diffScrollOff = 0
					return m, m.refresh
				}
			case paneLog:
				m.logScrollOff = 99999
			case paneDiff:
				m.diffScrollOff = 99999
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

	case pulseTickMsg:
		m.pulseFrame = (m.pulseFrame + 1) % 4
		return m, pulseTickCmd()

	case tickMsg:
		cmds := []tea.Cmd{m.refresh, tickCmd()}
		if m.diffVisible && time.Since(m.diffLastFetch) >= 2*time.Second {
			cmds = append(cmds, m.fetchDiffCmd())
		}
		return m, tea.Batch(cmds...)

	case diffMsg:
		m.diffContent = msg.content
		m.diffLastFetch = time.Now()
		return m, nil

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

		// Precompute live statuses (CPU probe writes disk; keep out of View).
		newStatuses := make(map[string]types.Status, len(m.tasks))
		for _, t := range m.tasks {
			newStatuses[t.ID] = liveStatus(t, m.cputimeDir)
		}
		m.liveStatuses = newStatuses

		// Update burn-rate sparkline history from log file size deltas.
		for _, t := range m.tasks {
			var sz int64
			if t.Log != "" {
				if info, err := os.Stat(t.Log); err == nil {
					sz = info.Size()
				}
			}
			if prev, ok := m.logSizeCache[t.ID]; ok {
				delta := float64(sz - prev)
				if delta < 0 {
					delta = 0
				}
				h := m.burnHistory[t.ID]
				h = append(h, delta)
				const maxHistory = 12
				if len(h) > maxHistory {
					h = h[len(h)-maxHistory:]
				}
				m.burnHistory[t.ID] = h
			}
			m.logSizeCache[t.ID] = sz
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
	innerH := m.height - 5
	if innerH < 3 {
		innerH = 3
	}

	var body string
	if m.diffVisible {
		// 3-column layout: each rounded border costs 2 cols → inner = width - 6
		total := m.width - 6
		leftW := total / 4
		rightW := total / 4
		centerW := total - leftW - rightW
		if centerW < 1 {
			centerW = 1
		}

		left := m.renderTaskList(leftW, innerH)
		center := m.renderLog(centerW, innerH)
		right := m.renderDiff(rightW, innerH)

		leftBorder := styleUnfocusBorder.Width(leftW).Height(innerH)
		centerBorder := styleUnfocusBorder.Width(centerW).Height(innerH)
		rightBorder := styleUnfocusBorder.Width(rightW).Height(innerH)
		switch m.activePane {
		case paneTaskList:
			leftBorder = styleFocusBorder.Width(leftW).Height(innerH)
		case paneLog:
			centerBorder = styleFocusBorder.Width(centerW).Height(innerH)
		case paneDiff:
			rightBorder = styleFocusBorder.Width(rightW).Height(innerH)
		}

		body = lipgloss.JoinHorizontal(lipgloss.Top,
			leftBorder.Render(left),
			centerBorder.Render(center),
			rightBorder.Render(right),
		)
	} else {
		// 2-column layout (existing behaviour)
		leftW := m.width * 2 / 5
		rightW := m.width - leftW - 4

		left := m.renderTaskList(leftW, innerH)
		right := m.renderLog(rightW, innerH)

		leftBorder := styleUnfocusBorder.Width(leftW).Height(innerH)
		rightBorder := styleUnfocusBorder.Width(rightW).Height(innerH)
		if m.activePane == paneTaskList {
			leftBorder = styleFocusBorder.Width(leftW).Height(innerH)
		} else {
			rightBorder = styleFocusBorder.Width(rightW).Height(innerH)
		}

		body = lipgloss.JoinHorizontal(lipgloss.Top,
			leftBorder.Render(left),
			rightBorder.Render(right),
		)
	}

	header := styleTitle.Render("fleetorch") + styleMuted.Render("  "+time.Now().Format("15:04:05"))

	// Two-line sticky footer
	paneName := "tasks"
	switch m.activePane {
	case paneLog:
		paneName = "log"
	case paneDiff:
		paneName = "diff"
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
	switch {
	case m.activePane == paneDiff:
		keymapStr = "tab: switch  •  j/k: scroll diff  •  g/G: top/bottom  •  d: hide diff  •  q: quit"
	case m.activePane == paneTaskList:
		keymapStr = "tab: switch  •  j/k: select  •  K: kill  •  d: toggle diff  •  ctrl+k: find  •  r: refresh  •  q: quit"
	default: // paneLog
		if m.diffVisible {
			keymapStr = "tab: switch  •  j/k: scroll  •  f: fold  •  d: toggle diff  •  g/G: top/bottom  •  q: quit"
		} else {
			keymapStr = "tab: switch  •  j/k: scroll  •  f: fold  •  g/G: top/bottom  •  q: quit"
		}
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
			live := m.liveStatuses[t.ID]
			dot := pulseDot(m.pulseFrame, live)
			statusStr := styleForStatus(live).Render(string(live))
			line := dot + " " + truncate(t.ID, 18) + "  " +
				styleMuted.Render(truncate(t.Agent, 12)) + "  " +
				statusStr
			if i == m.paletteSelIdx {
				b.WriteString(styleSel.Width(50).Render("▸ " + line))
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
		live := m.liveStatuses[t.ID]
		dot := pulseDot(m.pulseFrame, live)

		burningTag := ""
		if isBurning(t) && live == types.StatusActive {
			burningTag = " " + styleIdle.Render("burning")
		}

		statusStr := styleForStatus(live).Render(fmt.Sprintf("%-8s", string(live)))
		spark := sparklineStr(m.burnHistory[t.ID])
		bar := budgetBar(t.BudgetUSD)

		line := dot + " " + truncate(t.ID, 12) + "  " +
			truncate(t.Agent, 10) + "  " +
			statusStr + burningTag + "  " +
			fmt.Sprintf("%-6s", age(t.StartedAt)) + "  " +
			spark + " " + bar

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

// renderDiff renders the live git diff pane with scroll support.
func (m dashModel) renderDiff(w, h int) string {
	if len(m.tasks) == 0 {
		return styleMuted.Render("no diff available")
	}
	t := m.tasks[m.selected]
	wtDisplay := t.Worktree
	if len(wtDisplay) > w-8 && w > 8 {
		wtDisplay = "…" + wtDisplay[len(wtDisplay)-(w-9):]
	}
	header := styleTitle.Render("diff") + styleMuted.Render(" — "+wtDisplay)

	visibleH := h - 2
	if visibleH < 1 {
		visibleH = 1
	}

	if m.diffContent == "" {
		return header + "\n\n" + styleMuted.Render("loading…")
	}

	rawLines := strings.Split(m.diffContent, "\n")
	coloredLines := make([]string, len(rawLines))
	for i, l := range rawLines {
		coloredLines[i] = colorDiffLine(l)
	}

	maxOff := len(coloredLines) - visibleH
	if maxOff < 0 {
		maxOff = 0
	}
	off := m.diffScrollOff
	if off > maxOff {
		off = maxOff
	}

	end := off + visibleH
	if end > len(coloredLines) {
		end = len(coloredLines)
	}

	padded := make([]string, visibleH)
	copy(padded, coloredLines[off:end])
	body := strings.Join(padded, "\n")

	var posText string
	switch {
	case len(coloredLines) <= visibleH:
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

// pulseDot returns an animated dot character colored by task liveness.
func pulseDot(frame int, status types.Status) string {
	ch := string(pulseChars[frame%4])
	switch status {
	case types.StatusActive, types.StatusRunning:
		return lipgloss.NewStyle().Foreground(colorSuccess).Render(ch)
	case types.StatusIdle:
		return lipgloss.NewStyle().Foreground(colorWarning).Render(ch)
	default:
		return styleMuted.Render(ch)
	}
}

// sparklineStr renders a 12-cell burn-rate sparkline from log-size-delta samples.
func sparklineStr(samples []float64) string {
	const width = 12
	runes := make([]rune, width)

	if len(samples) == 0 {
		for i := range runes {
			runes[i] = sparkRunes[0]
		}
		return styleMuted.Render(string(runes))
	}

	maxVal := 0.0
	for _, v := range samples {
		if v > maxVal {
			maxVal = v
		}
	}

	// Right-align samples into the width window.
	src := samples
	if len(src) > width {
		src = src[len(src)-width:]
	}
	start := width - len(src)

	for i := range runes {
		runes[i] = sparkRunes[0] // pad left with low bar
	}
	for i, v := range src {
		idx := 0
		if maxVal > 0 {
			idx = int(v / maxVal * float64(len(sparkRunes)-1))
			if idx >= len(sparkRunes) {
				idx = len(sparkRunes) - 1
			}
		}
		runes[start+i] = sparkRunes[idx]
	}

	return styleMuted.Render(string(runes))
}

// isBurning returns true when a running task's log has been silent for >3 min,
// indicating stdout is buffered while the process may still be consuming CPU.
func isBurning(t *types.Task) bool {
	if t.PID <= 0 || t.Status == types.StatusDone || t.Status == types.StatusFailed {
		return false
	}
	if t.Log == "" {
		return false
	}
	info, err := os.Stat(t.Log)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) > 3*time.Minute
}

// colorDiffLine applies color to a unified diff line for the diff pane.
func colorDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return lipgloss.NewStyle().Foreground(colorSuccess).Render(line)
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return lipgloss.NewStyle().Foreground(colorDanger).Render(line)
	default:
		return styleMuted.Render(line)
	}
}

// runGitDiff produces a stat summary + first 200 lines of unified diff for worktree.
func runGitDiff(worktree string) string {
	if worktree == "" {
		return "no diff available"
	}

	stat, err := runGitCmd(worktree, "diff", "--stat", "HEAD")
	if err != nil {
		// Try unstaged diff if HEAD lookup fails (e.g. initial commit).
		stat, _ = runGitCmd(worktree, "diff")
		if stat == "" {
			return "no diff available"
		}
	}

	diff, _ := runGitCmd(worktree, "diff", "HEAD")
	if diff == "" {
		diff, _ = runGitCmd(worktree, "diff")
	}

	// Limit to 200 lines without shelling out to head(1) — cross-platform.
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

// runGitCmd runs git with the given args inside dir and returns stdout.
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
	cputimeDir := filepath.Join(paths.DataDir, "cputime")
	_ = os.MkdirAll(cputimeDir, 0o755)
	st := store.New(paths.StateFile)
	p := tea.NewProgram(newDashModel(st, cputimeDir), tea.WithAltScreen())
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
