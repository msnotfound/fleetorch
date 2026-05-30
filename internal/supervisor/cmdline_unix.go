//go:build !windows

package supervisor

import pty "github.com/aymanbagabas/go-pty"

// setShimCmdLine is a no-op on Unix. The shim quoting problem it solves is
// a Windows + cmd.exe-specific quirk.
func setShimCmdLine(_ *pty.Cmd, _ string) {}
