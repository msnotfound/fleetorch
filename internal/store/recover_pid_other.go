//go:build !linux

package store

// socketPID returns 0 on non-Linux platforms where /proc is not available.
func socketPID(_ string) int { return 0 }
