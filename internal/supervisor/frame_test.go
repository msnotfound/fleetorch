package supervisor

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, FrameData, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFrame(&buf, FrameResize, ResizePayload(24, 80)); err != nil {
		t.Fatal(err)
	}

	typ, payload, err := ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if typ != FrameData || string(payload) != "hello" {
		t.Errorf("first frame: got typ=%c payload=%q", typ, payload)
	}

	typ, payload, err = ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if typ != FrameResize {
		t.Errorf("expected resize, got %c", typ)
	}
	rows, cols, ok := ParseResize(payload)
	if !ok || rows != 24 || cols != 80 {
		t.Errorf("resize: rows=%d cols=%d ok=%v", rows, cols, ok)
	}
}

func TestFrameWriterChunks(t *testing.T) {
	var buf bytes.Buffer
	fw := newFrameWriter(&buf)
	big := bytes.Repeat([]byte("x"), maxPayload+100)
	n, err := fw.Write(big)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(big) {
		t.Errorf("wrote %d, want %d", n, len(big))
	}
	// Expect two data frames: one full, one with 100 bytes.
	typ1, p1, err := ReadFrame(&buf)
	if err != nil || typ1 != FrameData || len(p1) != maxPayload {
		t.Errorf("first frame: typ=%c len=%d err=%v", typ1, len(p1), err)
	}
	typ2, p2, err := ReadFrame(&buf)
	if err != nil || typ2 != FrameData || len(p2) != 100 {
		t.Errorf("second frame: typ=%c len=%d err=%v", typ2, len(p2), err)
	}
}
