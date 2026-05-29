//go:build windows

package supervisor

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

// ProcessCPUTime returns the total CPU time (user + kernel) for pid using
// the Windows GetProcessTimes API.
func ProcessCPUTime(pid int) (time.Duration, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer windows.CloseHandle(h)

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, fmt.Errorf("GetProcessTimes(%d): %w", pid, err)
	}

	// Filetime ticks are 100-nanosecond intervals.
	toNs := func(ft windows.Filetime) uint64 {
		return (uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)) * 100
	}
	total := toNs(kernel) + toNs(user)
	return time.Duration(total), nil
}
