package main

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/msnotfound/fleetorch/internal/agents"
	"github.com/msnotfound/fleetorch/internal/config"
)

// ---- color palette (hex) ---------------------------------------------------

var (
	tourAccent  = lipgloss.Color("#7D53DE")
	tourSuccess = lipgloss.Color("#2AF598")
	tourWarning = lipgloss.Color("#FF9F43")
	tourMuted   = lipgloss.Color("#6272A4")
)

// ---- slide definitions -----------------------------------------------------

type slide struct {
	title string
	body  string
}

var tourSlides = []slide{
	{
		title: "Welcome to fleetorch",
		body: strings.Join([]string{
			"",
			"  Orchestrate parallel AI coding agents — spawn, supervise,",
			"  attach, and merge — from a single binary.",
			"",
			"  Each agent gets its own git worktree, a PTY session, and a",
			"  budget cap so nothing runs away with your API credits.",
			"",
			"  This tour takes about a minute. Press → or space to advance.",
		}, "\n"),
	},
	{
		title: "Architecture",
		body: strings.Join([]string{
			"",
			"  fleetorch spawn  ──►  supervisor process",
			"                           │",
			"             ┌─────────────┼─────────────┐",
			"             ▼             ▼             ▼",
			"          worker        worker        worker",
			"         (PTY)          (PTY)          (PTY)",
			"            │              │              │",
			"        git worktree   git worktree   git worktree",
			"        (isolated)     (isolated)     (isolated)",
			"",
			"  The supervisor writes state to ~/.fleetorch/state.json.",
			"  Workers are sandboxed: each gets a fresh branch + worktree,",
			"  so agents never clobber each other's files.",
		}, "\n"),
	},
	{
		title: "Core commands",
		body: strings.Join([]string{
			"",
			"  spawn   <agent> <id> \"<prompt>\"  — start a new agent task",
			"  list                              — show all tasks + status",
			"  attach  <id>                      — attach to a task's PTY",
			"  dash                              — interactive TUI dashboard",
			"  kill    <id>                      — terminate a task",
			"  prune                             — remove finished tasks",
			"  doctor                            — check system health",
			"  logs    <id>                      — stream a task's log",
			"  agent   <id>  \"<prompt>\"          — send a follow-up prompt",
		}, "\n"),
	},
	{
		title: "Pro tips",
		body: strings.Join([]string{
			"",
			"  Multi-attach broadcast",
			"    fleetorch attach --broadcast <id1> <id2>",
			"    Keystrokes are sent to all attached PTYs simultaneously.",
			"",
			"  Detach without killing",
			"    Press  Ctrl-] q  inside an attach session to detach.",
			"    The agent keeps running; reconnect any time.",
			"",
			"  Health check before spawning",
			"    fleetorch doctor   — verifies agent binaries, git, budget.",
			"",
			"  Debug mode",
			"    FLEETORCH_DEBUG=1 fleetorch <cmd>  — verbose internal logs.",
			"",
			"  One more step: try spawning an agent from the tour.",
		}, "\n"),
	},
	{
		title: "Try it now: spawn an agent",
		body: strings.Join([]string{
			"",
			"  The next screen opens a guided spawn form.",
			"",
			"  You'll choose an installed agent type, set a task ID,",
			"  and write the prompt the agent should run.",
			"",
			"  Submit the form to spawn the agent immediately.",
			"  Press Esc in the form to cancel without spawning.",
			"",
			"  Press →, space, or enter to open the form.",
		}, "\n"),
	},
}

// ---- model -----------------------------------------------------------------

type tourModel struct {
	current int
	width   int
	height  int
	spawn   bool
}

func (m tourModel) Init() tea.Cmd { return nil }

func (m tourModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "right", "l", " ", "enter":
			if m.current < len(tourSlides)-1 {
				m.current++
			} else {
				m.spawn = true
				return m, tea.Quit
			}
		case "left", "h":
			if m.current > 0 {
				m.current--
			}
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m tourModel) View() string {
	if m.width == 0 {
		return ""
	}

	s := tourSlides[m.current]

	boxW := m.width - 6
	if boxW < 20 {
		boxW = 20
	}
	boxH := m.height - 8
	if boxH < 6 {
		boxH = 6
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(tourAccent)

	bodyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E0E0E0"))

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tourMuted).
		Width(boxW).
		Height(boxH).
		PaddingLeft(2).
		PaddingRight(2)

	// Dots indicator: ● for current, ○ for others
	dots := make([]string, len(tourSlides))
	for i := range tourSlides {
		if i == m.current {
			dots[i] = lipgloss.NewStyle().Foreground(tourAccent).Render("●")
		} else {
			dots[i] = lipgloss.NewStyle().Foreground(tourMuted).Render("○")
		}
	}
	indicator := strings.Join(dots, " ")

	// Footer controls
	var footerParts []string
	if m.current > 0 {
		footerParts = append(footerParts, lipgloss.NewStyle().Foreground(tourSuccess).Render("← back"))
	}
	if m.current < len(tourSlides)-1 {
		footerParts = append(footerParts, lipgloss.NewStyle().Foreground(tourSuccess).Render("→/space next"))
	} else {
		footerParts = append(footerParts, lipgloss.NewStyle().Foreground(tourWarning).Render("→/space/enter spawn"))
	}
	footerParts = append(footerParts, lipgloss.NewStyle().Foreground(tourMuted).Render("q/esc quit"))
	footer := strings.Join(footerParts, lipgloss.NewStyle().Foreground(tourMuted).Render("  ·  "))

	slideContent := titleStyle.Render(s.title) + "\n" + bodyStyle.Render(s.body)
	box := borderStyle.Render(slideContent)

	return lipgloss.JoinVertical(lipgloss.Center,
		"",
		box,
		"",
		lipgloss.NewStyle().Foreground(tourMuted).Render(indicator),
		lipgloss.NewStyle().Foreground(tourMuted).Render(footer),
	)
}

// ---- launcher --------------------------------------------------------------

func launchTour() error {
	p := tea.NewProgram(tourModel{}, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if m, ok := finalModel.(tourModel); ok && m.spawn {
		return runTourSpawnForm()
	}
	return nil
}

func runTourSpawnForm() error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	if err := agents.SeedDefaults(paths.AgentsDir); err != nil {
		return err
	}
	reg, err := agents.Load(paths.AgentsDir)
	if err != nil {
		return err
	}

	agentList := reg.List()
	if len(agentList) == 0 {
		return fmt.Errorf("no agent types installed")
	}

	var (
		agentName string
		taskID    string
		prompt    string
	)

	options := make([]huh.Option[string], 0, len(agentList))
	for _, agent := range agentList {
		label := agent.Name
		if note := strings.TrimSpace(agent.Notes); note != "" {
			label = fmt.Sprintf("%s - %s", agent.Name, truncate(note, 72))
		}
		options = append(options, huh.NewOption(label, agent.Name))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Agent type").
				Options(options...).
				Value(&agentName),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Task ID").
				Description("Use a short identifier without spaces.").
				Validate(validateTourTaskID).
				Value(&taskID),
		),
		huh.NewGroup(
			huh.NewText().
				Title("Prompt").
				Description("Describe the work this agent should do.").
				Lines(6).
				Validate(validateTourPrompt).
				Value(&prompt),
		),
	).WithTheme(huh.ThemeDracula())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		return err
	}

	return doSpawn(agentName, strings.TrimSpace(taskID), prompt, "", 0, 0, "", false, false, "")
}

func validateTourTaskID(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("task ID cannot be empty")
	}
	if trimmed != value || strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("task ID cannot contain spaces")
	}
	return nil
}

func validateTourPrompt(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("prompt cannot be empty")
	}
	return nil
}
