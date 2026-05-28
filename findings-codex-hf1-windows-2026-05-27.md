# fleetorch HF-1 Windows findings

- Tester: Codex
- Date: 2026-05-27
- Platform: Windows amd64, Microsoft Windows NT 10.0.26100.0, Windows PowerShell 5.1.26100.7462
- Baseline: v0.4.0 (`cbf0e6a`)

## Root Cause

HF-1 is not a ConPTY lifetime failure. The PowerShell agent and detached
worker were still running while `fleetorch list` reported `dead`.

`cmd/fleetorch/list.go` used `p.Signal(syscall.Signal(0))` as a PID liveness
probe. That is valid on Unix but not on Windows, where it returned failure for
the live PowerShell process. The same false result made `fleetorch kill` skip
the running child, and its Unix `SIGTERM` behavior was also invalid on Windows.

## Baseline Reproduction

Agent:

```toml
name = "psloop"
command = "powershell"
args = ["-NoLogo", "-NoProfile", "-Command", "Write-Host READY; $i=0; while($true){ $i++; Start-Sleep 2; Write-Host \"tick $i\" }"]
```

With v0.4.0, after spawning `psloop`, the recorded PowerShell PID was still
alive after 11 seconds and continued logging ticks, but `fleetorch list`
reported `dead`. `state.json` remained `running`, disproving a clean child
exit or PTY closure.

Debug trace before the fix reached:

```text
Spawn native-trace: process started PID=18284
Spawn native-trace: Spawn complete, returning task
worker native-trace: task registered, waiting
serveSocket(...native-trace.sock): listening
```

There was no `cmd.Wait returned` line before the false `dead` display.

## Fix

- Use `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` and
  `GetExitCodeProcess` for Windows PID liveness checks.
- Use Windows `Process.Kill` for user-requested termination; keep Unix
  `SIGTERM` behavior in a platform-specific helper.
- Order `kill` status persistence after the detached worker observes the
  forced child exit, avoiding a `dead` to `failed` race.
- Persist `FLEETORCH_DEBUG=1` worker traces to `debug-<task-id>.log` and log
  the child wait result.
- Add a Windows PID-liveness regression test and close PTY resources in the
  existing Windows supervisor test cleanup.

## Verification

- `go test -buildvcs=false -timeout 60s ./...`: passed on Windows.
- PowerShell `psloop` after the fix: `active` at 4 seconds and at 62 seconds;
  log reached `tick 30`.
- Captured `fleetorch attach`: replayed `READY` and live ticks, then detached
  with `Ctrl-] q`.
- `fleetorch kill`: stopped running loops and left status as `dead`.
- Concurrency: three simultaneous `psloop` tasks all showed `active`; all
  three killed cleanly.

Not executed in this automation environment:

- Manual terminal-window resize during `attach`; this requires an interactive,
  resizable terminal window rather than redirected process streams.
- Real codex/claude/gemini spawn; none of those CLIs was available on `PATH`.
