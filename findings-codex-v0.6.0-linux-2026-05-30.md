# fleetorch test findings

- Tester: codex
- Date: 2026-05-30
- Platform: linux x86_64, Ubuntu kernel `6.8.0-86-generic`
- fleetorch version tested: `fleetorch version 0.6.0`
- Test home: `/tmp/fo-home-codex-20260530`

## Doctor summary

`fleetorch doctor` under the default home reported:

- `fleetorch 0.6.0`, Go `go1.25.0`, built for `linux/amd64`
- `git`, `codex`, `gemini`, and `claude` found on PATH
- `agy` not on PATH
- `AF_UNIX --`
- Warning: `AF_UNIX is not available; attach will fall back to --follow`

With isolated `FLEETORCH_HOME=/tmp/fo-home-codex-20260530`, first-run seeding installed all six default agents: `agy`, `codex`, `gemini`, `claude-haiku`, `claude-sonnet`, `claude-opus`.

## Summary

- Tests run: repo unit tests, first-run defaults, doctor/json, agent list/add/edit/remove, synthetic spawn/logs/list/follow, real Codex/Gemini/Claude spawns, failure-mode probes, ledger, merge-resolve, monitor dry-run, dash non-TTY refusal, policy show, preset list, prune, CLI help/docs sanity.
- Hard failures: 1 environment-blocking runtime issue (`AF_UNIX` socket creation fails in this sandbox).
- Soft failures / weirdness: 3.
- Doc drift: 4.

## Verification

- `go test ./...`: PASS
  - `cmd/fleetorch`, `internal/agents`, `internal/config`, `internal/store`, and `internal/supervisor` passed.
- `fleetorch --version`: PASS, printed `fleetorch version 0.6.0`.
- `fleetorch doctor --json`: PASS, parseable JSON with expected paths/deps/warnings.

## Hard failures

### H1: Runtime attach socket cannot be created in this sandbox

- Section: 3a/3b synthetic spawn and attach surface
- Command: `env FLEETORCH_HOME=/tmp/fo-home-codex-20260530 fleetorch spawn sleepy sfg x --foreground`
- Expected: foreground worker starts socket and keeps `sleepy` alive for `sleep 60`
- Actual: `fleetorch: serveSocket(/tmp/fo-home-codex-20260530/sockets/sfg.sock): net.Listen unix failed: listen unix ... setsockopt: operation not permitted`
- Suspected cause: sandbox disallows the Unix socket option fleetorch uses. This matches `doctor` reporting `AF_UNIX --`.
- Impact: live PTY attach, multi-client broadcast, resize, and true concurrent-agent behavior could not be fully validated from this Codex sandbox.

## Soft failures / weirdness

### S1: Detached synthetic tasks died immediately when socket setup was unavailable

- Commands:
  - `fleetorch spawn shechord t1 hello-prompt`
  - `fleetorch spawn sleepy s1 x`
  - `fleetorch list`
  - `fleetorch logs t1`
  - `fleetorch logs s1`
- Expected: `shechord` and `sleepy` stay active.
- Actual: both quickly displayed as `dead`; logs only showed `READY`.
- Worker err logs were empty: `fleetorch logs s1 --err` printed `(no worker errors recorded for this task)`.
- This is probably the same AF_UNIX sandbox limitation, but the detached path does not surface the `setsockopt` failure in `logs --err`.

### S2: `list --json` reports stale persisted status alongside live status

- Command: `fleetorch list --json`
- Actual for dead synthetic tasks: `"status": "running"` with `"live_status": "dead"`.
- Table output showed `STATUS dead`.
- This may be intentional in v0.6.0 because `live_status` is the computed state, but JSON consumers need to know `status` alone can be stale.

### S3: `fleetorch config edit` without `EDITOR=cat` launched Vim in non-TTY

- Command: `env FLEETORCH_HOME=/tmp/fo-home-codex-20260530 fleetorch config edit`
- Actual: Vim warned `Output is not to a terminal` / `Input is not from a terminal` and remained open.
- With `EDITOR=cat`, the same command printed the config correctly.
- Suggested behavior: in non-TTY contexts, either refuse with a clear message or require `$EDITOR` to be non-interactive.

## Passing CLI checks

- `fleetorch agent edit shechord` with `EDITOR=cat`: PASS, printed TOML.
- `fleetorch agent add /tmp/scratch-fleetorch-codex.toml`: PASS, installed as `scratch.toml`.
- `fleetorch agent remove scratch`: PASS.
- `fleetorch merge-resolve /tmp/conflict-fleetorch-codex.txt --dry-run`: PASS, reported one block.
- `fleetorch merge-resolve /tmp/conflict-fleetorch-codex.txt`: PASS, produced:
  - `header`
  - `ours-line`
  - `theirs-line`
  - `footer`
- `fleetorch attach ghost`: PASS, clear `task not found` error.
- `fleetorch spawn nonsense tx x`: PASS, clear `unknown agent type` error.
- `fleetorch dash` without TTY: PASS, refused with `dash requires a terminal. Use --plain for a non-interactive table.`
- `fleetorch monitor --dry-run --interval 2s`: PASS, polled and reported active/failed counts.
- `fleetorch ledger`: PASS, showed synthetic spawn counts.
- `fleetorch kill sfg` after exit: PASS, printed `task sfg already exited (status: done); nothing to kill`.
- `fleetorch preset list` with isolated `FLEETORCH_HOME`: PASS, seeded `bugfix-swarm`, `feature-squad`, and `research-team`.
- `fleetorch policy show`: PASS.
- `fleetorch prune --dry-run` and `fleetorch prune`: PASS after a sequential re-check; list was empty after prune.

## Stale TOMLs

- `codex.toml`: OK against installed `codex exec --help`; `exec`, `--sandbox workspace-write`, and `--skip-git-repo-check` are still valid.
- `gemini.toml`: stale for current installed Gemini CLI behavior. `--yolo` still exists, but positional prompt args now default to interactive mode. Headless automation needs `-p/--prompt`; untrusted worktrees also need `--skip-trust` or equivalent trust configuration.
- `claude-haiku.toml`: stale for current installed Claude CLI behavior through Fleetorch. The seeded TOML has `prompt_arg = "{prompt}"`, but the Fleetorch-spawned Claude process failed with `Input must be provided either through stdin or as a prompt argument when using --print`. A temporary TOML with `{prompt}` embedded directly in `args` succeeded.
- `agy.toml`: not tested; `agy` is not on PATH.

## Real agent run

These were run outside the Codex sandbox with `FLEETORCH_HOME=/tmp/fo-real-agents-20260530`, where `fleetorch doctor` reported `AF_UNIX OK`.

### Codex seeded agent

- Command: `fleetorch spawn codex real-codex-1 "Print the SHA-256 of the string 'hello'. Output only the hex digest." --repo .`
- Status: `done`
- Log included expected digest: `2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824`
- Worker error log: clean.
- Worktree status: clean.

### Gemini seeded agent

- Command: `fleetorch spawn gemini real-gemini-1 "Write exactly one sentence: FLEETORCH_GEMINI_OK" --repo .`
- Status: produced `FLEETORCH_GEMINI_OK`, but stayed open in interactive mode and was killed manually.
- Log warning: `Positional arguments now default to interactive mode. To run in non-interactive mode, use the --prompt (-p) flag.`
- Interpretation: seeded Gemini TOML launches the current Gemini CLI in interactive mode rather than one-shot mode.

### Claude seeded agent

- Command: `fleetorch spawn claude-haiku real-claude-1 "Print exactly FLEETORCH_CLAUDE_OK and exit." --repo .`
- Status: `failed`
- Log: `Error: Input must be provided either through stdin or as a prompt argument when using --print`
- Worker error log: clean.
- Direct CLI check succeeded: `claude -p --model haiku --max-turns 1 "Print exactly FLEETORCH_CLAUDE_DIRECT_OK and exit."` printed `FLEETORCH_CLAUDE_DIRECT_OK`.
- Interpretation: Claude itself works; the seeded Fleetorch invocation shape is the failing piece.

### Temporary corrected Gemini agent

- TOML shape: `args = ["--skip-trust", "--yolo", "-p", "{prompt}"]`
- Command: `fleetorch spawn gemini-p-skip-trust real-gemini-p2-1 "Print exactly FLEETORCH_GEMINI_P2_OK and exit." --repo .`
- Status: `done`
- Log included `FLEETORCH_GEMINI_P2_OK`
- Worker error log: clean.
- Note: a version with `-p` but without `--skip-trust` failed on Gemini's trusted-folder gate.

### Temporary corrected Claude agent

- TOML shape: `args = ["-p", "{prompt}", "--model", "haiku", "--verbose", "--max-turns", "5", ...]`
- Command: `fleetorch spawn claude-haiku-prompt real-claude-p-1 "Print exactly FLEETORCH_CLAUDE_P_OK and exit." --repo .`
- Status: `done`
- Log included `FLEETORCH_CLAUDE_P_OK`
- Worker error log: clean.

## Doc drift

1. `TESTING.md` still says latest release is `v0.4.0`; installed/repo version is `v0.6.0`.
2. `README.md` CLI reference omits `policy` and `preset`, both present in `fleetorch --help`.
3. `README.md` says remote `fleetorch agent install <url>` registry is planned for `v0.5`; current version is `v0.6.0` and that command is not present.
4. `CHANGELOG.md` includes v0.5.0 and v0.6.0 entries, but bottom compare links stop at v0.4.8 and `[Unreleased]` compares `v0.4.8...HEAD`.

## Untested / blocked

- Full bidirectional attach, multi-client broadcast, and PTY resize were blocked in the Codex sandbox by the AF_UNIX socket failure. Real-agent spawns were rerun outside the sandbox, where `AF_UNIX OK`.
- Real `agy` was not run because `agy` is not on PATH.
- Install/upgrade paths were not rerun because the user stated the latest version was already installed and running.

## Suggested fixes

1. Surface detached worker socket setup failures through `logs <id> --err`; foreground revealed the root cause, detached mode did not.
2. Clarify `list --json` semantics for `status` vs `live_status`, or make `status` reflect computed live state if JSON consumers should not inspect both.
3. Make `config edit` / `agent edit` refuse interactive editors when stdin/stdout are not TTYs unless `$EDITOR` is explicitly set.
4. Update `TESTING.md`, README CLI reference, README v0.5 plugin note, and changelog compare links for v0.6.0.
