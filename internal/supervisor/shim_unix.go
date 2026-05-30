//go:build !windows

package supervisor

// maybeWrapShim is a no-op on Unix — .cmd / .bat files aren't invoked
// directly here. The Windows counterpart handles npm-wrapper quoting.
func maybeWrapShim(argv []string) []string { return argv }

// buildShimCmdLine is a Windows-only feature; on Unix it always returns "".
// See shim_windows.go for the rationale.
func buildShimCmdLine(argv []string) string { return "" }
