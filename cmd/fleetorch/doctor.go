package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/msnotfound/fleetorch/internal/agents"
	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newDoctorCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Print an environment + state report (paste into bug reports)",
		Long: `Collect everything a bug report should contain: fleetorch version,
OS/arch, resolved paths, dependency availability (git + agent CLIs),
installed agent types, current task/state stats, and obvious warnings.

Default output is human-readable. Use --json for a structured report
suitable for scripting or piping into ` + "`gh issue create`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := collectReport()
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(r)
			}
			return printReport(r)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of the human-readable report")
	return cmd
}

// Report is the structured form of a doctor run. Stable enough for scripts.
type Report struct {
	Version      VersionInfo      `json:"version"`
	Platform     PlatformInfo     `json:"platform"`
	Paths        PathsInfo        `json:"paths"`
	Dependencies []DependencyInfo `json:"dependencies"`
	Agents       AgentsInfo       `json:"agents"`
	State        StateInfo        `json:"state"`
	Warnings     []string         `json:"warnings"`
}

type VersionInfo struct {
	Fleetorch string `json:"fleetorch"`
	Go        string `json:"go"`
	BuiltFor  string `json:"built_for"`
}

type PlatformInfo struct {
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	AFUnixAvailable bool   `json:"af_unix_available"`
	StdoutIsTTY     bool   `json:"stdout_is_tty"`
	TermCols        int    `json:"term_cols,omitempty"`
	TermRows        int    `json:"term_rows,omitempty"`
}

type PathsInfo struct {
	FleetorchHome string `json:"fleetorch_home"` // env value, empty if unset
	ConfigDir     string `json:"config_dir"`
	DataDir       string `json:"data_dir"`
	AgentsDir     string `json:"agents_dir"`
	WorktreeDir   string `json:"worktree_dir"`
	LogDir        string `json:"log_dir"`
	SocketDir     string `json:"socket_dir"`
	StateFile     string `json:"state_file"`
	ConfigFile    string `json:"config_file"`
}

type DependencyInfo struct {
	Name    string `json:"name"`
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
}

type AgentsInfo struct {
	Installed []string `json:"installed"`
	Count     int      `json:"count"`
}

type StateInfo struct {
	TaskCount     int   `json:"task_count"`
	Active        int   `json:"active"`
	Done          int   `json:"done"`
	Failed        int   `json:"failed"`
	WorktreeBytes int64 `json:"worktree_bytes"`
	LogBytes      int64 `json:"log_bytes"`
}

func collectReport() (*Report, error) {
	r := &Report{
		Version: VersionInfo{
			Fleetorch: Version,
			Go:        runtime.Version(),
			BuiltFor:  runtime.GOOS + "/" + runtime.GOARCH,
		},
		Platform: PlatformInfo{
			OS:          runtime.GOOS,
			Arch:        runtime.GOARCH,
			StdoutIsTTY: term.IsTerminal(int(os.Stdout.Fd())),
		},
	}

	if r.Platform.StdoutIsTTY {
		if c, rw, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			r.Platform.TermCols, r.Platform.TermRows = c, rw
		}
	}

	r.Platform.AFUnixAvailable = checkAFUnix()

	paths, err := config.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolve paths: %w", err)
	}
	r.Paths = PathsInfo{
		FleetorchHome: os.Getenv("FLEETORCH_HOME"),
		ConfigDir:     paths.ConfigDir,
		DataDir:       paths.DataDir,
		AgentsDir:     paths.AgentsDir,
		WorktreeDir:   paths.WorktreeDir,
		LogDir:        paths.LogDir,
		SocketDir:     paths.SocketDir,
		StateFile:     paths.StateFile,
		ConfigFile:    paths.ConfigFile,
	}

	for _, name := range []string{"git", "agy", "codex", "gemini", "claude"} {
		r.Dependencies = append(r.Dependencies, probeDep(name))
	}

	if err := paths.EnsureDirs(); err == nil {
		if reg, err := agents.Load(paths.AgentsDir); err == nil {
			for _, a := range reg.List() {
				r.Agents.Installed = append(r.Agents.Installed, a.Name)
			}
			r.Agents.Count = len(r.Agents.Installed)
		}
	}

	st := store.New(paths.StateFile)
	if tasks, err := st.ListTasks(); err == nil {
		r.State.TaskCount = len(tasks)
		for _, t := range tasks {
			switch t.Status {
			case types.StatusActive, types.StatusRunning, types.StatusIdle:
				r.State.Active++
			case types.StatusDone:
				r.State.Done++
			case types.StatusFailed, types.StatusDead:
				r.State.Failed++
			}
		}
	}
	r.State.WorktreeBytes = dirSize(paths.WorktreeDir)
	r.State.LogBytes = dirSize(paths.LogDir)

	r.Warnings = collectWarnings(r)
	r.Warnings = append(r.Warnings, staleBuiltinAgentWarnings(paths.AgentsDir, r.Agents.Installed)...)
	return r, nil
}

func probeDep(name string) DependencyInfo {
	d := DependencyInfo{Name: name}
	p, err := exec.LookPath(name)
	if err != nil {
		return d
	}
	d.Found = true
	d.Path = p
	// Best-effort version probe — non-fatal, short timeout.
	for _, arg := range []string{"--version", "-v", "version"} {
		out, err := exec.Command(p, arg).CombinedOutput()
		if err == nil && len(out) > 0 {
			line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
			if len(line) > 80 {
				line = line[:80] + "..."
			}
			d.Version = line
			break
		}
	}
	return d
}

func checkAFUnix() bool {
	dir, err := os.MkdirTemp("", "fleetorch-afunix-probe-*")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	sockPath := filepath.Join(dir, "probe.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func collectWarnings(r *Report) []string {
	var warns []string

	for _, d := range r.Dependencies {
		if d.Name == "git" && !d.Found {
			warns = append(warns, "git is not on PATH — `spawn --repo .` will fail")
		}
	}

	agentNames := map[string]bool{}
	for _, a := range r.Agents.Installed {
		agentNames[a] = true
	}
	for _, d := range r.Dependencies {
		if d.Name == "git" {
			continue
		}
		// Map CLI presence to agent TOML availability. Mismatch = warning.
		switch d.Name {
		case "agy":
			if agentNames["agy"] && !d.Found {
				warns = append(warns, "agy CLI not on PATH; `agy` agent type will fail at spawn time")
			}
		case "codex":
			if agentNames["codex"] && !d.Found {
				warns = append(warns, "codex CLI not on PATH; `codex` agent type will fail at spawn time")
			}
		case "gemini":
			if agentNames["gemini"] && !d.Found {
				warns = append(warns, "gemini CLI not on PATH; `gemini` agent type will fail at spawn time")
			}
		case "claude":
			if !d.Found {
				if agentNames["claude-haiku"] || agentNames["claude-sonnet"] || agentNames["claude-opus"] {
					warns = append(warns, "claude CLI not on PATH; claude-* agent types will fail at spawn time")
				}
			}
		}
	}

	if !r.Platform.AFUnixAvailable {
		warns = append(warns, "AF_UNIX is not available; `attach` will fall back to --follow (read-only log tail)")
	}

	if r.Agents.Count == 0 {
		warns = append(warns, "no agent TOMLs installed; run `fleetorch agent list` to seed defaults")
	}

	return warns
}

func staleBuiltinAgentWarnings(agentsDir string, installed []string) []string {
	builtins, err := agents.BuiltinFiles()
	if err != nil {
		return nil
	}

	var warns []string
	for _, name := range installed {
		shipped, ok := builtins[name]
		if !ok {
			continue
		}
		onDisk, err := os.ReadFile(filepath.Join(agentsDir, name+".toml"))
		if err != nil || bytes.Equal(onDisk, shipped) {
			continue
		}
		warns = append(warns, fmt.Sprintf("warn: builtin agent %q on disk differs from shipped builtin. Run `fleetorch agent refresh-builtins` to update, or `agent edit %q` to inspect.", name, name))
	}
	return warns
}

func printReport(r *Report) error {
	w := os.Stdout

	fmt.Fprintln(w, "fleetorch doctor — environment report")
	fmt.Fprintln(w, strings.Repeat("=", 60))
	fmt.Fprintln(w)

	fmt.Fprintln(w, "VERSION")
	fmt.Fprintf(w, "  fleetorch  %s\n", r.Version.Fleetorch)
	fmt.Fprintf(w, "  Go         %s\n", r.Version.Go)
	fmt.Fprintf(w, "  built for  %s\n", r.Version.BuiltFor)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "PLATFORM")
	fmt.Fprintf(w, "  OS         %s\n", r.Platform.OS)
	fmt.Fprintf(w, "  arch       %s\n", r.Platform.Arch)
	fmt.Fprintf(w, "  AF_UNIX    %s\n", checkMark(r.Platform.AFUnixAvailable))
	fmt.Fprintf(w, "  stdout TTY %s", checkMark(r.Platform.StdoutIsTTY))
	if r.Platform.StdoutIsTTY && r.Platform.TermCols > 0 {
		fmt.Fprintf(w, " (%dx%d)", r.Platform.TermCols, r.Platform.TermRows)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "PATHS")
	if r.Paths.FleetorchHome != "" {
		fmt.Fprintf(w, "  FLEETORCH_HOME = %s\n", r.Paths.FleetorchHome)
	} else {
		fmt.Fprintln(w, "  FLEETORCH_HOME (not set — using OS defaults)")
	}
	fmt.Fprintf(w, "  config_dir   %s\n", r.Paths.ConfigDir)
	fmt.Fprintf(w, "  data_dir     %s\n", r.Paths.DataDir)
	fmt.Fprintf(w, "  agents_dir   %s\n", r.Paths.AgentsDir)
	fmt.Fprintf(w, "  worktree_dir %s\n", r.Paths.WorktreeDir)
	fmt.Fprintf(w, "  log_dir      %s\n", r.Paths.LogDir)
	fmt.Fprintf(w, "  socket_dir   %s\n", r.Paths.SocketDir)
	fmt.Fprintf(w, "  state_file   %s\n", r.Paths.StateFile)
	fmt.Fprintf(w, "  config_file  %s\n", r.Paths.ConfigFile)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "DEPENDENCIES")
	for _, d := range r.Dependencies {
		if d.Found {
			fmt.Fprintf(w, "  %-9s %s %s", d.Name, checkMark(true), d.Path)
			if d.Version != "" {
				fmt.Fprintf(w, "  (%s)", d.Version)
			}
			fmt.Fprintln(w)
		} else {
			fmt.Fprintf(w, "  %-9s %s not on PATH\n", d.Name, checkMark(false))
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "AGENTS")
	if r.Agents.Count == 0 {
		fmt.Fprintln(w, "  (none installed)")
	} else {
		fmt.Fprintf(w, "  %d installed: %s\n", r.Agents.Count, strings.Join(r.Agents.Installed, ", "))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "STATE")
	fmt.Fprintf(w, "  tasks      %d (active=%d, done=%d, failed=%d)\n",
		r.State.TaskCount, r.State.Active, r.State.Done, r.State.Failed)
	fmt.Fprintf(w, "  worktrees  %s\n", humanBytes(r.State.WorktreeBytes))
	fmt.Fprintf(w, "  logs       %s\n", humanBytes(r.State.LogBytes))
	fmt.Fprintln(w)

	if len(r.Warnings) > 0 {
		fmt.Fprintln(w, "WARNINGS")
		for _, msg := range r.Warnings {
			fmt.Fprintf(w, "  ! %s\n", msg)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintln(w, "no warnings — everything looks healthy.")
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "generated %s\n", time.Now().UTC().Format(time.RFC3339))
	return nil
}

func checkMark(ok bool) string {
	if ok {
		return "OK"
	}
	return "--"
}
