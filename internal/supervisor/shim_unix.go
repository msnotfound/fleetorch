//go:build !windows

package supervisor

// maybeWrapShim is a no-op on Unix — .cmd / .bat files aren't invoked
// directly here. The Windows counterpart handles npm-wrapper quoting.
func maybeWrapShim(argv []string) []string { return argv }
