# Migration from orcha to fleetorch

If you've been using the bash-based `orcha` harness, this guide will help you transition to the Go-based `fleetorch` binary.

## Key Philosophy Changes

- **No more tmux dependency:** `fleetorch` manages its own pseudo-terminals (PTYs). You don't need `tmux` installed, though you can still run `fleetorch` inside it.
- **Single binary:** Replace the collection of `orch-*` scripts with a single `fleetorch` executable.
- **Cross-platform:** `fleetorch` runs natively on Windows, macOS, and Linux.

## Command Mapping

| Old bash command | New `fleetorch` command |
| :--- | :--- |
| `orch-spawn <type> <id> "..."` | `fleetorch spawn <type> <id> "..."` |
| `orch-list` | `fleetorch list` (add `--json` for scripting) |
| `orch-watch <id> [--follow]` | `fleetorch watch <id> [--follow]` |
| `orch-kill <id>` | `fleetorch kill <id>` (no-op on already-finished tasks) |
| `orch-logs <id>` | `fleetorch logs <id>` (add `--err` to see worker-side startup errors) |
| `tmux attach -t orchestra \; select-window -t <id>` | `fleetorch attach <id>` |
| `dash` (alias) | `fleetorch dash` (or `--plain` for SSH/dumb terms) |
| `orch-monitor` | `fleetorch monitor` |
| `auto_keep_both.py <file>` | `fleetorch merge-resolve <file>` |
| *(none — manual cleanup)* | `fleetorch prune` — GC finished tasks, worktrees, sockets, err logs |
| *(none — needed `jq` against state file)* | `fleetorch list --json`, `fleetorch doctor --json` |
| *(none)* | `fleetorch doctor` — one-shot env report for bug reports |
| *(none)* | `fleetorch upgrade` — self-update from GitHub releases |
| *(none)* | `fleetorch completion bash\|zsh\|fish\|powershell` |

## Path Migrations

`fleetorch` follows the XDG Base Directory Specification (and platform-specific equivalents) for storing configuration and state.

| Item | Old `orcha` Path | New `fleetorch` Path (Linux) |
| :--- | :--- | :--- |
| **State JSON** | `~/agents/state/.orchestra-state.json` | `~/.local/share/fleetorch/state.json` |
| **Worktrees** | `~/agents/worktrees/` | `~/.local/share/fleetorch/worktrees/` |
| **Logs** | `~/agents/logs/` | `~/.local/share/fleetorch/logs/` |
| **Agent Types** | `~/agents/bin/` (hardcoded) | `~/.config/fleetorch/agents/*.toml` |

### Platform-specific Paths

| Platform | Config Dir | Data Dir (State/Worktrees/Logs) |
| :--- | :--- | :--- |
| **Linux** | `~/.config/fleetorch/` | `~/.local/share/fleetorch/` |
| **macOS** | `~/Library/Application Support/fleetorch/` | `~/Library/Application Support/fleetorch/` |
| **Windows** | `%APPDATA%\fleetorch\` | `%LOCALAPPDATA%\fleetorch\` |

## State File Format

The state file has been refactored for better performance and type safety in Go. `fleetorch` **cannot** read your old `.orchestra-state.json` file. 

**Recommendation:** Finish your in-flight `orcha` tasks before switching, or manually re-spawn them in `fleetorch`.

## Configuration

Instead of editing bash scripts or environment variables, use the TOML configuration file:

1. Run `fleetorch config show` to see current settings.
2. Edit `~/.config/fleetorch/config.toml` (or platform equivalent) to customize defaults.

## Agent Types (Plugins)

In `orcha`, agent types were defined as logic blocks within bash scripts. In `fleetorch`, they are standalone TOML files.

If you had custom agent logic in `orch-spawn`, you will need to create a corresponding TOML file in the `agents/` configuration directory. See `docs/agent-types.md` for the schema. Use `fleetorch agent edit <name>` to open one in `$EDITOR`.

## New capabilities not present in bash orcha

- **`fleetorch doctor`** — one-shot environment report covering version, OS, paths, dependency presence (git/agy/codex/gemini/claude with versions), agent inventory, state stats, and warnings. Bash orcha required ad-hoc shell-script probing for any of these.
- **`fleetorch prune`** — garbage-collect finished tasks with `--dry-run` preview and granular `--keep-worktrees` / `--keep-sockets` controls. Bash orcha left worktrees on disk and required manual cleanup.
- **Worker-side error sidecar** — `fleetorch logs <id> --err` surfaces startup failures of the detached worker. The bash equivalent silently lost stderr.
- **Self-update** — `fleetorch upgrade` (since v0.3.0) fetches the latest release, sha256-verifies, and atomically swaps the running binary (with Windows-safe rename-aside fallback).
- **`fleetorch list --json` / `doctor --json`** — structured output for scripting, replacing `jq` against the state file.
