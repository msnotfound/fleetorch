//go:build linux

package store

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// socketPID returns the PID of the process listening on socketPath by
// consulting /proc/net/unix (socket inode) and /proc/*/fd (fd→inode map).
// Returns 0 if the PID cannot be determined.
func socketPID(socketPath string) int {
	inode, err := socketInode(socketPath)
	if err != nil || inode == 0 {
		return 0
	}
	return pidForInode(inode)
}

// socketInode reads /proc/net/unix to find the kernel inode for the path.
func socketInode(socketPath string) (uint64, error) {
	f, err := os.Open("/proc/net/unix")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// Format: Num RefCount Protocol Flags Type St Inode Path
		if len(fields) < 8 {
			continue
		}
		if fields[7] == socketPath {
			inode, err := strconv.ParseUint(fields[6], 10, 64)
			if err != nil {
				continue
			}
			return inode, nil
		}
	}
	return 0, nil
}

// pidForInode scans /proc/*/fd to find the PID that holds a file descriptor
// with the given socket inode.
func pidForInode(inode uint64) int {
	target := fmt.Sprintf("socket:[%d]", inode)

	proc, err := os.Open("/proc")
	if err != nil {
		return 0
	}
	defer proc.Close()

	pids, err := proc.Readdirnames(-1)
	if err != nil {
		return 0
	}

	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 {
			continue
		}
		fdDir := filepath.Join("/proc", pidStr, "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if link == target {
				return pid
			}
		}
	}
	return 0
}
