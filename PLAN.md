# fleetorch v0.1.0 — Ship-Today Plan

## What we're building

A cross-platform single-binary replacement for the bash/tmux **orcha** harness. Same UX (spawn N agents in parallel, each in its own git worktree, attach to their live output), zero external runtime dependencies, works on Linux/macOS/Windows.

**Tagline:** *A fleet of orchestrated AI coding agents — in one binary.*

## Why a Go rewrite

The bash version depends on `tmux`, `jq`, GNU coreutils, and POSIX shell. None of those are available on native Windows. A Go binary using `aymanbagabas/go-pty` (ConPTY on Windows, PTY on Unix) and `charmbracelet/bubbletea` for the TUI gives us:

- **Single static binary** per platform — no runtime deps
- **Native Windows** via ConPTY — no WSL required
- **Same live-attach UX** — we own the PTY, we can re-attach stdin/stdout to it from any `fleetorch attach <id>` call
- **TUI dashboard** that doesn't depend on tmux

## CLI surface

```
fleetorch spawn   <type> <id> "<prompt>"  [--repo .] [--budget N] [--turns N] [--model M]
fleetorch list                            # status table
fleetorch watch   <id> [--follow]         # snapshot / tail of log
fleetorch attach  <id>                    # drop INTO live PTY
fleetorch dash                            # TUI dashboard
fleetorch kill    <id>
fleetorch agent   list|add|remove         # plugin agent types
fleetorch config  edit|show
fleetorch logs    <id> [--full]
fleetorch version
```

## Layout

```
~/projects/fleetorch/
├── cmd/fleetorch/main.go            # cobra entrypoint
├── internal/
│   ├── types/                       # Task, AgentType, Config
│   ├── config/                      # XDG-compliant TOML loader
│   ├── store/                       # JSON state read/write (atomic)
│   ├── agents/                      # plugin loader for agent-type descriptors
│   ├── supervisor/                  # PTY spawn, lifecycle, log piping
│   └── tui/                         # bubbletea dashboard + attach proxy
├── examples/agents/                 # codex.toml, claude-sonnet.toml, gemini.toml, ...
├── docs/                            # README expansion, migration-from-orcha
├── scripts/install.sh               # curl|sh installer
├── .goreleaser.yml                  # cross-platform release
├── .github/workflows/release.yml    # tag → GoReleaser
├── go.mod
├── LICENSE                          # MIT
└── README.md                        # public-facing
```

## Cross-platform paths

| Platform | Config | State | Worktrees | Logs |
|---|---|---|---|---|
| Linux | `~/.config/fleetorch/config.toml` | `~/.local/share/fleetorch/state.json` | `~/.local/share/fleetorch/worktrees/` | `~/.local/share/fleetorch/logs/` |
| macOS | `~/Library/Application Support/fleetorch/config.toml` | `~/Library/Application Support/fleetorch/state.json` | `~/Library/Application Support/fleetorch/worktrees/` | `~/Library/Application Support/fleetorch/logs/` |
| Windows | `%APPDATA%\fleetorch\config.toml` | `%LOCALAPPDATA%\fleetorch\state.json` | `%LOCALAPPDATA%\fleetorch\worktrees\` | `%LOCALAPPDATA%\fleetorch\logs\` |

Use `kirsle/configdir` or roll our own per-OS helper.

## Agent type plugin model

Each agent type is a TOML file in `~/.config/fleetorch/agents/` (auto-seeded from `examples/agents/` on first run):

```toml
# claude-sonnet.toml
name = "claude-sonnet"
command = "claude"
args = ["-p", "--model", "sonnet", "--verbose", "--allowedTools", "Read,Edit,Write,Bash"]
prompt_arg = "{prompt}"             # placeholder replaced at spawn time
default_budget_usd = 2.0
default_turns = 150
sandbox = "worktree"                # worktree | none
streams_freely = false              # claude headless buffers stdout
notes = "Buffers stdout — trust filesystem activity over log lines."
```

Adding a new agent type = adding a TOML file. No code change.

## Phasing (today)

| Phase | Owner | Wall time | Deliverable |
|---|---|---|---|
| 0 — Scaffold | host (me) | 20 min | go.mod + dirs + stub cobra CLI + git init |
| 1 — Shared types | host | 30 min | types/, config/ loader, basic XDG paths |
| 2 — Parallel build | orcha agents | 60 min wall | supervisor, store, agents loader, README, install.sh, GH workflow |
| 3 — TUI + attach | host | 60 min | dash + attach commands working |
| 4 — Integration smoke | host | 30 min | end-to-end spawn→list→watch→kill against codex |
| 5 — Release | host | 30 min | GitHub repo, v0.1.0 tag, GoReleaser binaries, install.sh hits Releases |

**Total estimated wall time:** ~3.5 hours.

## Phase 2 parallel decomposition (orcha agents)

All six are independent — no shared files, no sequential deps. Each agent gets a worktree of fleetorch on a branch `agent/<slice>`.

| Slice | Agent | Scope | Files owned |
|---|---|---|---|
| A. Supervisor | `claude-sonnet` (hard, PTY tricky) | Spawn process in PTY, pipe to log file, track lifecycle, kill cleanly | `internal/supervisor/*.go` |
| B. Store | `codex` (mechanical JSON I/O) | Atomic read/write of state.json, task registry CRUD | `internal/store/*.go` |
| C. Agents loader | `codex` (mechanical TOML parse) | Read agent-type TOMLs, resolve placeholders, validate | `internal/agents/*.go` + `examples/agents/*.toml` |
| D. README + docs | `gemini` (free, big context — reads ORCHA.md + this plan) | Public-facing README, migration guide, agent-type docs | `README.md`, `docs/*.md` |
| E. Install + release config | `codex` (mechanical bash/yaml) | `scripts/install.sh` (detect OS/arch, download from Releases), `.goreleaser.yml` | `scripts/install.sh`, `.goreleaser.yml` |
| F. GH Actions | `codex` (mechanical yaml) | `.github/workflows/release.yml` triggering GoReleaser on tag push | `.github/workflows/*.yml` |

Cost ceiling: ~$8 total ($5 sonnet + $0 codex×3 + $0 gemini + buffer).

## Known risks + mitigations

1. **Windows ConPTY untestable from Linux.** → Ship Windows binary as `beta`, request issue reports. GoReleaser builds it; `go-pty` claims to handle ConPTY.
2. **Bubbletea TUI in 60 min is tight.** → v0.1 TUI = single-pane list with auto-refresh. Multi-pane in v0.2.
3. **Claude headless buffers stdout.** → Document in README, surface as `streams_freely=false` flag per agent type, show a "working silently" indicator in `list`/`dash` if filesystem activity > log activity.
4. **Codex parallel TPM contention.** → Run codex slices B/C/E/F sequentially (3 codex agents, fired one at a time) or cap concurrency at 2.
5. **NTFS confusion.** → Already mitigated; project lives at `~/projects/fleetorch` on ext4.
6. **`go-pty` cross-compilation.** → Verify it doesn't need CGO; if it does, use GoReleaser's cross builders or build natively per platform.

## Out of scope for v0.1.0

- `orch-monitor` equivalent (Haiku narrator agent) — port in v0.2
- ccmanager-style pane in dashboard — v0.2
- Conflict resolution helper (`auto_keep_both.py`) — port to Go later, or stay python in `scripts/`
- Cost ledger tracking — surface in `list` if easy, full report in v0.2
- Plugin marketplace / `fleetorch agent install <url>` — v0.3
