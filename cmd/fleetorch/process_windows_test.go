//go:build windows

package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestPIDAliveWindows(t *testing.T) {
	if !pidAlive(os.Getpid()) {
		t.Fatal("pidAlive reports the current Windows process as dead")
	}

	cmd := exec.Command("cmd.exe", "/C", "exit 0")
	if err := cmd.Run(); err != nil {
		t.Fatalf("run completed process: %v", err)
	}
	if pidAlive(cmd.Process.Pid) {
		t.Fatal("pidAlive reports an exited Windows process as live")
	}
}
