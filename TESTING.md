# fleetorch — end-to-end test plan

You (a human or a coding agent: Claude, Codex, Gemini) are about to install fleetorch from scratch and exercise its full surface. The goal is **not** to confirm the happy path — it's to find bugs. Be skeptical, document everything, and finish with a written report.

The repo: https://github.com/msnotfound/fleetorch
The latest release at time of writing: **v0.3.1**.

---

## How to use this document

1. Work top-to-bottom. Don't skip sections.
2. For every numbered test, write down:
   - the exact command you ran (or modifications you made)
   - the actual output (or a tail of it)
   - PASS / FAIL / WEIRD-BUT-OK
3. If a test fails, **continue testing** — don't stop. Note the failure and move on. We want full coverage, not a clean run.
4. At the end, fill in the "Findings" report template.

Where the doc says `<your-machine>`, substitute appropriately for your OS.

**Cross-platform conventions used in this doc:**

- Unless explicitly marked **Windows-only** or **Unix-only**, examples are shown in bash/zsh syntax. On Windows, use the PowerShell equivalents shown in Section 1d and Section A1 below.
- `$FT` (Unix) / `$env:FT` (PowerShell) — path to the `fleetorch` binary you're testing.
- `~/.local/bin` (Unix) / `$env:LOCALAPPDATA\fleetorch` (Windows) — install destination.
- `$FLEETORCH_HOME` (Unix) / `$env:FLEETORCH_HOME` (PowerShell) — overrides every fleetorch data path.

---

## 0. Pre-flight

Verify your machine has the deps that the agents fleetorch wraps will need:

```bash
which git              # required by spawn --repo
which sh               # smoke-test agents use it
which nc               # optional, for raw-socket smoke
gh --version           # optional, for poking at release info
```

If you intend to test real agents (Section 6), you'll also need at least one of:

```bash
which codex            # OpenAI Codex CLI
which gemini           # Google Gemini CLI (gemini-cli, gemini_cli, etc.)
which claude           # Anthropic Claude CLI (Claude Code or the `claude` binary)
```

If none of these are installed, **skip Section 6** and note it in the findings.

---

## 1. Install fleetorch

Do **every** install path appropriate for your OS, in a clean shell. Verify each independently. If a step fails, document and continue to the next path.

### 1a. `curl | sh` installer — **Unix only**

```bash
mkdir -p /tmp/fo-test-install
export FLEETORCH_BIN_DIR=/tmp/fo-test-install
curl -fsSL https://raw.githubusercontent.com/msnotfound/fleetorch/main/scripts/install.sh | sh
ls -la /tmp/fo-test-install/fleetorch
/tmp/fo-test-install/fleetorch --version
```

**Expected:** binary exists, runs, prints `fleetorch version 0.3.1` (or newer).
**Weak point to probe:** what happens if you run the installer twice in a row? Does it overwrite cleanly? What if `/tmp/fo-test-install` is read-only? What if you set `FLEETORCH_VERSION=v0.2.0` — does it pin?

**On Windows the installer should refuse:** Windows uses `cmd.exe` / PowerShell and has no `sh`. Confirm that running the installer through Git Bash or WSL either (a) detects Windows and points the user at the releases page, or (b) succeeds with WSL semantics. Document which happened.

### 1b. Direct binary download

Pick the asset matching your OS/arch from https://github.com/msnotfound/fleetorch/releases/latest, extract, verify checksum.

**Unix (Linux / macOS):**

```bash
cd /tmp
curl -fsSLO https://github.com/msnotfound/fleetorch/releases/latest/download/checksums.txt
curl -fsSLO https://github.com/msnotfound/fleetorch/releases/latest/download/fleetorch_0.3.1_linux_x86_64.tar.gz   # adjust for macos_x86_64 / macos_arm64 / linux_arm64
sha256sum -c --ignore-missing checksums.txt
tar -xzf fleetorch_0.3.1_linux_x86_64.tar.gz
./fleetorch --version
```

**Windows (PowerShell):**

```powershell
$ver = "0.3.1"
$arch = "x86_64"               # only x86_64 ships for Windows today; arm64 not yet built
$base = "https://github.com/msnotfound/fleetorch/releases/download/v$ver"
$dst  = "$env:USERPROFILE\Downloads\fleetorch"
New-Item -ItemType Directory -Force -Path $dst | Out-Null
Invoke-WebRequest "$base/checksums.txt" -OutFile "$dst\checksums.txt"
Invoke-WebRequest "$base/fleetorch_${ver}_windows_${arch}.zip" -OutFile "$dst\fleetorch.zip"

# Verify checksum (PowerShell 5+ has Get-FileHash)
$want = (Get-Content "$dst\checksums.txt" | Select-String "fleetorch_${ver}_windows_${arch}.zip").Line.Split()[0]
$got  = (Get-FileHash "$dst\fleetorch.zip" -Algorithm SHA256).Hash.ToLower()
if ($want -ne $got) { throw "checksum mismatch: want $want, got $got" }

Expand-Archive "$dst\fleetorch.zip" -DestinationPath $dst -Force
& "$dst\fleetorch.exe" --version
```

To make `fleetorch` available from any shell, add `$dst` to your user PATH:

```powershell
[Environment]::SetEnvironmentVariable("PATH", "$env:PATH;$dst", [EnvironmentVariableTarget]::User)
# Open a new PowerShell window for the change to take effect.
```

**Expected (both):** checksum verifies; binary runs and reports `fleetorch version 0.3.1`.
**Weak point:** confirm the README documents this manual Windows path (the `curl|sh` installer explicitly skips Windows).

### 1c. `go install` — all platforms

Requires Go 1.23+ installed and `$GOPATH/bin` (Unix) or `%GOPATH%\bin` (Windows) on PATH.

**Unix:**

```bash
go install github.com/msnotfound/fleetorch/cmd/fleetorch@latest
which fleetorch
fleetorch --version
```

**Windows (PowerShell):**

```powershell
go install github.com/msnotfound/fleetorch/cmd/fleetorch@latest
Get-Command fleetorch
fleetorch --version
```

**Expected:** builds, lands in `$GOPATH/bin` (or `$env:GOPATH\bin`), runs.

### 1d. `fleetorch upgrade` self-updater (from v0.3.0 or newer)

**Unix:**

```bash
mkdir -p /tmp/fo-upgrade-test && cd /tmp/fo-upgrade-test
curl -fsSL https://github.com/msnotfound/fleetorch/releases/download/v0.3.0/fleetorch_0.3.0_linux_x86_64.tar.gz | tar -xz
chmod +x fleetorch
./fleetorch --version           # expect: 0.3.0
./fleetorch upgrade             # should upgrade to 0.3.1 (or newer)
./fleetorch --version           # expect: 0.3.1
./fleetorch upgrade             # second time should say "already on …"
./fleetorch upgrade --force     # should re-download even though already latest
```

**Windows (PowerShell):**

```powershell
$work = "$env:TEMP\fo-upgrade-test"
New-Item -ItemType Directory -Force -Path $work | Out-Null
Set-Location $work
Invoke-WebRequest "https://github.com/msnotfound/fleetorch/releases/download/v0.3.0/fleetorch_0.3.0_windows_x86_64.zip" -OutFile "old.zip"
Expand-Archive "old.zip" -DestinationPath . -Force
.\fleetorch.exe --version              # expect: 0.3.0
.\fleetorch.exe upgrade                # should upgrade to 0.3.1 (or newer)
.\fleetorch.exe --version              # expect: 0.3.1
.\fleetorch.exe upgrade                # second run: "already on …"
.\fleetorch.exe upgrade --force        # forces re-download
```

**Weak point:** upgrade only exists from v0.3.0 onward; v0.1.0/v0.2.0 users have to re-run `curl|sh` (Unix) or re-download manually (Windows). Verify the README documents this.

**Windows-specific weak point:** on Windows, the running `.exe` is locked by the OS. The upgrade flow does an atomic rename (`fleetorch.exe.new → fleetorch.exe`). Verify this actually works — Windows historically refused to rename over a running executable, but recent Windows builds allow it. If it fails, the symptom will be "replace binary at …: Access is denied". Report it.

---

## 2. First-run defaults

Pick **one** of the binaries from Section 1 and use it for the rest of the document.

**Unix:**

```bash
export FT=/path/to/fleetorch                     # adjust
rm -rf /tmp/fo-home
export FLEETORCH_HOME=/tmp/fo-home

$FT config show
$FT agent list
ls -la /tmp/fo-home/agents/
```

**Windows (PowerShell):**

```powershell
$env:FT = "C:\path\to\fleetorch.exe"             # adjust
$env:FLEETORCH_HOME = "$env:TEMP\fo-home"
Remove-Item -Recurse -Force $env:FLEETORCH_HOME -ErrorAction SilentlyContinue

& $env:FT config show
& $env:FT agent list
Get-ChildItem "$env:FLEETORCH_HOME\agents\"
```

**Expected:**
- `config show` prints seven paths, all rooted at the home you set
- `agent list` shows 5 seeded agents: codex, gemini, claude-haiku, claude-sonnet, claude-opus
- The agents dir contains 5 TOML files
- A `socket_dir` is listed (added in v0.3)

**Weak point (both platforms):** delete the agents dir and re-run `agent list` — does it re-seed correctly? What if you delete just one TOML? Does seeding leave a `.tmp` file behind on Windows if interrupted?

**Windows-specific:** when `FLEETORCH_HOME` is *not* set, the default Windows paths should land under `%LOCALAPPDATA%\fleetorch\` (data) and `%APPDATA%\fleetorch\` (config). Confirm `config show` reports those for a fresh install with no override.

---

## 3. CLI surface — every command, dry exercise

Use a synthetic agent so you don't need real LLM access yet.

**Unix (`shechord`):**

```bash
cat > /tmp/fo-home/agents/shechord.toml <<'EOF'
name = "shechord"
command = "sh"
args = ["-c", "echo READY; while IFS= read -r line; do echo \"echo: $line\"; done"]
EOF
$FT agent list                 # shechord appears
```

**Windows (`pwchord`):**

PowerShell makes a fine echo-loop. Save this TOML — note the `command = "powershell"` and the `$line` inside the script:

```powershell
@'
name = "pwchord"
command = "powershell"
args = ["-NoLogo", "-NoProfile", "-Command", "Write-Host READY; while ($line = Read-Host) { Write-Host \"echo: $line\" }"]
'@ | Set-Content -Path "$env:FLEETORCH_HOME\agents\pwchord.toml"
& $env:FT agent list           # pwchord appears
```

For the rest of Section 3, Unix readers should use `shechord` and Windows readers should substitute `pwchord` for every `shechord`.

### 3a. spawn / list

```bash
$FT spawn shechord t1 "hello-prompt"
sleep 1
$FT list                       # t1 should be 'active', show BudgetUSD column
$FT logs t1                    # should show READY
```

### 3b. attach (bidirectional)

```bash
$FT attach t1
# In the attached session, type: hi<Enter>
# Expect: "echo: hi"
# Detach with: Ctrl-] then q
# Expect: "[detached]" message
```

### 3c. attach --follow (read-only)

```bash
$FT attach t1 --follow
# Should tail the log; type-input should NOT be sent to the agent
# Ctrl-C to exit
```

### 3d. dash (TUI)

```bash
$FT dash
# Verify: task list left, log tail right
# Press: j/k to navigate, g/G top/bottom, r refresh, q quit
# Press K twice to kill the selected task (with confirmation footer)
```

**Weak point:** confirm the K-kill confirmation footer cancels on any other key.

### 3e. dash --plain

```bash
$FT dash --plain    # auto-refresh table, Ctrl-C to exit
```

### 3f. kill

```bash
$FT spawn shechord t2 "x"
sleep 1
$FT kill t2
$FT list           # t2 should be 'dead'
```

### 3g. agent edit / add / remove

```bash
EDITOR=cat $FT agent edit shechord   # prints the file via $EDITOR=cat
echo 'name = "scratch"\ncommand = "true"' > /tmp/scratch.toml
$FT agent add /tmp/scratch.toml
$FT agent list                       # scratch present
$FT agent remove scratch
$FT agent list                       # scratch gone
```

### 3h. ledger

```bash
$FT ledger          # cumulative spawn counts; shechord >= 2 after 3a + 3f
```

### 3i. merge-resolve

```bash
cat > /tmp/conflict.txt <<'EOF'
header
<<<<<<< HEAD
ours-line
=======
theirs-line
>>>>>>> branch
footer
EOF
$FT merge-resolve /tmp/conflict.txt --dry-run
$FT merge-resolve /tmp/conflict.txt
cat /tmp/conflict.txt
```

**Expected:** body becomes `header\nours-line\ntheirs-line\nfooter`. Dry-run does not modify.

### 3j. monitor (dry-run)

```bash
timeout 5 $FT monitor --dry-run --interval 2s
```

**Expected:** prints "polling every 2s" then a status line every 2s with active/stuck/failed counts.

### 3k. config edit

```bash
EDITOR=cat $FT config edit         # prints the (auto-created) config.toml
```

### 3l. version / help

```bash
$FT --version
$FT --help                         # all commands listed
$FT spawn --help                   # flag docs make sense
```

---

## 4. Concurrency — real fleet behavior

This is the most important test. fleetorch's whole reason for being is parallel agents.

```bash
# Spawn 5 in parallel
for i in 1 2 3 4 5; do
  $FT spawn shechord parallel-$i "x" >/dev/null
done

sleep 2
$FT list                  # all 5 active
ls /tmp/fo-home/sockets/  # 5 .sock files
```

In **one terminal**, attach to `parallel-1`. In **another terminal**, also attach to `parallel-1`. In a **third terminal**, attach to `parallel-2`.

- Type `from-term-A` in the first terminal — both terminals attached to `parallel-1` should see `echo: from-term-A`.
- Type `from-term-C` in the third terminal — only the parallel-2 terminal sees it.

**Expected:** broadcast to multiple attached clients works; tasks are isolated from each other.

Kill them all:

```bash
for i in 1 2 3 4 5; do $FT kill parallel-$i; done
```

**Weak point:** what happens to attached terminals when their task is killed externally? Do they exit cleanly with `[task exited]`? Does Ctrl-]q still detach cleanly mid-stream?

---

## 5. PTY + resize stress

```bash
$FT spawn shechord pty-test "x"
$FT attach pty-test
# Once attached, resize your terminal window (drag the corner, or run `resize` in another tab)
# In the attached session, type:  stty size<Enter>
# Expected: the new (rows cols) — not the size you started at
# Type: \x1b[5n           (some terminals respond to status query — not required to test)
# Detach with Ctrl-] q
$FT kill pty-test
```

**Weak point:** on Windows the resize is *polled* every 250ms, not signal-driven — so changes may take up to a quarter second to propagate. On Unix it should be near-instant.

---

## 6. Real agents — the actual test

**Skip this section if you don't have codex/claude/gemini installed and authenticated.**

For each available CLI:

### 6a. codex

```bash
$FT spawn codex real-codex-1 "Print the SHA-256 of the string 'hello'. Output only the hex digest." --repo .
sleep 30
$FT list                                # real-codex-1 should be active or done
$FT logs real-codex-1 | tail -40        # look for the digest
```

**Expected:** codex runs, finishes, output contains `2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824` (or however codex decides to format it).

**Weak point — high-value to test:** the seeded `codex.toml` has these args:
```
args = ["exec", "--sandbox", "workspace-write", "--skip-git-repo-check"]
```
If the codex CLI has changed these flags, fleetorch's spawn will fail. Check `codex --help` against the TOML.

### 6b. gemini

```bash
$FT spawn gemini real-gemini-1 "Write a haiku about parallel agents." --repo .
sleep 30
$FT logs real-gemini-1 | tail -20
```

**Weak point:** seeded `gemini.toml` uses `args = ["--yolo"]`. Confirm this flag still exists. Gemini's sandbox restricts file access to cwd — pre-stage any external context files inside the worktree before spawn.

### 6c. claude

```bash
$FT spawn claude-haiku real-claude-1 "Write a one-line Go program that prints the current Unix timestamp." --repo .
sleep 60
$FT logs real-claude-1 | tail -40
```

**Weak point:** claude with `-p` (`--print`) buffers stdout. The log may stay empty for minutes while the agent works. Check filesystem under the worktree (`ls /tmp/fo-home/worktrees/real-claude-1/`) to confirm activity. Verify the `--max-budget-usd` flag is respected.

### 6d. Mix and match

Spawn one of each (codex + gemini + claude-haiku) at once. Verify `dash` shows all three correctly.

---

## 7. Failure-mode probing

Try to break it. Document each.

1. **Spawn a non-existent agent:** `$FT spawn nonsense t-x "x"` — expect a clear error, not a crash.
2. **Attach to a task that doesn't exist:** `$FT attach ghost` — expect a clean "task not found" error.
3. **Spawn the same task-id twice:** `$FT spawn shechord dup "x"; $FT spawn shechord dup "x"` — expect the second to either suffix with `-2` or error clearly.
4. **Kill a task that's already done:** spawn, wait for exit, then `$FT kill`. Should not error.
5. **Spawn with a non-existent --repo:** `$FT spawn shechord t "x" --repo /no/such/path` — expect a clean error.
6. **Spawn with a bare `@/non/existent/prompt`:** prompt resolution should fall back gracefully.
7. **Delete state.json mid-run:** `rm /tmp/fo-home/state.json` while tasks are running; then `$FT list`. Does it recover or panic?
8. **Two `fleetorch list` running concurrently against the same store:** stress the flock.
9. **Wide terminal vs narrow terminal for `dash`:** does the layout break at 60 cols? 200 cols?
10. **Pipe `dash` output (no TTY):** does it fall back, or print escape codes? It should error or refuse.

---

## 8. Known weak points to probe specifically

These were called out by fleetorch's author as the highest-risk areas. Spend extra time here.

1. **Seeded agent TOMLs may be stale.** The codex/gemini/claude flags were transcribed from an older bash harness. If a CLI has changed flags between when fleetorch shipped and when you're testing, spawn will fail in a way that looks like fleetorch's fault. **Check `codex --help`, `gemini --help`, `claude --help` against the TOMLs in `/tmp/fo-home/agents/` and report mismatches.**

2. **Windows-specific code never tested on Windows hardware.** If you're on Windows (Win10 1803+):
   - Confirm `fleetorch spawn shechord t "x"` writes a `.sock` file under `%LOCALAPPDATA%\fleetorch\sockets\`
   - Confirm `attach` opens that socket and proxies bidirectionally
   - Confirm terminal resize *eventually* (within ~250 ms) propagates to the PTY
   - If on older Windows: confirm `attach` falls back to `--follow` gracefully

3. **NTFS / network filesystems.** Try setting `FLEETORCH_HOME` to a Windows-mounted drive (or NFS, SMB) and see if file locking misbehaves. Atomic rename of `state.json.tmp → state.json` is the brittle point.

4. **Claude `-p` buffering.** When testing 6c, verify that even if the log shows no output, the worktree shows file activity. fleetorch's `list` should mark the task `idle` (not `dead`) when log activity is stale but PID is alive.

5. **Detach robustness.** Inside an attach session, send a `Ctrl-]` *followed by something other than q* (e.g. `Ctrl-]` then `a`). The literal `Ctrl-]a` bytes should reach the agent — fleetorch should NOT eat the Ctrl-]. Then try `Ctrl-]` `Ctrl-]` (two in a row) — second one should pass through.

6. **Frame protocol overflow.** Send a single line longer than 65535 bytes via attach. Should split into multiple frames cleanly.

7. **`fleetorch monitor` with no `claude` on PATH.** Should print a warning, not crash.

8. **`fleetorch upgrade` with no network.** Disconnect, run upgrade. Should fail cleanly with a "download failed" error, not corrupt the running binary. The atomic rename means a failed download should leave the old binary in place.

---

## 9. Documentation sanity

Read these files and confirm they match observed behavior:

- `README.md` — every command listed under "CLI Reference" should actually exist (`agent list|add|remove|edit`, `config`, `ledger`, `merge-resolve`, etc.)
- `docs/agent-types.md` — every field documented should match what `agents.go` actually parses
- `docs/migration-from-orcha.md` — every old → new mapping should be accurate
- `CHANGELOG.md` — v0.1, v0.2, v0.3, v0.3.1 entries should match the GitHub releases

Report any drift.

---

## 10. Findings report — fill this in

Copy this template into a file `findings-<your-name-or-agent>-<date>.md` and complete every section.

```markdown
# fleetorch test findings

- Tester: <human name or agent name: claude / codex / gemini>
- Date: <YYYY-MM-DD>
- Platform: <linux/macos/windows> <arch> <distro/version>
- fleetorch version tested: <output of `fleetorch --version`>

## Summary
- Tests run: X / Y
- Hard failures: <count> (anything that crashed, hung, or produced wrong output)
- Soft failures: <count> (anything weird-but-recovered)
- Doc drift: <count> (README/CHANGELOG vs observed)

## Hard failures
For each:
- Section/test number
- Command run
- Expected
- Actual
- Suspected cause (if you have a guess)

## Soft failures / weirdness

## Stale TOMLs (Section 8.1)
Per agent, list each flag in the seeded TOML vs the current CLI's `--help`. Note any that no longer exist.

## Windows-specific findings (Section 8.2)
- AF_UNIX support: <works / falls back to --follow>
- Resize polling: <works / lag / broken>
- Other:

## Suggested fixes
Concrete patches you'd open as PRs.

## Recommended next-version blockers
What MUST be fixed before v0.4.0 ships.
```

---

## Appendix A — Windows-specific cheat sheet

These are the most commonly-needed translations between the Unix examples in this doc and PowerShell. If you're on Windows and a Unix snippet isn't obvious, look here first.

### A1. Shell-syntax translations

| Unix (bash/zsh)             | Windows (PowerShell)                              |
|-----------------------------|---------------------------------------------------|
| `export FT=/path/to/fleetorch` | `$env:FT = "C:\path\to\fleetorch.exe"`         |
| `$FT spawn …`               | `& $env:FT spawn …`                               |
| `export FLEETORCH_HOME=/tmp/fo-home` | `$env:FLEETORCH_HOME = "$env:TEMP\fo-home"` |
| `mkdir -p /tmp/x`           | `New-Item -ItemType Directory -Force -Path "$env:TEMP\x"` |
| `rm -rf /tmp/x`             | `Remove-Item -Recurse -Force "$env:TEMP\x" -EA SilentlyContinue` |
| `cat file`                  | `Get-Content file`                                |
| `tail -f file`              | `Get-Content file -Wait`                          |
| `curl -fsSL <url> -o file`  | `Invoke-WebRequest <url> -OutFile file`           |
| `sha256sum file`            | `Get-FileHash file -Algorithm SHA256`             |
| `which fleetorch`           | `Get-Command fleetorch`                           |
| `timeout 5 cmd`             | `$j = Start-Job { cmd }; Wait-Job $j -Timeout 5; Stop-Job $j; Receive-Job $j` |
| `printf '\x1dq'`            | `[char]0x1d + 'q'` then pipe into the process     |
| `nc -U /path/to.sock`       | not bundled; install `ncat` from Nmap, or write a small Go/PowerShell client (see Section 4) |

### A2. Where things live on Windows

| Concept           | Default Windows path                                       |
|-------------------|------------------------------------------------------------|
| Config dir        | `%APPDATA%\fleetorch\` (e.g. `C:\Users\<you>\AppData\Roaming\fleetorch\`) |
| Data / state dir  | `%LOCALAPPDATA%\fleetorch\` (e.g. `C:\Users\<you>\AppData\Local\fleetorch\`) |
| Agent TOMLs       | `%APPDATA%\fleetorch\agents\`                              |
| Worktrees         | `%LOCALAPPDATA%\fleetorch\worktrees\`                      |
| Logs              | `%LOCALAPPDATA%\fleetorch\logs\`                           |
| Sockets           | `%LOCALAPPDATA%\fleetorch\sockets\`                        |
| state.json        | `%LOCALAPPDATA%\fleetorch\state.json`                      |

Setting `$env:FLEETORCH_HOME` overrides every one of these to a single directory.

### A3. Windows-only failure modes to watch for

These are things that **only** break on Windows or that have unique Windows symptoms. Probe each:

1. **Locked .exe during `fleetorch upgrade`.** Some Windows builds refuse `MoveFile` over a running executable. If `upgrade` fails with "Access is denied", note it — fleetorch needs a workaround (move-old-aside, then move-new-into-place).
2. **AF_UNIX availability.** `attach` needs Win10 1803+ for Unix-domain sockets. On older Windows, `attach` should fall back to `--follow` automatically. Run `[System.Environment]::OSVersion` to check your build.
3. **Path separators in TOML.** Agent TOMLs use `command = "powershell"` etc. Don't write backslash-quoted paths into TOML — use forward slashes, which Windows accepts in most APIs, or rely on `command` being just an executable name resolved via PATH.
4. **Long path support.** If `FLEETORCH_HOME` ends up >260 chars total (e.g. deep network share), Windows may reject file ops unless long-path support is enabled. Symptoms: `The system cannot find the path specified` for paths that visibly exist.
5. **Network filesystems.** If `FLEETORCH_HOME` is on an SMB or OneDrive-synced folder, `state.json.tmp → state.json` atomic rename may fail silently. Stay on local disk for testing.
6. **Antivirus interference.** Real-time AV may quarantine the `.exe` mid-download or block PTY allocation. If `spawn` produces no log and the worker exits immediately, check Windows Defender quarantine.
7. **PowerShell ExecutionPolicy.** If `command = "powershell"` agents fail with `cannot be loaded because running scripts is disabled`, the agent needs `-ExecutionPolicy Bypass` or the policy needs adjusting. Note which.
8. **Console codepage.** If agent output shows `?` for non-ASCII characters in `dash`, the Windows console is probably on a legacy codepage. Run `chcp 65001` in the same shell before launching `dash` and re-test.
9. **TUI rendering.** `dash` uses ANSI escape sequences. The built-in `conhost.exe` in Windows 10 supports them; older terminals don't. Confirm `dash` looks right in Windows Terminal (recommended) and falls back gracefully (or refuses) in legacy consoles.
10. **Resize polling latency.** Section 5's resize test: on Windows the change propagates within ~250 ms (poller interval), not instantly. Stretch the terminal slowly and confirm the agent's `stty size` (or equivalent) catches up.

---

## Notes for agent testers (Claude / Codex / Gemini)

If you're an autonomous agent reading this:

1. **Do not stop on first failure.** Push through to the end and give a complete picture.
2. **Capture exact command and exit code** in your findings — "it failed" is not actionable.
3. **For interactive tests (attach, dash):** use `timeout 5 fleetorch attach …` with a piped command stream rather than trying to type. Send detach sequence as `printf '\x1dq'`.
4. **Skip 6a/6b/6c if your environment can't actually call those CLIs** — note it in findings and move on. Section 4 (concurrency with shechord) is the next-best stress test.
5. **You may install fleetorch in your worktree** via `go install`, then call `$(go env GOPATH)/bin/fleetorch`. Don't rely on a system-wide install.
6. **Don't trust your own success messages** without a verification step. After every `spawn`, run `list` and confirm the row is there. After every `kill`, run `list` and confirm status moved to `dead`/`failed`/`done`.

Good hunting.
