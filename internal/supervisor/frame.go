package supervisor

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Wire format on the per-task control socket (v0.3+):
//
//   +------+------------+------------------+
//   | type | length (BE)|     payload      |
//   | 1 b  |     2 b    |    len bytes     |
//   +------+------------+------------------+
//
// FrameData carries PTY stdio bytes. FrameResize carries 4 bytes: rows then
// cols as big-endian uint16. Anything else is reserved for future use and
// must be ignored by older readers.
const (
	FrameData   byte = 'd'
	FrameResize byte = 'r'

	maxPayload = 65535 // fits in uint16 length field
)

var errPayloadTooLarge = errors.New("frame: payload exceeds max")

// WriteFrame encodes a single frame to w. Always writes the full frame or
// returns an error.
func WriteFrame(w io.Writer, typ byte, payload []byte) error {
	if len(payload) > maxPayload {
		return errPayloadTooLarge
	}
	var hdr [3]byte
	hdr[0] = typ
	binary.BigEndian.PutUint16(hdr[1:3], uint16(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

// ReadFrame decodes one frame from r. Returns the type and payload (a fresh
// slice owned by the caller).
func ReadFrame(r io.Reader) (typ byte, payload []byte, err error) {
	var hdr [3]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	typ = hdr[0]
	n := binary.BigEndian.Uint16(hdr[1:3])
	if n == 0 {
		return typ, nil, nil
	}
	if int(n) > maxPayload {
		return 0, nil, fmt.Errorf("frame: payload length %d exceeds max %d", n, maxPayload)
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return typ, payload, nil
}

// ResizePayload encodes (rows, cols) into the 4-byte payload of a resize frame.
func ResizePayload(rows, cols uint16) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:2], rows)
	binary.BigEndian.PutUint16(b[2:4], cols)
	return b
}

// ParseResize decodes a resize frame payload into (rows, cols).
func ParseResize(payload []byte) (rows, cols uint16, ok bool) {
	if len(payload) < 4 {
		return 0, 0, false
	}
	return binary.BigEndian.Uint16(payload[0:2]), binary.BigEndian.Uint16(payload[2:4]), true
}

// frameWriter wraps a raw conn so that Write([]byte) emits framed data.
// teeWriter sees this as a plain io.Writer.
type frameWriter struct {
	w io.Writer
}

func newFrameWriter(w io.Writer) *frameWriter { return &frameWriter{w: w} }

func (f *frameWriter) Write(p []byte) (int, error) {
	// Split into max-payload chunks so very large bursts still encode.
	written := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > maxPayload {
			chunk = chunk[:maxPayload]
		}
		if err := WriteFrame(f.w, FrameData, chunk); err != nil {
			return written, err
		}
		written += len(chunk)
		p = p[len(chunk):]
	}
	return written, nil
}
