//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func pidAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	return exitCode == uint32(windows.STATUS_PENDING)
}

func signalStop(p *os.Process) error {
	return p.Kill()
}

// signalKillTree mirrors the Unix-side process-group kill on Windows. On
// Windows there is no SIGKILL or POSIX process group; p.Kill() terminates the
// agent process. Child processes that were created with their own job object
// would survive — fleetorch does not currently set up a job object for the PTY
// child, so in practice p.Kill() is the strongest signal we have.
func signalKillTree(p *os.Process) error {
	return p.Kill()
}
