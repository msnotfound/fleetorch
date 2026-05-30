//go:build windows

package supervisor

import "testing"

func TestMaybeWrapShimWrapsCmdShim(t *testing.T) {
	in := []string{`C:\Users\MAYANK SAHU\AppData\Roaming\npm\gemini.cmd`, "--yolo", "-p", "hi there"}
	out := maybeWrapShim(in)
	if len(out) != len(in)+3 {
		t.Fatalf("expected wrap to add 3 prefix elements, got %v", out)
	}
	if out[1] != "/S" || out[2] != "/C" {
		t.Errorf("expected [cmd.exe /S /C ...], got %v", out[:3])
	}
}

func TestBuildShimCmdLinePreservesQuoting(t *testing.T) {
	argv := []string{
		`C:\Windows\System32\cmd.exe`,
		"/S", "/C",
		`C:\Users\MAYANK SAHU\AppData\Roaming\npm\gemini.cmd`,
		"--skip-trust",
		"--yolo",
		"-p",
		"hi there",
	}
	cmdLine := buildShimCmdLine(argv)
	if cmdLine == "" {
		t.Fatal("expected non-empty cmdline for wrapped shim")
	}
	// Must contain /S /C outer wrap.
	if !contains(cmdLine, "/S /C") {
		t.Errorf("missing /S /C in %q", cmdLine)
	}
	// Shim path must be quoted (contains spaces).
	if !contains(cmdLine, `"C:\Users\MAYANK SAHU\AppData\Roaming\npm\gemini.cmd"`) {
		t.Errorf("shim path not quoted in %q", cmdLine)
	}
	// Multi-word prompt must be quoted as a single arg.
	if !contains(cmdLine, `"hi there"`) {
		t.Errorf("multi-word prompt not quoted in %q", cmdLine)
	}
	// Plain args (no spaces) should NOT be quoted.
	if contains(cmdLine, `"--yolo"`) {
		t.Errorf("flag should not be quoted in %q", cmdLine)
	}
}

func TestBuildShimCmdLineReturnsEmptyForNonShim(t *testing.T) {
	// Regular binary, not wrapped — no cmdLine override.
	argv := []string{`C:\Users\foo\codex.exe`, "--help"}
	if got := buildShimCmdLine(argv); got != "" {
		t.Errorf("expected empty for non-shim argv, got %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
