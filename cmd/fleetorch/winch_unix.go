//go:build !windows

package main

import (
	"net"
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

// startWinchWatcher subscribes to SIGWINCH and emits a resize frame on every
// signal. Returns a cancel func.
func startWinchWatcher(conn net.Conn) func() {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, unix.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				signal.Stop(ch)
				return
			case <-ch:
				sendResizeFromTerm(conn)
			}
		}
	}()
	return func() { close(done) }
}
