//go:build !windows

package supervisor

import (
	"os"
	"os/exec"
	"syscall"
)

func signalTerm(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

func exitCodeOf(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}
