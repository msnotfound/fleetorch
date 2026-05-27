//go:build windows

package supervisor

import (
	"os"
	"os/exec"
)

// On Windows there is no SIGTERM. Kill() is the only graceful option exposed
// by os.Process. ConPTY tears the child down when the master closes.
func signalTerm(p *os.Process) error {
	return p.Kill()
}

func exitCodeOf(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}
