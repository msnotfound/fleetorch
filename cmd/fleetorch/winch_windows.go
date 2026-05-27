//go:build windows

package main

import "os"

// Windows has no SIGWINCH. Resize propagation is a no-op for now; the
// initial size still ships at attach time. A future revision can poll
// console size and synthesize resize frames.
func winchSignal() []os.Signal { return nil }
