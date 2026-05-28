# Agent Types Reference

`fleetorch` uses a plugin-based model for agent types. Each agent type is defined by a TOML file located in your configuration directory (e.g., `~/.config/fleetorch/agents/`).

## TOML Schema

Every agent-type TOML file supports the following fields:

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | string | The unique identifier for the agent type (e.g., "claude-sonnet"). |
| `command` | string | The executable command to run (must be on your PATH). |
| `args` | array | List of arguments to pass to the command. |
| `prompt_arg` | string | A template string (e.g., `{prompt}`) that is appended to the arguments list after replacing `{prompt}` with the user's input. Replacement also occurs for any `{prompt}` found within the `args` array. |
| `default_budget_usd` | float | The default maximum budget for a task (if supported by the agent). |
| `default_turns` | int | The default maximum number of turns for a task. |
| `sandbox` | string | The isolation model: `worktree` (default) or `none`. |
| `streams_freely` | bool | Set to `false` if the agent buffers stdout (like headless Claude), prompting `fleetorch` to show a "working silently" indicator. |
| `notes` | string | Optional helpful context about the agent's behavior or quirks. |

---

## Default Agent Types

Below are the configurations for the six default agent types included with `fleetorch`.

### 1. Antigravity (`agy.toml`)
One-shot tasks through Google's Antigravity CLI.

```toml
name = "agy"
command = "agy"
args = ["--print"]
prompt_arg = "{prompt}"
default_budget_usd = 0
default_turns = 0
sandbox = "worktree"
streams_freely = false
notes = "Google Antigravity CLI. Uses print mode for one-shot tasks; output may be buffered by agy."
```

### 2. Codex (`codex.toml`)
Optimized for mechanical work and bulk changes using OpenAI's models.

```toml
name = "codex"
command = "codex"
args = ["exec", "--sandbox", "workspace-write", "--skip-git-repo-check"]
prompt_arg = "{prompt}"
default_budget_usd = 0
default_turns = 0
sandbox = "worktree"
streams_freely = true
notes = "Free if user has OpenAI credits. Often forgets to commit at end — finalize manually."
```

### 3. Gemini (`gemini.toml`)
Best for long-document analysis and codebase-wide reading with a 1M token context.

```toml
name = "gemini"
command = "gemini"
args = ["--yolo"]
prompt_arg = "{prompt}"
default_budget_usd = 0
default_turns = 0
sandbox = "worktree"
streams_freely = true
notes = "Free tier eligible. 1M context. Sandboxes to cwd — pre-stage files before spawning."
```

### 4. Claude Haiku (`claude-haiku.toml`)
Quick and inexpensive for small, self-contained tasks.

```toml
name = "claude-haiku"
command = "claude"
args = ["-p", "--model", "haiku", "--verbose", "--max-turns", "50", "--allowedTools", "Read,Edit,Write,Bash(git *),Bash(npm *),Bash(pnpm *),Bash(node *),Bash(python *),Bash(pytest *),Bash(go *)"]
prompt_arg = "{prompt}"
default_budget_usd = 0.5
default_turns = 50
sandbox = "worktree"
streams_freely = false
notes = "Short structured tasks, ~$0.30-1 per module. Lower quality than sonnet on math/architecture."
```

### 5. Claude Sonnet (`claude-sonnet.toml`)
The balanced default for architectural work and complex refactors.

```toml
name = "claude-sonnet"
command = "claude"
args = ["-p", "--model", "sonnet", "--verbose", "--max-turns", "150", "--allowedTools", "Read,Edit,Write,Bash(git *),Bash(npm *),Bash(pnpm *),Bash(node *),Bash(python *),Bash(pytest *),Bash(go *)"]
prompt_arg = "{prompt}"
default_budget_usd = 2.0
default_turns = 150
sandbox = "worktree"
streams_freely = false
notes = "Architectural work, cross-file refactors. ~$5-15 per module. Buffers stdout — trust filesystem activity."
```

### 6. Claude Opus (`claude-opus.toml`)
High-fidelity reasoning for novel or extremely difficult research problems.

```toml
name = "claude-opus"
command = "claude"
args = ["-p", "--model", "opus", "--verbose", "--max-turns", "200", "--allowedTools", "Read,Edit,Write,Bash(git *),Bash(npm *),Bash(pnpm *),Bash(node *),Bash(python *),Bash(pytest *),Bash(go *)"]
prompt_arg = "{prompt}"
default_budget_usd = 5.0
default_turns = 200
sandbox = "worktree"
streams_freely = false
notes = "Genuinely novel reasoning. Only with explicit user authorization — expensive."
```

---

## Tips for Custom Agents

- **Placeholders:** Use `{prompt}` in the `args` array to indicate where the user's task description should be injected.
- **Pathing:** Ensure the `command` is available in your system's PATH. Use absolute paths if the executable is in a non-standard location.
- **Streaming:** If your custom agent doesn't print output immediately (e.g., it writes to a file and then dumps everything at the end), set `streams_freely = false` to give better feedback in the `fleetorch` dashboard.
