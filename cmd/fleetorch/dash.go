package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

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
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type dashModel struct {
	store *store.Store

	tasks    []*types.Task
	selected int

	logTail string

	width  int
	height int

	err          error
	confirmKill  bool   // K pressed once; second press confirms
	flashMessage string // transient footer note
}

func newDashModel(st *store.Store) dashModel {
	return dashModel{store: st}
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
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.selected < len(m.tasks)-1 {
				m.selected++
				m.logTail = ""
			}
			return m, nil
		case "k", "up":
			if m.selected > 0 {
				m.selected--
				m.logTail = ""
			}
			return m, nil
		case "g":
			m.selected = 0
			m.logTail = ""
			return m, nil
		case "G":
			if len(m.tasks) > 0 {
				m.selected = len(m.tasks) - 1
				m.logTail = ""
			}
			return m, nil
		case "r":
			return m, m.refresh
		case "K":
			if len(m.tasks) == 0 {
				return m, nil
			}
			id := m.tasks[m.selected].ID
			if !m.confirmKill {
				m.confirmKill = true
				m.flashMessage = "press K again to kill " + id + ", any other key cancels"
				return m, nil
			}
			m.confirmKill = false
			m.flashMessage = "killed " + id
			if err := doKill(id, false); err != nil {
				m.flashMessage = "kill failed: " + err.Error()
			}
			return m, m.refresh
		default:
			// Any other key cancels a pending kill confirmation.
			if m.confirmKill {
				m.confirmKill = false
				m.flashMessage = ""
			}
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
			m.logTail = readLogTail(m.tasks[m.selected].Log, logTailBytes)
		}
		m.err = nil
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil
	}
	return m, nil
}

// ---- styles --------------------------------------------------------------

var (
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")).Background(lipgloss.Color("236"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleActive   = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	styleIdle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleDone     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleFailed   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
)

func (m dashModel) View() string {
	if m.width == 0 {
		return "loading…"
	}

	leftW := m.width * 2 / 5
	rightW := m.width - leftW - 4
	innerH := m.height - 4 // borders + bottom bar

	left := m.renderTaskList(leftW, innerH)
	right := m.renderLog(rightW, innerH)

	header := styleTitle.Render("fleetorch") + styleDim.Render("  "+time.Now().Format("15:04:05"))
	bottomKeys := "j/k navigate  g/G top/bottom  K kill  r refresh  q quit"
	bottom := styleDim.Render(bottomKeys)
	if m.flashMessage != "" {
		bottom = styleIdle.Render(m.flashMessage) + "  " + styleDim.Render(bottomKeys)
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		styleBorder.Width(leftW).Height(innerH).Render(left),
		styleBorder.Width(rightW).Height(innerH).Render(right),
	)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, bottom)
}

func (m dashModel) renderTaskList(w, h int) string {
	if len(m.tasks) == 0 {
		return styleDim.Render("no tasks") + "\n\n" + styleDim.Render("spawn one: fleetorch spawn <agent> <id> \"<prompt>\"")
	}

	var b strings.Builder
	for i, t := range m.tasks {
		live := liveStatus(t)
		statusStr := styleForStatus(live).Render(string(live))
		line := fmt.Sprintf("%-12s  %-13s  %s  %s  $%.2f",
			truncate(t.ID, 12), truncate(t.Agent, 13), statusStr, age(t.StartedAt), t.BudgetUSD)
		if i == m.selected {
			line = styleSelected.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if m.err != nil {
		b.WriteString("\n" + styleFailed.Render("error: "+m.err.Error()))
	}
	_ = w
	_ = h
	return b.String()
}

func (m dashModel) renderLog(w, h int) string {
	if len(m.tasks) == 0 {
		return ""
	}
	t := m.tasks[m.selected]
	header := styleTitle.Render(t.ID) + styleDim.Render(" — "+t.Log)
	body := m.logTail
	if body == "" {
		body = styleDim.Render("(no log output yet)")
	} else {
		body = tailLinesString(body, h-2)
	}
	_ = w
	return header + "\n\n" + body
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
		return styleDim
	}
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

func tailLinesString(s string, n int) string {
	if n <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func doDashTUI() error {
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
