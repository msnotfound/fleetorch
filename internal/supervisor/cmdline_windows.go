//go:build windows

package supervisor

import (
	"syscall"

	pty "github.com/aymanbagabas/go-pty"
)

// setShimCmdLine assigns the literal Windows command line that go-pty will
// pass to CreateProcess. go-pty checks c.SysProcAttr.CmdLine first and uses it
// in preference to building one from c.Args, which is exactly what we want for
// the shim wrapping path.
func setShimCmdLine(cmd *pty.Cmd, cmdLine string) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CmdLine = cmdLine
}
