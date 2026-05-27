//go:build windows

package main

import (
	"net"
	"time"
)

const winchPollInterval = 250 * time.Millisecond

// startWinchWatcher polls the terminal size and emits a resize frame whenever
// it changes. Windows has no SIGWINCH; this is the cheapest cross-platform
// proxy for it.
func startWinchWatcher(conn net.Conn) func() {
	done := make(chan struct{})
	lastRows, lastCols, _ := currentTermSize()
	go func() {
		t := time.NewTicker(winchPollInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				rows, cols, ok := currentTermSize()
				if !ok {
					continue
				}
				if rows != lastRows || cols != lastCols {
					sendResize(conn, rows, cols)
					lastRows, lastCols = rows, cols
				}
			}
		}
	}()
	return func() { close(done) }
}
