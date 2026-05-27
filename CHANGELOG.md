# Changelog

All notable changes to fleetorch are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.1] — 2026-05-27

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

[Unreleased]: https://github.com/msnotfound/fleetorch/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.1
[0.3.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.3.0
[0.2.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.2.0
[0.1.0]: https://github.com/msnotfound/fleetorch/releases/tag/v0.1.0
