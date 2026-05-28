package supervisor

import "testing"

func TestMaybeWrapShimNoOpForRegularBinaries(t *testing.T) {
	// On all platforms, a path without .cmd/.bat extension is returned unchanged.
	in := []string{"/usr/bin/sh", "-c", "echo hi"}
	out := maybeWrapShim(in)
	if len(out) != len(in) {
		t.Fatalf("len(out) = %d, want %d", len(out), len(in))
	}
	for i, v := range in {
		if out[i] != v {
			t.Errorf("argv[%d] = %q, want %q", i, out[i], v)
		}
	}
}

func TestMaybeWrapShimEmptyArgv(t *testing.T) {
	out := maybeWrapShim(nil)
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}
