package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/supervisor"
)

func newAttachCmdReal() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "attach <task-id>",
		Short: "Drop into the live PTY of a running task (or --follow for read-only log tail)",
		Long: `Attach to a running task's PTY. Anything you type is sent to the agent;
output streams to your terminal. Detach with Ctrl-] q. Terminal resizes
(SIGWINCH) are propagated to the agent's PTY.

If the task's control socket is unavailable, falls back to read-only log
tail. Use --follow to force log-tail mode.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return doAttach(args[0], follow)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Skip socket; tail log file only (read-only)")
	return cmd
}

func doAttach(taskID string, followOnly bool) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	st := store.New(paths.StateFile)
	task, err := st.GetTask(taskID)
	if err != nil {
		return err
	}

	if !followOnly && task.Socket != "" && socketAlive(task.Socket) {
		return attachSocket(task.Socket)
	}
	return followLog(task.Log, task.PID)
}

func socketAlive(path string) bool {
	c, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// attachSocket proxies the terminal to the task's framed PTY socket.
// Detach sequence: Ctrl-] (0x1d) followed by 'q'.
func attachSocket(path string) error {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return fmt.Errorf("dial socket: %w", err)
	}
	defer conn.Close()

	stdinFd := int(os.Stdin.Fd())
	restore := func() {}
	if term.IsTerminal(stdinFd) {
		old, err := term.MakeRaw(stdinFd)
		if err == nil {
			restore = func() { _ = term.Restore(stdinFd, old) }
		}
	}
	defer restore()

	sendResizeFromTerm(conn) // initial size

	// Watch for terminal resize and propagate.
	winch := startWinchWatcher(conn)
	defer winch()

	fmt.Fprint(os.Stderr, "[attached — detach with Ctrl-] q]\r\n")

	var once sync.Once
	done := make(chan struct{})
	stop := func() { once.Do(func() { close(done) }) }

	// Socket → stdout (parse frames).
	go func() {
		for {
			typ, payload, err := supervisor.ReadFrame(conn)
			if err != nil {
				stop()
				return
			}
			if typ == supervisor.FrameData && len(payload) > 0 {
				if _, werr := os.Stdout.Write(payload); werr != nil {
					stop()
					return
				}
			}
		}
	}()

	// Stdin → socket (data frames, filtered for detach sequence).
	go func() {
		buf := make([]byte, 1024)
		escape := false
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				filtered := make([]byte, 0, n)
				for _, b := range buf[:n] {
					if escape {
						if b == 'q' || b == 'Q' {
							if len(filtered) > 0 {
								_ = supervisor.WriteFrame(conn, supervisor.FrameData, filtered)
							}
							stop()
							return
						}
						filtered = append(filtered, 0x1d, b)
						escape = false
						continue
					}
					if b == 0x1d {
						escape = true
						continue
					}
					filtered = append(filtered, b)
				}
				if len(filtered) > 0 {
					if werr := supervisor.WriteFrame(conn, supervisor.FrameData, filtered); werr != nil {
						stop()
						return
					}
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					_, _ = fmt.Fprintf(os.Stderr, "\r\n[stdin error: %v]\r\n", err)
				}
				stop()
				return
			}
		}
	}()

	<-done
	fmt.Fprint(os.Stderr, "\r\n[detached]\r\n")
	return nil
}

func sendResizeFromTerm(conn net.Conn) {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return
	}
	_ = supervisor.WriteFrame(conn, supervisor.FrameResize,
		supervisor.ResizePayload(uint16(rows), uint16(cols)))
}

// followLog is the read-only fallback for tasks without a live socket.
func followLog(path string, pid int) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(os.Stdout, f); err != nil {
		return err
	}
	for {
		if pid > 0 && !pidAlive(pid) {
			fmt.Fprintln(os.Stderr, "\n[task exited]")
			return nil
		}
		if _, err := io.Copy(os.Stdout, f); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// startWinchWatcher returns a cancel func. SIGWINCH is Unix-only; on
// Windows the watcher is a no-op (return value still safe to call).
func startWinchWatcher(conn net.Conn) func() {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, winchSignal()...)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				signal.Stop(ch)
				return
			case <-ch:
				sendResizeFromTerm(conn)
			}
		}
	}()
	return func() { close(done) }
}
