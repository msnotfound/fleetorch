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
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aymanbagabas/go-pty"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/types"
)

// debugLogging is true when FLEETORCH_DEBUG=1 is set in the environment.
// When enabled, Spawn emits log.Printf lines at each lifecycle boundary so
// Windows testers can pinpoint where the flow stalls (AF_UNIX unavailability,
// PTY allocation blocking, etc.).
var debugLogging = os.Getenv("FLEETORCH_DEBUG") == "1"

func debugf(format string, args ...any) {
	if debugLogging {
		log.Printf("[fleetorch-debug] "+format, args...)
	}
}

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

	// --- Bug fix: resolve bare command names via PATH on all platforms.
	//
	// On Windows, go-pty / ConPTY does not perform PATH lookup for the
	// executable the way Unix exec does; passing "powershell" resolves
	// relative to the worktree instead of %PATH%, producing:
	//   exec: "...\worktrees\foo\powershell": executable file not found in %PATH%
	//
	// Calling exec.LookPath here normalises the path to an absolute one before
	// it ever reaches go-pty, fixing every seeded agent TOML on Windows.
	debugf("Spawn %s: raw command %q argv=%v", spec.ID, argv[0], argv[1:])
	resolved, lookErr := exec.LookPath(argv[0])
	if lookErr != nil {
		return nil, fmt.Errorf("agent command %q not found on PATH: %w", argv[0], lookErr)
	}
	debugf("Spawn %s: resolved %q → %q", spec.ID, argv[0], resolved)
	argv[0] = resolved

	// --- Windows-only: wrap .cmd / .bat shims via cmd.exe /C so the shim
	// path is quoted properly. Direct CreateProcess of an npm-wrapper .cmd
	// under a user profile with spaces (e.g. "C:\Users\MAYANK SAHU\...\codex.cmd")
	// fails with "'C:\Users\MAYANK' is not recognized…". No-op on Unix.
	if wrapped := maybeWrapShim(argv); &wrapped[0] != &argv[0] || len(wrapped) != len(argv) {
		debugf("Spawn %s: wrapped shim via cmd.exe /C: %v", spec.ID, wrapped)
		argv = wrapped
	}

	logF, err := os.OpenFile(spec.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	debugf("Spawn %s: log file opened at %s", spec.ID, spec.Log)

	// --- Blocking call 1: pty.New()
	// On Windows this allocates a ConPTY (ConCreatePseudoConsole). If the
	// Windows build is too old or ConPTY is unavailable, this may hang or
	// return an error. FLEETORCH_DEBUG=1 lets testers see whether we make it
	// past this point.
	p, err := pty.New()
	if err != nil {
		logF.Close()
		return nil, fmt.Errorf("open pty: %w", err)
	}
	debugf("Spawn %s: PTY allocated", spec.ID)

	cmd := p.Command(argv[0], argv[1:]...)
	if spec.Worktree != "" {
		cmd.Dir = spec.Worktree
	}

	// --- Blocking call 2: cmd.Start()
	// Forks the process. On Windows this also wires the ConPTY handles.
	if err := cmd.Start(); err != nil {
		_ = p.Close()
		logF.Close()
		return nil, fmt.Errorf("start: %w", err)
	}
	debugf("Spawn %s: process started PID=%d", spec.ID, cmd.Process.Pid)

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
	debugf("Spawn %s: registered in task map", spec.ID)

	// --- Goroutine 1: copyPTY
	// Reads from the PTY master and fans out to the log + ring buffer.
	// On Windows this may block if ConPTY produces no output immediately —
	// that is normal and should not prevent the socket from being created.
	go e.copyPTY()

	// --- Goroutine 2: wait
	// Calls cmd.Wait() which blocks until the child process exits.
	go e.wait()

	if spec.Socket != "" {
		task.Socket = spec.Socket
		// --- Goroutine 3: serveSocket
		// Creates the Unix-domain socket and accepts attach clients.
		// On Windows, AF_UNIX requires Win10 build 1803+. If net.Listen fails
		// (e.g. older Windows, or the sockets directory doesn't exist), it now
		// logs to stderr instead of silently returning so the failure is visible.
		debugf("Spawn %s: launching serveSocket at %s", spec.ID, spec.Socket)
		go e.serveSocket(spec.Socket)
	}

	if spec.PipeStdoutTo != "" {
		sockPath := filepath.Join(m.paths.SocketDir, spec.PipeStdoutTo+".sock")
		debugf("Spawn %s: dialing pipe-stdout-to socket %s", spec.ID, sockPath)
		go e.dialPipe(sockPath)
	}

	debugf("Spawn %s: Spawn complete, returning task", spec.ID)
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
	debugf("process %s: cmd.Wait returned err=%v", e.task.ID, err)
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

// dialPipe connects to a peer task's control socket and adds a self-removing
// framed writer to this task's tee chain. If the socket is unavailable the
// function logs a warning and returns — the spawned task continues unaffected.
// If the socket dies mid-stream the pipeWriter detaches itself silently.
func (e *entry) dialPipe(sockPath string) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		log.Printf("fleetorch: --pipe-stdout-to %s: connect failed: %v (pipe disabled)", sockPath, err)
		return
	}
	pw := &pipeWriter{conn: conn, fw: newFrameWriter(conn), tee: e.out}
	e.out.attach(pw)
	<-e.done
	e.out.detach(pw)
	conn.Close()
}

// pipeWriter is a framed io.Writer that removes itself from the tee chain when
// a write error occurs (i.e. the target socket closed).
type pipeWriter struct {
	mu   sync.Mutex
	conn net.Conn
	fw   *frameWriter
	tee  *teeWriter
	dead bool
}

func (p *pipeWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dead {
		return len(b), nil
	}
	if _, err := p.fw.Write(b); err != nil {
		p.dead = true
		// Detach asynchronously to avoid a deadlock: teeWriter.Write holds
		// its RLock while calling us; tee.detach needs the write lock.
		go func() {
			p.tee.detach(p)
			p.conn.Close()
		}()
	}
	return len(b), nil
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
