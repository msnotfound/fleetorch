// Package supervisor owns the lifecycle of every spawned task.
// It opens a cross-platform PTY for each process (via aymanbagabas/go-pty),
// tees the output to a log file and an in-memory ring buffer for attach replay,
// and exposes Spawn/Kill/Attach/Wait/IsAlive.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aymanbagabas/go-pty"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/types"
)

const (
	ringCapacity     = 4 << 10 // 4 KiB recent output for attach replay
	gracefulShutdown = 5 * time.Second
)

var ErrNotFound = errors.New("task not found")

// Manager tracks every spawned task in this process.
type Manager struct {
	paths config.Paths

	mu    sync.RWMutex
	tasks map[string]*entry
}

type entry struct {
	task   *types.Task
	pty    pty.Pty
	cmd    *pty.Cmd
	logF   *os.File
	ring   *ringBuf
	out    *teeWriter
	done   chan struct{}
	exit   int
	exitMu sync.Mutex

	lnMu sync.Mutex
	ln   net.Listener
}

// New returns a Manager. paths is used to anchor worktrees/logs paths; the
// caller is still responsible for resolving per-task paths in SpawnSpec.
func New(paths config.Paths) *Manager {
	return &Manager{paths: paths, tasks: make(map[string]*entry)}
}

// Spawn starts the process described by spec and returns a Task snapshot.
// The returned Task is owned by the caller — the Manager keeps its own copy.
func (m *Manager) Spawn(ctx context.Context, spec types.SpawnSpec) (*types.Task, error) {
	if spec.ID == "" {
		return nil, errors.New("spawn: empty task id")
	}
	if spec.Agent.Command == "" {
		return nil, errors.New("spawn: agent has empty Command")
	}

	argv := renderArgs(spec)

	logF, err := os.OpenFile(spec.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}

	p, err := pty.New()
	if err != nil {
		logF.Close()
		return nil, fmt.Errorf("open pty: %w", err)
	}

	cmd := p.Command(argv[0], argv[1:]...)
	if spec.Worktree != "" {
		cmd.Dir = spec.Worktree
	}

	if err := cmd.Start(); err != nil {
		_ = p.Close()
		logF.Close()
		return nil, fmt.Errorf("start: %w", err)
	}

	ring := newRingBuf(ringCapacity)
	out := newTeeWriter(logF, ring)

	task := &types.Task{
		ID:        spec.ID,
		Agent:     spec.Agent.Name,
		Worktree:  spec.Worktree,
		Log:       spec.Log,
		PID:       cmd.Process.Pid,
		StartedAt: time.Now().UTC(),
		Status:    types.StatusRunning,
		BudgetUSD: spec.BudgetUSD,
	}

	e := &entry{
		task: task,
		pty:  p,
		cmd:  cmd,
		logF: logF,
		ring: ring,
		out:  out,
		done: make(chan struct{}),
	}

	m.mu.Lock()
	m.tasks[spec.ID] = e
	m.mu.Unlock()

	go e.copyPTY()
	go e.wait()
	if spec.Socket != "" {
		task.Socket = spec.Socket
		go e.serveSocket(spec.Socket)
	}

	return cloneTask(task), nil
}

// IsAlive reports whether the task's process is still running.
func (m *Manager) IsAlive(taskID string) bool {
	e, ok := m.get(taskID)
	if !ok {
		return false
	}
	select {
	case <-e.done:
		return false
	default:
		return true
	}
}

// Kill sends SIGTERM, waits up to gracefulShutdown, then SIGKILL.
// Always closes the PTY and log file.
func (m *Manager) Kill(taskID string) error {
	e, ok := m.get(taskID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, taskID)
	}

	if e.cmd != nil && e.cmd.Process != nil {
		_ = signalTerm(e.cmd.Process)
		select {
		case <-e.done:
		case <-time.After(gracefulShutdown):
			_ = e.cmd.Process.Kill()
			<-e.done
		}
	}
	_ = e.pty.Close()
	_ = e.logF.Close()

	m.mu.Lock()
	delete(m.tasks, taskID)
	m.mu.Unlock()
	return nil
}

// Wait blocks until the task's process exits and returns the exit code.
func (m *Manager) Wait(taskID string) (int, error) {
	e, ok := m.get(taskID)
	if !ok {
		return -1, fmt.Errorf("%w: %s", ErrNotFound, taskID)
	}
	<-e.done
	e.exitMu.Lock()
	defer e.exitMu.Unlock()
	return e.exit, nil
}

// Attach replays recent output to out, then bidirectionally proxies stdin/stdout
// until in returns EOF or the task exits.
//
// Callers are expected to wrap their terminal in raw mode before calling.
func (m *Manager) Attach(taskID string, in io.Reader, out io.Writer) error {
	e, ok := m.get(taskID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, taskID)
	}

	if snap := e.ring.Snapshot(); len(snap) > 0 {
		if _, err := out.Write(snap); err != nil {
			return err
		}
	}

	e.out.attach(out)
	defer e.out.detach(out)

	stop := make(chan struct{})

	go func() {
		if in != nil {
			_, _ = io.Copy(e.pty, in)
		}
		close(stop)
	}()

	select {
	case <-e.done:
	case <-stop:
	}
	return nil
}

func (m *Manager) get(id string) (*entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.tasks[id]
	return e, ok
}

func (e *entry) copyPTY() {
	_, _ = io.Copy(e.out, e.pty)
}

func (e *entry) wait() {
	err := e.cmd.Wait()
	e.exitMu.Lock()
	if err == nil {
		e.exit = 0
		e.task.Status = types.StatusDone
		zero := 0
		e.task.ExitCode = &zero
	} else {
		code := exitCodeOf(err)
		e.exit = code
		e.task.Status = types.StatusFailed
		e.task.ExitCode = &code
	}
	e.exitMu.Unlock()
	close(e.done)
}

func cloneTask(t *types.Task) *types.Task {
	c := *t
	return &c
}

func renderArgs(spec types.SpawnSpec) []string {
	argv := []string{spec.Agent.Command}
	replace := func(s string) string {
		return strings.ReplaceAll(s, "{prompt}", spec.Prompt)
	}
	for _, a := range spec.Agent.Args {
		argv = append(argv, replace(a))
	}
	if spec.Agent.PromptArg != "" {
		argv = append(argv, replace(spec.Agent.PromptArg))
	}
	return argv
}
