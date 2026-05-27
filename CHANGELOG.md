# Changelog

All notable changes to fleetorch are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/msnotfound/fleetorch/compare/v0.3.3...HEAD
[0.3.3]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.3
[0.3.2]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.2
[0.3.1]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.1
[0.3.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.0
[0.2.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.2.0
[0.1.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.1.0
