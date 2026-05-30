package supervisor

import (
	"errors"
	"fmt"
	"net"
	"os"
)

func listenSocket(path string) (net.Listener, error) {
	_ = os.Remove(path) // stale sock from a previous run

	ln, err := net.Listen("unix", path)
	if err != nil {
		debugf("serveSocket(%s): net.Listen failed: %v", path, err)
		return nil, fmt.Errorf("net.Listen unix %s: %w", path, err)
	}
	debugf("serveSocket(%s): listening", path)
	return ln, nil
}

// serveSocket exposes the running PTY to any number of concurrent clients.
// Each client gets a replay of the ring buffer, a live tee of PTY output, and
// stdin written to the PTY.
//
// Returns when the listener closes (Kill or process exit closes it).
func (e *entry) serveSocket(path string, ln net.Listener) {
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

	fw := newFrameWriter(conn)

	// Replay recent output so the client has context.
	if snap := e.ring.Snapshot(); len(snap) > 0 {
		_, _ = fw.Write(snap)
	}

	// Live tee. PTY output gets framed via fw before hitting the socket.
	e.out.attach(fw)
	defer e.out.detach(fw)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			typ, payload, err := ReadFrame(conn)
			if err != nil {
				return
			}
			switch typ {
			case FrameData:
				if _, werr := e.pty.Write(payload); werr != nil {
					return
				}
			case FrameResize:
				if rows, cols, ok := ParseResize(payload); ok {
					_ = e.pty.Resize(int(cols), int(rows))
				}
			default:
				// Unknown frame type — ignore for forward compatibility.
			}
		}
	}()

	select {
	case <-done:
	case <-e.done:
	}
}
