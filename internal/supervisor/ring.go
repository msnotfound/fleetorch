package supervisor

import "sync"

// ringBuf is a fixed-capacity byte ring buffer.
// Used to replay the last N bytes of PTY output when a new client attaches.
type ringBuf struct {
	mu   sync.Mutex
	buf  []byte
	size int
	full bool
	w    int // next write index
}

func newRingBuf(size int) *ringBuf {
	return &ringBuf{buf: make([]byte, size), size: size}
}

func (r *ringBuf) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	for _, b := range p {
		r.buf[r.w] = b
		r.w++
		if r.w == r.size {
			r.w = 0
			r.full = true
		}
	}
	return n, nil
}

// Snapshot returns the current contents in order (oldest first).
func (r *ringBuf) Snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		out := make([]byte, r.w)
		copy(out, r.buf[:r.w])
		return out
	}
	out := make([]byte, r.size)
	copy(out, r.buf[r.w:])
	copy(out[r.size-r.w:], r.buf[:r.w])
	return out
}
