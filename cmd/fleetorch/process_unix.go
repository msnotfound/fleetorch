//go:build !windows

package main

import (
	"os"
	"syscall"
)

func pidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// signalStop tries to terminate the agent and any sub-processes it spawned.
// The PTY-attached child is a session leader (TIOCSCTTY) and heads its own
// process group, so SIGTERM to the negative pgid hits the whole tree. A TUI
// agent that traps SIGTERM on the main process (gemini, claude, etc.) will
// still receive it via the group. Falls back to single-PID signal if pgid
// lookup fails for any reason.
func signalStop(p *os.Process) error {
	if pgid, err := syscall.Getpgid(p.Pid); err == nil {
		if perr := syscall.Kill(-pgid, syscall.SIGTERM); perr == nil {
			return nil
		}
	}
	return p.Signal(syscall.SIGTERM)
}

// signalKillTree force-kills the process group, used as the escalation when
// SIGTERM did not bring the agent down within the grace window. Without this
// step, SIGKILL on the lone PTY child leaves child node/python processes
// re-parented to init.
func signalKillTree(p *os.Process) error {
	if pgid, err := syscall.Getpgid(p.Pid); err == nil {
		if perr := syscall.Kill(-pgid, syscall.SIGKILL); perr == nil {
			return nil
		}
	}
	return p.Kill()
}
