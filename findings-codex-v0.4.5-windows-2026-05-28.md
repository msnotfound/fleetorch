# fleetorch v0.4.5 Windows automated findings

- Tester: Codex
- Date: 2026-05-28
- Platform: Windows amd64, Microsoft Windows NT 10.0.26100.0, PowerShell 5.1.26100.7462
- fleetorch version tested: `fleetorch version 0.4.5`
- Follow-up build tested: `fleetorch version 0.4.5+agy`

## Summary

- Go tests: passed (`go test -buildvcs=false -timeout 60s ./...`).
- Cross-builds: passed with `CGO_ENABLED=0` for linux/darwin/windows x amd64/arm64.
- Core synthetic-agent flow: mostly passed.
- Real seeded agents on Windows: failed before reaching agent logic for npm `.cmd` shims under a user path with spaces.
- New `agy` builtin support: added and smoke-tested; `agy --print` reaches the prompt and exits 0.

## Automated Coverage That Passed

- `config show`, `agent list`, `doctor`, `doctor --json`, `list --json`.
- Windows default path resolution without `FLEETORCH_HOME`.
- Synthetic PowerShell agent spawn/list/logs.
- Bidirectional `attach` over AF_UNIX socket with input reaching the agent.
- `attach --follow` read-only mode; typed input did not reach the agent.
- `dash` non-TTY refusal and `dash --plain`.
- `kill`, `ledger`, `watch`, `merge-resolve`, `monitor --dry-run`.
- HF-1 regression: PowerShell loop remained `active` past 60 seconds and reached `tick 30`.
- Three concurrent PowerShell loops all showed `active` and killed cleanly.
- `prune --dry-run` and `prune` for finished tasks.
- Completion generation for PowerShell and bash.
- Concurrent `list --json` from five PowerShell jobs.
- Worker error sidecar for a bad command: `logs --err` surfaced the startup failure.
- New `agy` seed: six builtin TOMLs seeded, `doctor` reports `agy`, ledger records `agy`, and `fleetorch spawn agy` completed.

## Findings

### 1. Windows npm `.cmd` shims fail when the user profile path contains spaces

Seeded `codex`, `gemini`, and `claude-haiku` all failed immediately on this machine:

```text
'C:\Users\MAYANK' is not recognized as an internal or external command,
operable program or batch file.
```

The CLIs are installed as npm `.cmd` shims under:

```text
C:\Users\MAYANK SAHU\AppData\Roaming\npm\
```

Using 8.3 short paths for those shims allowed Codex to run successfully, proving the underlying CLI/auth worked:

```text
FLEETORCH_CODEX_OK
```

Gemini and Claude then reached their own CLI-level failures, listed below.

### 2. `logs --err` is blank for clean tasks

For clean tasks, v0.4.5 creates a zero-byte sidecar such as `errors/t1.err`. `fleetorch logs t1 --err` opens the file and prints nothing, rather than the documented:

```text
(no worker errors recorded for this task)
```

This makes clean startup indistinguishable from missing output.

### 3. `agent edit` cannot run an editor path containing spaces

`agent edit` works when `EDITOR=C:\Windows\System32\more.com`.

It fails when `EDITOR` points at a `.cmd` file in a path with spaces:

```text
'C:\Users\MAYANK' is not recognized as an internal or external command,
operable program or batch file.
Error: exit status 1
```

This is likely `exec.Command(resolveEditor(), target)` treating a whole command string/path-with-spaces as the executable without shell parsing or argument splitting.

### 4. `agent add` / `agent remove` mismatch when filename differs from TOML name

`agent add scratch-valid.toml` installs:

```text
agents\scratch-valid.toml
```

if the file contains:

```toml
name = "scratch"
```

`agent list` shows `scratch`, but `agent remove scratch` tries to remove `agents\scratch.toml` and fails because the installed filename was `scratch-valid.toml`.

### 5. Deleting `state.json` mid-run orphans a live task

After deleting `state.json` while `pwchord` was running:

- `fleetorch list` showed no tasks.
- `fleetorch logs state-gone` returned `task not found`.
- `fleetorch prune --include-running` said `no tasks to prune`.
- The PowerShell agent process was still alive.

This is an intentional failure-mode probe, but the recovery story is weak: fleetorch has enough logs/sockets/worktrees on disk to detect likely orphaned workers but does not surface or prune them without state rows.

### 6. Seeded Gemini TOML is stale for Gemini CLI 0.43.0

With a short-path shim and corrected `--prompt`, Gemini reached the CLI but failed:

```text
Gemini CLI is not running in a trusted directory. To proceed, either use `--skip-trust`, set the `GEMINI_CLI_TRUST_WORKSPACE=true` environment variable, or trust this directory in interactive mode.
```

The current seeded TOML only uses:

```toml
args = ["--yolo"]
prompt_arg = "{prompt}"
```

Gemini 0.43.0 documents noninteractive mode as `-p/--prompt` and requires trust handling for headless automation.

### 7. Seeded Claude prompt delivery failed behind short-path shim

With a short-path shim, Claude launched but exited with:

```text
Error: Input must be provided either through stdin or as a prompt argument when using --print
```

The seeded form is `claude -p ... {prompt}`. This may be a wrapper/PTY interaction or a current Claude CLI parsing change; it needs a smaller repro against `claude.cmd` and `claude.exe`.

### 8. `agy` support works through fleetorch with `--print`, not with `--dangerously-skip-permissions`

Fleetorch variant probes showed `agy --print {prompt}` works under ConPTY and printed the requested marker. Adding `--dangerously-skip-permissions` caused agy to ignore the task and respond to the flag text instead. The added builtin uses only `--print`.

## Manual-Only Coverage Not Completed

- True interactive Windows Terminal resize while attached.
- Full-screen `dash` navigation and `K` confirmation behavior in a real TTY.

The automation runner can open sockets, send input, and capture output, but it cannot resize a real terminal window or exercise Bubble Tea key handling in an interactive console.
