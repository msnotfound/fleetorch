//go:build windows

package flock

import (
	"os"
	"syscall"
	"unsafe"
)

const lockfileExclusiveLock = 0x2

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

type platformLock struct {
	file       *os.File
	overlapped syscall.Overlapped
}

func (l *platformLock) lock(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	r1, _, callErr := procLockFileEx.Call(
		file.Fd(),
		uintptr(lockfileExclusiveLock),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&l.overlapped)),
	)
	if r1 == 0 {
		_ = file.Close()
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return syscall.EINVAL
	}
	l.file = file
	return nil
}

func (l *platformLock) unlock() error {
	if l.file == nil {
		return nil
	}
	r1, _, callErr := procUnlockFileEx.Call(
		l.file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&l.overlapped)),
	)
	var err error
	if r1 == 0 {
		if callErr != syscall.Errno(0) {
			err = callErr
		} else {
			err = syscall.EINVAL
		}
	}
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}
