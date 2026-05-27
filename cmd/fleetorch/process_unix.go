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

func signalStop(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
