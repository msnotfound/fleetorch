# Agent Types Reference

`fleetorch` uses a plugin-based model for agent types. Each agent type is defined by a TOML file located in your configuration directory (e.g., `~/.config/fleetorch/agents/`).

## TOML Schema

Every agent-type TOML file supports the following fields:

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | string | The unique identifier for the agent type (e.g., "claude-sonnet"). |
| `command` | string | The executable command to run (must be on your PATH). |
| `args` | array | List of arguments to pass to the command. |
| `prompt_arg` | string | The placeholder (e.g., `{prompt}`) used in `args` that will be replaced with the actual user prompt at spawn time. |
| `default_budget_usd` | float | The default maximum budget for a task (if supported by the agent). |
| `default_turns` | int | The default maximum number of turns for a task. |
| `sandbox` | string | The isolation model: `worktree` (default) or `none`. |
| `streams_freely` | bool | Set to `false` if the agent buffers stdout (like headless Claude), prompting `fleetorch` to show a "working silently" indicator. |
| `notes` | string | Optional helpful context about the agent's behavior or quirks. |

---

## Default Agent Types

Below are the configurations for the five default agent types included with `fleetorch`.

### 1. Codex (`codex.toml`)
Optimized for mechanical work and bulk changes using OpenAI's models.

```toml
name = "codex"
command = "codex"
args = ["-p", "{prompt}", "--sandbox", "workspace-write", "--skip-git-repo-check"]
prompt_arg = "{prompt}"
default_budget_usd = 0.0 # Free if you have credits
default_turns = 100
sandbox = "worktree"
streams_freely = true
notes = "Mechanical CRUD, bulk grep/sed, tests, boilerplate."
```

### 2. Gemini (`gemini.toml`)
Best for long-document analysis and codebase-wide reading with a 1M token context.

```toml
name = "gemini"
command = "gemini"
args = ["yolo", "{prompt}"]
prompt_arg = "{prompt}"
default_budget_usd = 0.0 # Free tier eligible
default_turns = 50
sandbox = "worktree"
streams_freely = true
notes = "Deep codebase analysis. Pre-stage external files if necessary."
```

### 3. Claude Haiku (`claude-haiku.toml`)
Quick and inexpensive for small, self-contained tasks.

```toml
name = "claude-haiku"
command = "claude"
args = ["-p", "--model", "haiku", "--verbose", "--allowedTools", "Read,Edit,Write,Bash", "--prompt", "{prompt}"]
prompt_arg = "{prompt}"
default_budget_usd = 0.50
default_turns = 50
sandbox = "worktree"
streams_freely = false
notes = "Short structured tasks. Buffers stdout — trust filesystem activity."
```

### 4. Claude Sonnet (`claude-sonnet.toml`)
The balanced default for architectural work and complex refactors.

```toml
name = "claude-sonnet"
command = "claude"
args = ["-p", "--model", "sonnet", "--verbose", "--allowedTools", "Read,Edit,Write,Bash", "--prompt", "{prompt}"]
prompt_arg = "{prompt}"
default_budget_usd = 2.00
default_turns = 150
sandbox = "worktree"
streams_freely = false
notes = "Architectural work and design synthesis. Buffers stdout."
```

### 5. Claude Opus (`claude-opus.toml`)
High-fidelity reasoning for novel or extremely difficult research problems.

```toml
name = "claude-opus"
command = "claude"
args = ["-p", "--model", "opus", "--verbose", "--allowedTools", "Read,Edit,Write,Bash", "--prompt", "{prompt}"]
prompt_arg = "{prompt}"
default_budget_usd = 5.00
default_turns = 200
sandbox = "worktree"
streams_freely = false
notes = "Novel research only. High cost per turn."
```

---

## Tips for Custom Agents

- **Placeholders:** Use `{prompt}` in the `args` array to indicate where the user's task description should be injected.
- **Pathing:** Ensure the `command` is available in your system's PATH. Use absolute paths if the executable is in a non-standard location.
- **Streaming:** If your custom agent doesn't print output immediately (e.g., it writes to a file and then dumps everything at the end), set `streams_freely = false` to give better feedback in the `fleetorch` dashboard.
