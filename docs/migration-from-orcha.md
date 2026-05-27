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
| `orch-list` | `fleetorch list` |
| `orch-watch <id> [--follow]` | `fleetorch watch <id> [--follow]` |
| `orch-kill <id>` | `fleetorch kill <id>` |
| `orch-logs <id>` | `fleetorch logs <id>` |
| `tmux attach -t orchestra \; select-window -t <id>` | `fleetorch attach <id>` |
| `dash` (alias) | `fleetorch dash` |

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

If you had custom agent logic in `orch-spawn`, you will need to create a corresponding TOML file in the `agents/` configuration directory. See `docs/agent-types.md` for the schema.
