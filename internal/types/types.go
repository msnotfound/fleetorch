// Package types holds the shared data structures used across fleetorch.
// These are the contract that supervisor, store, agents, and TUI all agree on.
package types

import "time"

// Status is the lifecycle state of a task.
type Status string

const (
	StatusRunning Status = "running" // spawned, process alive
	StatusActive  Status = "active"  // running + log activity in last 3 min
	StatusIdle    Status = "idle"    // running + no log activity 3+ min
	StatusDone    Status = "done"    // exited 0
	StatusFailed  Status = "failed"  // exited non-zero
	StatusDead    Status = "dead"    // process gone unexpectedly
)

// Task is a single spawned agent run.
type Task struct {
	ID        string    `json:"id"`
	Agent     string    `json:"agent"`         // agent-type name
	Worktree  string    `json:"worktree"`      // absolute path
	Log       string    `json:"log"`           // absolute path to log file
	Socket    string    `json:"socket,omitempty"` // absolute path to control socket
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Status    Status    `json:"status"`
	ExitCode  *int      `json:"exit_code,omitempty"` // nil if still running
	BudgetUSD float64   `json:"budget_usd"`
	Repo      string    `json:"repo,omitempty"`      // source repo, if any
	Branch    string    `json:"branch,omitempty"`    // agent/<id> branch
}

// AgentType is a plugin descriptor loaded from a TOML file.
type AgentType struct {
	Name             string   `toml:"name"`
	Command          string   `toml:"command"`            // binary name on $PATH
	Args             []string `toml:"args"`               // base args
	PromptArg        string   `toml:"prompt_arg"`         // template "{prompt}" — replaced at spawn
	DefaultBudgetUSD float64  `toml:"default_budget_usd"`
	DefaultTurns     int      `toml:"default_turns"`
	Sandbox          string   `toml:"sandbox"`            // "worktree" | "none"
	StreamsFreely    bool     `toml:"streams_freely"`     // false = stdout buffered (e.g. claude headless)
	Notes            string   `toml:"notes,omitempty"`
}

// SpawnSpec is the input to Supervisor.Spawn.
type SpawnSpec struct {
	ID        string
	Agent     AgentType
	Prompt    string
	Worktree  string
	Log       string
	Socket    string // optional Unix-domain control socket for live attach
	BudgetUSD float64
	Turns     int
	Model     string
}

// State is the on-disk task registry.
type State struct {
	Tasks  []*Task `json:"tasks"`
	Ledger Ledger  `json:"ledger"`
}

// Ledger tracks cumulative spawn counts per agent type.
type Ledger map[string]int

// Ledger keys — keep stable for jq/dashboard consumers.
const (
	LedgerCodex        = "codex"
	LedgerGemini       = "gemini"
	LedgerClaudeHaiku  = "claude_haiku"
	LedgerClaudeSonnet = "claude_sonnet"
	LedgerClaudeOpus   = "claude_opus"
)
