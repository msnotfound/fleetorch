//go:build !windows

package supervisor

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProcessCPUTime returns the total CPU time (user + system) consumed by pid
// by parsing /proc/<pid>/stat fields 14+15. Returns an error on platforms
// without /proc (macOS, BSD) so callers can fall back gracefully.
func ProcessCPUTime(pid int) (time.Duration, error) {
	path := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}

	// /proc/<pid>/stat layout (1-indexed):
	//   (1) pid (2) comm (3) state (4) ppid ... (14) utime (15) stime ...
	//
	// comm can contain spaces and ')', so find the last ')' and parse what
	// follows: state ppid pgroup session tty_nr tpgid flags minflt cminflt
	// majflt cmajflt utime stime ...
	s := string(data)
	lastParen := strings.LastIndex(s, ")")
	if lastParen < 0 {
		return 0, fmt.Errorf("unexpected /proc/%d/stat format", pid)
	}
	fields := strings.Fields(s[lastParen+1:])
	// fields[0]=state fields[1]=ppid ... fields[11]=utime fields[12]=stime
	if len(fields) < 13 {
		return 0, fmt.Errorf("too few fields in /proc/%d/stat", pid)
	}

	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse utime: %w", err)
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse stime: %w", err)
	}

	// USER_HZ is 100 on virtually all Linux configurations.
	const userHZ = 100
	ms := (utime+stime) * 1000 / userHZ
	return time.Duration(ms) * time.Millisecond, nil
}
