package supervisor

import (
	"io"
	"sync"
)

// teeWriter writes to a primary sink (log + ring) plus any number of
// dynamically attached secondary sinks (terminals via Attach).
type teeWriter struct {
	primary []io.Writer

	mu       sync.RWMutex
	attached map[io.Writer]struct{}
}

func newTeeWriter(log io.Writer, ring io.Writer) *teeWriter {
	return &teeWriter{
		primary:  []io.Writer{log, ring},
		attached: make(map[io.Writer]struct{}),
	}
}

func (t *teeWriter) Write(p []byte) (int, error) {
	for _, w := range t.primary {
		_, _ = w.Write(p)
	}
	t.mu.RLock()
	for w := range t.attached {
		_, _ = w.Write(p)
	}
	t.mu.RUnlock()
	return len(p), nil
}

func (t *teeWriter) attach(w io.Writer) {
	t.mu.Lock()
	t.attached[w] = struct{}{}
	t.mu.Unlock()
}

func (t *teeWriter) detach(w io.Writer) {
	t.mu.Lock()
	delete(t.attached, w)
	t.mu.Unlock()
}
