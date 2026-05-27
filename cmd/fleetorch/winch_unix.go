//go:build !windows

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func winchSignal() []os.Signal { return []os.Signal{unix.SIGWINCH} }
