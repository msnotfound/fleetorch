# fleetorch

**A fleet of orchestrated AI coding agents — in one binary.**

`fleetorch` is a cross-platform CLI for spawning, managing, and attaching to multiple AI coding agents running in parallel. It handles the orchestration grunt-work: isolated git worktrees per agent, PTY-based execution with live multi-client attach, cross-platform terminal control, and a unified dashboard to watch the fleet.

**Linux, macOS, and Windows.** Single static binary, no runtime dependencies.

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/msnotfound/fleetorch/main/scripts/install.sh | sh
```
```powershell
# Windows
irm https://raw.githubusercontent.com/msnotfound/fleetorch/main/scripts/install.ps1 | iex
```
```bash
fleetorch spawn claude-sonnet refactor-auth "Refactor AuthMiddleware to use JWT" --repo .
fleetorch list
```

---

## Why fleetorch

- **Single static binary.** No `tmux`, `jq`, bash, GNU coreutils, or PowerShell modules required at runtime.
- **Cross-platform.** Linux, macOS, and Windows are fully supported.
- **Plugin model.** Each agent type is a TOML file — drop one in to add a new agent, no recompile.
- **Live attach with multi-client broadcast.** Multiple terminals can attach to the same task and see the same PTY stream. Detach with `Ctrl-] q`.
- **Terminal resize works.** SIGWINCH on Unix; a 250ms size-polling proxy on Windows (which has no SIGWINCH). The agent's PTY follows your terminal in both cases.
- **Cost-routed by default.** Codex / Gemini / Claude variants live side-by-side in the agent table — start cheap, climb only when needed.
- **Self-updating.** `fleetorch upgrade` pulls the latest GitHub Release, sha256-verifies it, and swaps the running binary in place (with a Windows-safe rename-aside fallback).

---

## Install

### Linux / macOS — one-liner

```bash
curl -fsSL https://raw.githubusercontent.com/msnotfound/fleetorch/main/scripts/install.sh | sh
```

Installs to `~/.local/bin` if writable, else `/usr/local/bin`. Override with `FLEETORCH_BIN_DIR=/path/to/dir`.

### Windows — one-liner

In any PowerShell window (built-in PowerShell 5.1 works; PowerShell 7+ is fine):

```powershell
irm https://raw.githubusercontent.com/msnotfound/fleetorch/main/scripts/install.ps1 | iex
```

This installs to `%LOCALAPPDATA%\Programs\fleetorch\` and adds it to your **user** `PATH`. Override the destination with `$env:FLEETORCH_BIN_DIR`. Open a new PowerShell window after install for the PATH change to take effect.

### Manual install (any platform)

Pick the release asset for your OS/arch from [the releases page](https://github.com/msnotfound/fleetorch/releases/latest):

| OS | Architecture | Asset |
|---|---|---|
| Linux | x86_64 | `fleetorch_X.Y.Z_linux_x86_64.tar.gz` |
| Linux | arm64 | `fleetorch_X.Y.Z_linux_arm64.tar.gz` |
| macOS | Intel | `fleetorch_X.Y.Z_macos_x86_64.tar.gz` |
| macOS | Apple Silicon | `fleetorch_X.Y.Z_macos_arm64.tar.gz` |
| Windows | x86_64 | `fleetorch_X.Y.Z_windows_x86_64.zip` |
| Windows | arm64 | `fleetorch_X.Y.Z_windows_arm64.zip` |

**Verify the checksum** (every release ships a `checksums.txt`):

```bash
# Unix
sha256sum -c --ignore-missing checksums.txt
```
```powershell
# Windows PowerShell
$want = (Select-String -Path .\checksums.txt -Pattern 'fleetorch_.*_windows_x86_64\.zip').Line.Split()[0]
$got  = (Get-FileHash .\fleetorch_*_windows_x86_64.zip -Algorithm SHA256).Hash.ToLower()
if ($want -ne $got) { throw "checksum mismatch" } else { 'OK' }
```

Then extract:
- **Unix:** `tar -xzf fleetorch_*.tar.gz` → `./fleetorch` binary, move it onto your `$PATH`.
- **Windows:** `Expand-Archive fleetorch_*.zip -DestinationPath .` → `fleetorch.exe`, copy it somewhere on `%PATH%` (e.g. `%LOCALAPPDATA%\Programs\fleetorch\`).

### From source (any platform)

Requires Go 1.23+.

```bash
go install github.com/msnotfound/fleetorch/cmd/fleetorch@latest
```

### Shell completion (any platform)

fleetorch ships completions for the major shells via cobra. Install once and forget:

```bash
# bash
fleetorch completion bash | sudo tee /etc/bash_completion.d/fleetorch
# zsh
fleetorch completion zsh > "${fpath[1]}/_fleetorch"
# fish
fleetorch completion fish > ~/.config/fish/completions/fleetorch.fish
```
```powershell
# PowerShell — append to your profile (creates $PROFILE if it doesn't exist)
if (-not (Test-Path $PROFILE)) { New-Item -ItemType File -Path $PROFILE -Force | Out-Null }
fleetorch completion powershell | Out-File -Append -Encoding utf8 $PROFILE
. $PROFILE  # reload current session
```

### Self-update (v0.3.0+)

Once you have v0.3.0 or newer installed, just:

```bash
fleetorch upgrade           # already-latest → no-op
fleetorch upgrade --force   # re-download even if already on latest
```

The upgrader fetches the latest release, sha256-verifies the archive against the published `checksums.txt`, and atomically replaces the running binary. On Windows it handles the running-`.exe` lock by moving the old binary aside (`fleetorch.exe.old`) before the swap — the `.old` is cleaned up on the next upgrade.

Users on v0.1.0 / v0.2.0 don't have the `upgrade` command yet — re-run the platform installer above to get to v0.3+.

---

## Quickstart

**Unix:**

```bash
fleetorch spawn claude-sonnet my-task "Refactor the database connection pool in internal/db" --repo .
fleetorch list
fleetorch attach my-task
```

**Windows (PowerShell):**

```powershell
fleetorch spawn claude-sonnet my-task "Refactor the database connection pool in internal/db" --repo .
fleetorch list
fleetorch attach my-task
```

`fleetorch list` will show something like:

```
TASK-ID   AGENT          STATUS  AGE  BUDGET  WORKTREE
my-task   claude-sonnet  active  3s   $2.00   ~/.local/share/fleetorch/worktrees/my-task
```

`fleetorch attach my-task` drops into the agent's live PTY — your keystrokes go to the agent, its output streams back. Detach with `Ctrl-] q`. Multiple terminals can attach to the same task simultaneously and all see the same stream.

If you'd rather just tail the log (read-only): `fleetorch attach my-task --follow` or `fleetorch logs my-task`.

For an interactive overview of the whole fleet: `fleetorch dash` (bubbletea TUI; `j/k` to navigate, capital `K` to kill the selected task, `r` to refresh, `q` to quit). `fleetorch dash --plain` falls back to a simple auto-refreshing table for SSH/dumb terminals.

---

## Where things live on disk

fleetorch follows OS conventions — XDG on Linux, `~/Library` on macOS, `%APPDATA%` / `%LOCALAPPDATA%` on Windows. All seven paths can be overridden to a single directory by setting **`FLEETORCH_HOME`** — useful for testing, portable installs, or sandboxing.

| Concept     | Linux                                         | macOS                                                  | Windows                                  |
|-------------|-----------------------------------------------|--------------------------------------------------------|------------------------------------------|
| Config dir  | `~/.config/fleetorch/`                        | `~/Library/Application Support/fleetorch/`             | `%APPDATA%\fleetorch\`                   |
| Data dir    | `~/.local/share/fleetorch/`                   | `~/Library/Application Support/fleetorch/`             | `%LOCALAPPDATA%\fleetorch\`              |
| Agent TOMLs | `~/.config/fleetorch/agents/`                 | `~/Library/Application Support/fleetorch/agents/`      | `%APPDATA%\fleetorch\agents\`            |
| Worktrees   | `~/.local/share/fleetorch/worktrees/`         | `~/Library/Application Support/fleetorch/worktrees/`   | `%LOCALAPPDATA%\fleetorch\worktrees\`    |
| Logs        | `~/.local/share/fleetorch/logs/`              | `~/Library/Application Support/fleetorch/logs/`        | `%LOCALAPPDATA%\fleetorch\logs\`         |
| Sockets     | `~/.local/share/fleetorch/sockets/`           | `~/Library/Application Support/fleetorch/sockets/`     | `%LOCALAPPDATA%\fleetorch\sockets\`      |
| state.json  | `~/.local/share/fleetorch/state.json`         | `~/Library/Application Support/fleetorch/state.json`   | `%LOCALAPPDATA%\fleetorch\state.json`    |

Run `fleetorch config show` at any time to print the resolved paths for your machine.

---

## Agent types

fleetorch seeds 6 default agent types on first run. Each is a TOML file in your agents dir — edit any of them with `fleetorch agent edit <name>`.

| Agent | Wraps | Default budget | Default turns | When to use |
|---|---|---|---|---|
| `agy` | Google Antigravity CLI | — | — | Antigravity one-shot tasks via `agy --print`; useful when agy has the right local auth/session context. |
| `codex` | OpenAI Codex CLI | — | — | Mechanical refactors, boilerplate, test scaffolding. Free if you have OpenAI credits. |
| `gemini` | Google Gemini CLI | — | — | Long-doc / wide-codebase reads (1M context). Sandboxes to cwd — pre-stage files. |
| `claude-haiku` | `claude -p --model haiku` | $0.50 | 50 | Short structured tasks, finishers. ~$0.30–1 per module. |
| `claude-sonnet` | `claude -p --model sonnet` | $2.00 | 150 | Architectural work, cross-file refactors, design synthesis. ~$5–15 per module. |
| `claude-opus` | `claude -p --model opus` | $5.00 | 200 | Genuinely novel reasoning. Only with explicit authorization — expensive. |

The cheap path is the default. Climb the ladder only when you need to.

### Known quirks of the wrapped agents

These aren't fleetorch bugs — they're behaviors of the underlying CLIs that fleetorch can't paper over. Worth knowing:

- **Antigravity (`agy`) print mode may buffer output.** fleetorch uses `agy --print` for one-shot runs and marks it as not streaming freely.
- **Codex routinely exits 0 without committing.** Files end up staged in the worktree but no commit is created. After `codex` tasks finish, check `git -C <worktree> status` and commit manually if needed. fleetorch's `agent list` flags this in the codex notes column.
- **Gemini sandboxes to the current working directory.** It cannot read files outside the worktree fleetorch puts it in. If you want gemini to consume a file that lives elsewhere, copy it into the worktree before spawn.
- **Claude headless (`claude -p`) buffers stdout.** A `claude-sonnet` agent can be working silently for 10+ minutes before any log lines appear. **Trust filesystem activity over log lines** — `ls <worktree>` and `git -C <worktree> status` will show progress even when the log looks frozen. fleetorch's `list` marks the task `idle` (not `dead`) in this case.

These are all documented in the seeded agent TOMLs (`fleetorch agent list` shows the notes column) and in [TESTING.md](TESTING.md).

---

## How it works

Each `fleetorch spawn` forks a small **`fleetorch worker`** subprocess that owns the agent's PTY for its entire lifetime. The parent CLI returns immediately. The worker:

1. Creates an isolated **git worktree** on a new `agent/<task-id>` branch (or a scratch dir, if no `--repo`).
2. Opens a **cross-platform PTY** via [`go-pty`](https://github.com/aymanbagabas/go-pty) (real PTY on Unix, ConPTY on Windows).
3. Spawns the agent process inside the PTY, tees output to a log file *and* a 4KiB ring buffer for attach replay.
4. Listens on a per-task **Unix-domain socket** (Win10 1803+ supports `AF_UNIX`). `attach` clients connect there, exchange a small framed protocol (`[type:1][len:2BE][payload]`) carrying both data and terminal-resize messages, and proxy stdio bidirectionally.
5. Updates `state.json` (file-locked, atomic rename) on start, status changes, and exit.

```
                          ┌──────────────────┐
fleetorch spawn  ────────►│ fleetorch worker │── git worktree
                          │    (per task)    │── PTY ── agent process
                          └────────┬─────────┘── log file
                                   │
                              UNIX socket
                                   │
fleetorch attach ◄─────────────────┤
fleetorch dash   ◄─────────────────┤  (multi-client)
fleetorch list   ◄── state.json
```

---

## CLI reference

| Command | Description |
|---|---|
**Core lifecycle**

| Command | Description |
|---|---|
| `spawn <type> <id> <prompt>` | Create a worktree and start an agent. `--repo .` to fork from current repo. `--foreground` to skip detach. |
| `list [--json]` | Status table of every tracked task (status, age, budget, worktree). `--json` emits a structured array — pipe to `jq` for scripting. |
| `kill <id> [--purge]` | SIGTERM the task; `--purge` also removes its worktree. No-op on already-exited tasks. |
| `prune [--dry-run] [--older-than DUR] [--keep-worktrees \| --keep-sockets]` | Garbage-collect finished tasks. Removes state rows, worktrees, sockets, and orphan socket files. Use `--dry-run` to preview. |

**Inspect**

| Command | Description |
|---|---|
| `attach <id>` | Drop into the task's live PTY (bidirectional). `--follow` for read-only log tail. Detach with `Ctrl-] q`. |
| `watch <id> [--follow]` | Snapshot or tail logs. `--follow` is identical to `attach --follow`. |
| `logs <id> [--full \| --err]` | Print the log file (last 200 lines by default). `--err` shows the worker-side error sidecar — the place to look when `spawn` "succeeded" but `list` shows nothing. |
| `dash [--plain]` | Interactive bubbletea TUI. `j/k` navigate, `K` kill selected, `r` refresh, `q` quit. `--plain` falls back to an auto-refresh table for SSH/dumb terms. |
| `doctor [--json]` | One-stop diagnostic: fleetorch version, OS, paths, dependency status, agent inventory, state stats, and warnings. Paste into bug reports. |

**Manage**

| Command | Description |
|---|---|
| `agent list \| add \| remove \| edit` | Manage agent-type plugins. `edit <name>` opens the TOML in `$EDITOR`. |
| `config show \| edit` | Print resolved paths or open the config file in `$EDITOR`. |
| `ledger` | Cumulative spawn counts per agent type. |
| `policy show` | Manage spawn policy caps. |
| `preset list \| run` | Run coordinated agent teams from TOML blueprints. |
| `merge-resolve <file>...` | Resolve git conflict blocks by concatenating both sides — port of the bash-era `auto_keep_both.py`. |
| `upgrade [--force]` | Self-update to the latest GitHub release. |
| `monitor [--interval 60s]` | Foreground narrator: polls the fleet and summarizes stuck/failed tasks via `claude-haiku` (~$0.05/hr). |
| `completion bash\|zsh\|fish\|powershell` | Emit a shell completion script. See the [Shell completion](#shell-completion-any-platform) section above. |
| `--version`, `--help` | Standard. Per-command `--help` works too. |

---

## Adding a new agent type

Drop a TOML file into your agents dir (`fleetorch config show` reveals the path):

```toml
# my-custom-agent.toml
name = "my-custom-agent"
command = "custom-agent-cli"
args = ["--model", "turbo"]
prompt_arg = "{prompt}"             # placeholder substituted at spawn time
default_budget_usd = 1.50
default_turns = 100
sandbox = "worktree"                # worktree | none
streams_freely = true               # false = stdout is buffered (e.g. `claude -p`)
notes = "Free-form description shown in agent list."
```

Then `fleetorch agent list` will show it and `fleetorch spawn my-custom-agent ...` will use it. See `docs/agent-types.md` for the full schema.

---

## Project status

**Current version:** v0.6.6

- **Linux / macOS** — fully supported and first-class. All features working as designed.
- **Windows** — builds clean and ships x86_64 and arm64. HF-1 was fixed in v0.4.1: live long-running agents are now identified using Windows process APIs.
  - `upgrade` from v0.3.0/v0.3.1 binaries still fails with "Access is denied" because those releases predate the rename-aside fix.
- **Architectures shipped per release:** linux x86_64+arm64, macOS x86_64+arm64, Windows x86_64+arm64 (six binaries).
- **Terminal resize.** SIGWINCH on Unix; 250 ms polling proxy on Windows. Both propagate to the agent's PTY.
- **Self-update.** `fleetorch upgrade` works on all three OSes (with a rename-aside fallback for Windows's locked-`.exe` case).
- **Dashboard.** Interactive bubbletea TUI; `--plain` for SSH/dumb terms.
- **Custom agents.** Drop a TOML in `<AgentsDir>/`. Remote registry / marketplace is not planned at this time.

### Known caveats

- **Windows — fixes since v0.3.x:**
  - `exec.LookPath` fix means shipped TOMLs (`powershell`, `agy`, `codex`, etc.) now resolve via `%PATH%` as expected.
  - v0.4.1 fixes HF-1: long-running agents were alive with a working socket, but Unix-style PID probing made `list` report `dead` and made `kill` skip them.
  - `fleetorch upgrade` from v0.3.0/v0.3.1 binaries on Windows still fails with "Access is denied" — these versions predate the rename-aside fix. Affected users must re-run the PowerShell installer.
- **Seeded agent TOMLs** are written against the agy / codex / gemini / claude CLI flags as they existed at fleetorch's release. If any of those CLIs change flags upstream, edit the corresponding TOML (`fleetorch agent edit codex` etc.). PRs welcome to keep them current.
- **Antivirus on Windows** may quarantine `fleetorch.exe` until you allow it — unsigned binaries are common to flag. Add an exclusion for `%LOCALAPPDATA%\Programs\fleetorch\` if needed.
- **Windows older than 10 1803** doesn't support `AF_UNIX`. `attach` automatically falls back to `--follow` (read-only). The rest of fleetorch — spawn, list, kill, dash, logs — works unchanged.
- **Windows npm-installed CLIs (`.cmd` shims) work even with spaces in your username** (since v0.4.7). fleetorch detects `.cmd` / `.bat` agent commands and invokes them via `cmd.exe /C "<path>"` so the shim path is properly quoted. Without this, `codex.cmd` / `claude.cmd` / `agy.cmd` under `C:\Users\<name with spaces>\AppData\Roaming\npm\` would fail to launch.

---

## Testing

End-to-end test plan: [TESTING.md](TESTING.md). It walks a human (or a coding agent like Claude / Codex / Gemini) through the full install → spawn → attach → kill flow on Linux, macOS, and Windows, and lists the known weak points to probe specifically. If you find a real bug, please open an issue with the relevant section number.

## Contributing

Issues and PRs welcome. fleetorch grew out of a private bash harness called [`orcha`](docs/migration-from-orcha.md) — this is its public form.

**High-value areas for outside contribution right now:**

- Real-world bug reports on Windows (especially around ConPTY, the AF_UNIX socket on Win11, antivirus interactions).
- Stale-TOML PRs: if your agy / codex / gemini / claude CLI version uses different flags than the seeded defaults, send a one-file patch to `internal/agents/builtin/`.
- A demo GIF for the README hero. The placeholder slot is reserved.
- v0.4 agent marketplace design and implementation.

## License

`fleetorch` is released under the [MIT License](LICENSE).
