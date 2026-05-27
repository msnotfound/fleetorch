package supervisor

import (
	"errors"
	"io"
	"net"
	"os"
)

// serveSocket listens on the entry's control socket and exposes the running
// PTY to any number of concurrent clients. Each client gets a replay of the
// ring buffer, a live tee of PTY output, and stdin written to the PTY.
//
// Returns when the listener closes (Kill or process exit closes it).
func (e *entry) serveSocket(path string) {
	_ = os.Remove(path) // stale sock from a previous run

	ln, err := net.Listen("unix", path)
	if err != nil {
		// Best-effort: socket is optional. attach falls back to read-only follow.
		return
	}
	e.lnMu.Lock()
	e.ln = ln
	e.lnMu.Unlock()

	go func() {
		<-e.done
		_ = ln.Close()
		_ = os.Remove(path)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go e.handleClient(conn)
	}
}

func (e *entry) handleClient(conn net.Conn) {
	defer conn.Close()

	// Replay recent output so the client has context.
	if snap := e.ring.Snapshot(); len(snap) > 0 {
		_, _ = conn.Write(snap)
	}

	// Live tee.
	e.out.attach(conn)
	defer e.out.detach(conn)

	// Forward client stdin into the PTY. When client closes, this returns.
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(e.pty, conn)
		close(done)
	}()

	select {
	case <-done:
	case <-e.done:
	}
}

