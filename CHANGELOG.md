# Changelog

All notable changes to fleetorch are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.0] — 2026-05-29

A large release — 12 features across three areas: runtime/state, policy/orchestration, and TUI/UX.

### Added — Runtime / state
- **Orphan recovery** — `list` and `prune --recover-orphans` scan the socket dir, cross-reference live PIDs, and restore lost task entries. If `state.json` is deleted or corrupted while workers are running, you no longer lose visibility into them.
- **CPU-time liveness** — replaces the log-mtime "idle after 3min" heuristic with real CPU usage probes (`/proc/<pid>/stat` on Unix, `GetProcessTimes` on Windows). A long-running silent agent (e.g. Claude headless burning tokens without flushing stdout) now correctly shows as `active` instead of `idle`.

### Added — Policy / orchestration
- **Ledger-as-policy** — new `[policy]` config section enforces caps at spawn time: `max_concurrent_total`, `max_concurrent_per_agent`, `max_spend_usd_per_hour`, `max_spend_usd_per_day`. Bypass with `--force`. New `fleetorch policy show` reports caps + current usage.
- **`--pipe-stdout-to <task-id>`** — IPC mesh. New spawn flag mirrors the task's PTY stdout into another running task's control socket, letting agents form pipelines.
- **Team presets** — `fleetorch preset list` / `fleetorch preset run <name>` spawn coordinated agent topologies from named TOML blueprints. Three seeded: `feature-squad` (codex builds, sonnet reviews, haiku writes docs), `bugfix-swarm` (3 codex agents race on the same bug, sonnet picks the winner), `research-team` (gemini reads long docs, sonnet synthesizes, haiku summarizes). Custom presets live under `<DataDir>/presets/`.

### Added — TUI / UX
- **Interactive spawn form on the final tour slide** — bare `fleetorch` now ends with a huh-based form (agent type → task id → prompt) that calls `spawn` directly. No need to remember the CLI flags on first run.
- **Floating kill-confirm modal** — pressing `K` in dash now opens a centered confirmation dialog (`y/N`) with task id, agent, age, and budget burned, instead of killing silently.
- **Command palette (`Ctrl-K`)** — fuzzy finder over task IDs. Type to filter, enter to jump selection, esc to cancel. Modals are mutually exclusive.
- **Fold reasoning blocks** (`f` key) — log viewport collapses `<thinking>` blocks, `tool_use:` lines, and raw JSON tool-call payloads into a single `[+ N hidden reasoning lines]` marker. Toggle on/off.
- **Live pulse indicator** — animated dot (`●◐○◑`) next to each task in dash (500ms tick) and a static colored dot in `fleetorch list`. Color reflects liveness; tasks with silent stdout but live CPU show a ` burning` label.
- **Burn-rate sparklines** — 12-cell `▁–█` sparkline alongside the budget bar in dash showing recent burn rate per task.
- **Split-pane git diff viewer** (`d` key) — toggles a third pane showing `git diff --stat HEAD` + first 200 lines of `git diff HEAD` for the focused task's worktree. Color-coded, 2s cache, `Tab` cycles through all three panes.

## [0.5.0] — 2026-05-29

### Added
- **Interactive intro tour on bare `fleetorch` invocation.** Running `fleetorch` with no arguments on a TTY now launches a 4-slide bubbletea tour (welcome → architecture → core commands → pro tips) instead of dumping help text. Non-TTY invocation (pipes, CI) still falls through to `cmd.Help()` so scripts and `fleetorch | grep` keep working. Navigate with arrows / `h`/`l` / space, exit with `q`/`esc`/`ctrl+c`.
- **Dashboard uplift.** `fleetorch dash` is now a real multi-pane TUI:
  - Hex color palette (`#7D53DE` accent, Dracula-inspired) replaces ad-hoc ANSI 256 integers.
  - Rounded borders with focus highlighting — `Tab` cycles panes; active border glows in accent, inactive dims.
  - Scrollable log viewport — `j`/`k` and `g`/`G` when the log pane has focus; position indicator (`[ TOP ]` / `[ N% ]` / `[ BOT ]`) pinned to the log frame.
  - Budget progress bars (`█▌░` blocks) with green / yellow / red thresholds, in both the dash task list and the `fleetorch list` table.
  - Sticky context-aware footer — two lines: pane name + task count on top, keymap that adapts to the focused pane on the bottom.

## [0.4.8] — 2026-05-29

### Fixed
- `fleetorch agent add` now installs custom agent TOMLs as `<name>.toml` using the parsed `name` field, so `agent remove <name>` and `agent edit <name>` can find agents added from differently named source files.
- `$VISUAL` / `$EDITOR` values with flags or paths containing spaces now launch correctly for `fleetorch agent edit` and `fleetorch config edit`. Handles unquoted Windows paths ending in `.exe`/`.cmd`/`.bat`/`.com` plus standard shell-style quoted arguments. A shared `editorCommand()` helper deduplicates the two call sites.
- `fleetorch logs <id> --err` now prints `(no worker errors recorded for this task)` when the worker error sidecar exists but is empty after a clean run — previously this silently printed nothing, leaving the user unsure whether the task succeeded or the command failed.

## [0.4.7] — 2026-05-28

### Fixed
- **Windows `.cmd` / `.bat` shims now launch correctly even when the user-profile path contains spaces.** npm-installed CLIs (`codex`, `gemini`, `claude`, `agy`) land at `%APPDATA%\npm\<name>.cmd` — a tiny batch wrapper. Direct invocation via `CreateProcess` built an unquoted command line like `cmd.exe /C C:\Users\MAYANK SAHU\…\codex.cmd …`, which `cmd.exe` parsed up to the first space and reported `'C:\Users\MAYANK' is not recognized as an internal or external command`. fleetorch now detects `.cmd` / `.bat` extensions in the resolved command and explicitly prepends `cmd.exe /C` so Go's arg-quoter wraps the shim path in quotes. Unix unchanged (`maybeWrapShim` is a no-op).
- Diagnosed and reported by Codex in the v0.4.5 / v0.4.6 Windows findings runs. Closes the last open issue from those reports.

### Internal
- `internal/supervisor/shim_unix.go` + `shim_windows.go` — platform-split helper. `shim_test.go` cross-platform smoke tests.

## [0.4.6] — 2026-05-28

### Added
- **Sixth seeded agent type: `agy`** — Google Antigravity CLI (`agy --print` for one-shot tasks). Marked `streams_freely = false` because `agy --print` may buffer output.
- `doctor` now probes `agy` on PATH and warns if the `agy` TOML is installed but the CLI isn't.
- README "Agent types" table now lists 6 defaults; "Known quirks" section adds an `agy` bullet.

### Contributed
- PR #2 by Codex (running on Windows via antigravity). Includes a 146-line v0.4.5 Windows findings report confirming HF-1 fix, AF_UNIX bidirectional attach, prune flow, and three-concurrent-loop survival.

## [0.4.5] — 2026-05-28

Docs-only catch-up. No binary changes from v0.4.4.

### Docs
- README CLI reference reorganised into three groups (Core lifecycle, Inspect, Manage) and dedupes the duplicated `list` row that snuck in during v0.4.4. Adds `completion` row with a link to the shell-completion section.
- README "Current version" bumped to v0.4.5 (was stuck at v0.4.1 since the v0.4.1 PR).
- TESTING.md adds `fleetorch doctor` as the recommended Section 0 pre-flight ("paste the output verbatim at the top of your findings file"); adds §3m (doctor + list --json), §3n (logs --err), §3o (prune) covering the v0.4.3 and v0.4.4 additions; updates §7 failure-mode items to reflect the v0.4.0 behavior changes (kill no-op, dash refuses non-TTY) and adds two new items for silent-worker-failure diagnosis and post-run cleanup.
- docs/migration-from-orcha.md command-mapping table expanded with monitor, merge-resolve, prune, list --json, doctor, upgrade, completion. Adds a "New capabilities not present in bash orcha" section.

## [0.4.4] — 2026-05-28

### Added
- **`fleetorch doctor`** — one-shot environment report. Prints fleetorch version, Go version + build target, OS / arch, resolved paths (including whether `$FLEETORCH_HOME` is set), AF_UNIX availability, TTY size, dependency status (git + codex + gemini + claude with discovered version strings), installed agent count, task / worktree / log statistics, and any obvious warnings. Built specifically for "paste into a bug report" — no judgment required, just `fleetorch doctor --json | gh issue ...`.
- **`fleetorch list --json`** — emits a structured array (with `id`, `agent`, `status`, `live_status`, `age_seconds`, `pid`, `budget_usd`, `worktree`, `log`, `socket`, etc.) for scripting. The default table output is unchanged.
- **`fleetorch doctor --json`** — same structured format for the doctor report.

## [0.4.3] — 2026-05-28

### Added
- **`fleetorch prune`** — garbage-collect finished tasks. Removes state rows, worktrees, control sockets, and worker error logs. Sweeps orphan `.sock` files left by crashed workers. Supports `--dry-run`, `--older-than DUR`, `--keep-worktrees`, `--keep-sockets`, `--keep-errors`, `--include-running` (the last for cleaning up after a real crash). Reports per-task disk usage and total reclaimable size.
- **Worker error sidecar** — every detached `fleetorch worker` now writes startup errors to `<DataDir>/errors/<task-id>.err`, regardless of `FLEETORCH_DEBUG`. Previously, if the worker failed before registering state (bad agent command, log-file permission, PTY allocation), the user got a happy `spawned: X` message but `list` showed nothing and there was no diagnostic. Now: `fleetorch logs <id> --err` surfaces the captured error.
- **`fleetorch logs --err`** — print the worker-side error sidecar instead of the agent log.
- **README "Shell completion" section** — documents `fleetorch completion bash|zsh|fish|powershell`, including the PowerShell `Out-File -Append $PROFILE` recipe.

### Changed
- **`install.ps1` is now ~10× faster on PowerShell 5.1.** Sets `$ProgressPreference = 'SilentlyContinue'` around the `Invoke-WebRequest` calls (PS5.1's progress UI is notoriously expensive). Restored before exit so the user's session preference isn't leaked.
- **`install.ps1` prints a completion hint** at the end — copy-paste line to install PowerShell tab completion to `$PROFILE`.

## [0.4.2] — 2026-05-28

### Fixed
- **`$EDITOR` fallback now Windows-aware.** `fleetorch config edit` and `fleetorch agent edit` previously defaulted to `vi` on every platform, which fails on stock Windows where `vi` isn't on PATH. The new `resolveEditor()` helper checks `$VISUAL` first (POSIX convention), then `$EDITOR`, then defaults to `notepad` on Windows and `vi` elsewhere.
- **`list` no longer mangles paths on Windows.** `shortPath()` was abbreviating `C:\Users\…` to `~/AppData\Local\fleetorch\…` — mixed `~` (not understood by `cmd.exe`) and backslashes. Windows now shows the literal path; Unix path abbreviation is unchanged.
- **`agent list` truncation uses ASCII `...`** instead of the U+2026 Unicode ellipsis. Legacy Windows consoles without an active UTF-8 codepage rendered the ellipsis as `?`; Win11 Terminal handles it but the safer character costs nothing.

## [0.4.1] — 2026-05-27

### Fixed
- **HF-1: Windows long-lived agents reported dead while still running.** `list`, `dash`, `attach --follow`, and `kill` used Unix `Signal(0)` process probing on Windows, which treats live processes as unreachable. Windows now queries `GetExitCodeProcess`, and `kill` uses Windows process termination semantics.
- **Windows kill status ordering.** A user-requested kill no longer races the detached worker's exit update and appears as `failed`; it is persisted as `dead` after the worker observes termination.

### Changed
- `FLEETORCH_DEBUG=1` now preserves detached-worker lifecycle logs in `debug-<task-id>.log`, including child exit information, so Windows PTY investigations are observable after the parent CLI returns.

## [0.4.0] — 2026-05-27

### Fixed
- **Windows bare-command resolution.** Shipped agent TOMLs with `command = "powershell"` / `command = "codex"` etc. were being resolved as paths relative to the worktree instead of looked up on `%PATH%`. Supervisor now `exec.LookPath`s the command name before handing to go-pty. Reported by two independent Windows testers.
- **`kill` on already-exited task is now a no-op.** Previously flipped `done` to `dead` in the display. Now prints a friendly "already exited" message and returns success without touching state.
- **`dash` refuses non-TTY stdout.** Previously emitted raw ANSI alternate-screen sequences when piped or redirected. Now prints a clear message pointing at `--plain`.

### Changed
- Tempered the README's "Windows first-class" framing. Honest about the two known Windows issues still being tracked (long-lived agent state registration, v0.3.0/v0.3.1 self-upgrade lock).
- Reconciled `docs/agent-types.md` worked examples with the actual shipped builtin TOMLs (separate commit by docs agent).
- Added defensive logging behind `FLEETORCH_DEBUG=1` so the next Windows tester can pinpoint where long-lived spawn stalls.

### Docs
- `TESTING.md`: bumped version refs to v0.4.0; added windows_arm64 to the asset table; corrected expected `config show` path count (8, not 7); replaced speculative Windows-upgrade caveat with the confirmed-from-the-field reality.

### Known issues carried into next release
- Long-running Windows agent state.json / socket registration may still fail on some Windows configurations. Under investigation; logging added to help next tester localize the stall.
- Real codex/gemini/claude end-to-end test on Windows is still blocked by the registration issue.

## [0.3.3] — 2026-05-27

### Added
- `TESTING.md` at the repo root: a 600-line end-to-end test plan with platform-specific install paths (Linux/macOS/Windows), a PowerShell appendix, and a findings-report template. Designed for both human testers and autonomous agents (Claude / Codex / Gemini) to follow top-to-bottom.

### Docs
- README: new "Known quirks of the wrapped agents" section covering codex's no-commit habit, gemini's cwd sandbox, and claude headless stdout buffering. These were previously only in the agent TOML notes column — now they're discoverable from the front page.
- README: new "Testing" section linking to TESTING.md.

## [0.3.2] — 2026-05-27

### Added
- **`scripts/install.ps1`** — first-class PowerShell installer for Windows. One-liner: `irm https://raw.githubusercontent.com/msnotfound/fleetorch/main/scripts/install.ps1 | iex`. Detects arch (x86_64 / arm64), fetches the latest release, sha256-verifies the asset against the published checksums, extracts to `%LOCALAPPDATA%\Programs\fleetorch\`, and adds it to the user `PATH`.
- **Windows arm64 build.** GoReleaser now produces `fleetorch_X.Y.Z_windows_arm64.zip` alongside x86_64.

### Fixed
- **`fleetorch upgrade` on Windows** no longer fails when the running `.exe` is locked. On rename failure, the upgrader moves the current binary to `fleetorch.exe.old` before sliding the new one into place, and best-effort sweeps `.old` files on subsequent upgrades. Unix behavior unchanged (direct atomic rename).

### Docs
- README rewritten with Windows-parity install instructions: PowerShell one-liner, manual download with `Invoke-WebRequest` + `Get-FileHash` checksum verification, `Expand-Archive` extract, user-PATH setup. No more "Windows is beta" framing — Windows is a first-class target.
- New "Where things live on disk" table covering Linux / macOS / Windows defaults plus the `FLEETORCH_HOME` override.
- CLI reference now lists every shipped command including `ledger`, `merge-resolve`, `upgrade`, `monitor`, and `agent edit`.
- `list` output sample corrected to match the actual columns (`TASK-ID AGENT STATUS AGE BUDGET WORKTREE`).
- Marketplace note updated: now scheduled for v0.4 (was incorrectly listed for v0.3 in earlier drafts).

### Added
- Windows SIGWINCH polling: on Windows (which has no SIGWINCH signal), `attach` now polls the terminal size every 250ms and emits a resize frame only when it changes. Unix continues to use real SIGWINCH.

### Verified
- `fleetorch upgrade` round-trip smoke-tested against the live v0.3.0 release (download → sha256 verify → atomic swap → version check).

## [0.3.0] — 2026-05-27

### Added
- `fleetorch monitor` — foreground narrator that polls fleet state every 60s and spawns a claude-haiku summary of stuck/errored tasks.
- `fleetorch upgrade` — self-updater that fetches the latest GitHub Release, verifies the checksum, and atomically replaces the running binary.
- `fleetorch merge-resolve <file>` — port of the bash-era `auto_keep_both.py`. Replaces each git conflict block in a file with the concatenation of both sides. Useful after merging waves of parallel agent branches.
- `fleetorch agent edit <name>` — opens an installed agent TOML in `$EDITOR`.
- `fleetorch ledger` — prints cumulative spawn counts per agent type.
- `dash`: capital `K` kills the selected task (with confirmation footer).
- `list` and `dash` surface each task's `BudgetUSD` ceiling.
- `attach` propagates terminal resize (SIGWINCH) to the agent's PTY via a small framed protocol on the socket.

### Changed
- Socket wire format is now framed (`[type:1][len:2BE][payload]`) to carry both data and control messages (resize). Backwards compatible at the CLI level — only matters if you talked to the v0.2 socket with `nc`.

### Docs
- Windows attach caveat: requires Win10 1803+ for AF_UNIX; older Windows falls back to `--follow`.

## [0.2.0] — 2026-05-27

### Added
- Bidirectional PTY attach via per-task Unix socket. `fleetorch attach <id>` puts your terminal in raw mode and proxies stdio. Detach with `Ctrl-] q`. Multiple clients can attach simultaneously.
- `dash` is now an interactive bubbletea TUI: task list pane, live log tail pane, `j/k` navigation, status colors, 1s auto-refresh. `--plain` falls back to the prior auto-refresh table.
- `fleetorch config show` now includes `socket_dir`.

## [0.1.0] — 2026-05-27

### Added
- Initial public release.
- Single Go binary replacing the bash/tmux orcha harness.
- Cross-platform: Linux, macOS, Windows (beta).
- Commands: `spawn`, `list`, `watch`, `attach` (read-only), `kill`, `dash` (plain), `logs`, `agent list|add|remove`, `config show|edit`, `version`.
- TOML plugin model for agent types, with 5 defaults seeded on first run (codex, gemini, claude-haiku, claude-sonnet, claude-opus).
- XDG-compliant paths with `FLEETORCH_HOME` override.
- GoReleaser pipeline → GitHub Releases on every tag push.
- `curl|sh` installer at `scripts/install.sh`.

[Unreleased]: https://github.com/msnotfound/fleetorch/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/msnotfound/fleetorch/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/msnotfound/fleetorch/compare/v0.4.8...v0.5.0
[0.4.8]: https://github.com/msnotfound/fleetorch/releases/tag/v0.4.8
[0.4.7]: https://github.com/msnotfound/fleetorch/releases/tag/v0.4.7
[0.4.6]: https://github.com/msnotfound/fleetorch/releases/tag/v0.4.6
[0.4.5]: https://github.com/msnotfound/fleetorch/releases/tag/v0.4.5
[0.4.4]: https://github.com/msnotfound/fleetorch/releases/tag/v0.4.4
[0.4.3]: https://github.com/msnotfound/fleetorch/releases/tag/v0.4.3
[0.4.2]: https://github.com/msnotfound/fleetorch/releases/tag/v0.4.2
[0.4.1]: https://github.com/msnotfound/fleetorch/releases/tag/v0.4.1
[0.4.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.4.0
[0.3.3]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.3
[0.3.2]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.2
[0.3.1]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.1
[0.3.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.0
[0.2.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.2.0
[0.1.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.1.0
