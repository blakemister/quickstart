# Agent Instructions for qs

This file is for AI coding agents (Claude Code, Codex, etc.) working on or with this project. It is NOT user documentation — see README.md for that.

## Project Overview

`qs` is a Go TUI terminal launcher. Users run `qs`, pick a project folder, pick an AI coding tool, and it launches that tool in the project directory. It also supports multi-monitor window management as an advanced feature.

- **Binary name**: `qs`
- **Module**: `github.com/bcmister/qs`
- **Config version**: 5 (auto-migrates from v2/v3/v4)
- **Current version**: v0.4.0
- **Platform**: Windows only (uses Win32 API)

## Building

```bash
go mod tidy
go build -o qs.exe .
```

Or use the installer which builds, copies to `~/.qs/bin/`, and adds to PATH:

```powershell
.\install.ps1
```

## Running Tests

```bash
go test ./...
```

Tests exist in `internal/config/config_test.go` and `internal/launcher/launcher_test.go` and `internal/tui/picker_test.go`.

## Project Structure

```
main.go                         Entry point, calls cmd.Execute()
internal/
  cmd/
    root.go                     Main command — loads config, runs first-run flow or picker
    setup.go                    `qs setup` — launches setup wizard TUI
    accounts.go                 `qs accounts` — launches account management TUI
    monitors.go                 `qs monitors` — prints detected monitors
    version.go                  `qs version` — prints version string
    dash.go                     `qs dash` — session dashboard; builds the LaunchResolver + PsmuxEngine
    profile.go                  `qs profile add/list` — author profiles
    project.go                  `qs project add/list` — author project bindings
    keys.go                     `qs keys set/list` — author per-profile secrets (masked)
  config/
    config.go                   Config struct, Load/Save, v2/v3/v4 → v5 migration
    accounts.go                 Account struct, DefaultAccounts list, helpers
    keys.go                     AccountKeys map, profile-secret namespace, masking
    profile.go                  Profile + GitIdentity structs, lookups
    service.go                  Service/ServiceStatus/Category/StatusState
    project.go                  Project struct, ServicesForProject, path lookup
    sessionenv.go               ResolveSessionEnv/ResolveProfileEnv firewall (clean env)
    status.go                   ProbeServiceStatus (probes with firewall env)
    authoring.go                AddProfile/AddService/AddProject, SetProfileKey
    config_test.go              Config tests
  session/
    engine.go                   SessionEngine interface, Session/Spec/Event types, LaunchResolver
    psmux.go                    PsmuxEngine — psmux-backed engine, NewPsmuxEngine
    poll.go                     Background poller reconciling cached state with psmux
    swarm.go                    Git Bash / agent-teams shell setup for launched sessions
  launcher/
    launcher.go                 Win32 window spawning + positioning (wt.exe, SetWindowPos)
    launcher_test.go            Launcher tests
  monitor/
    monitor.go                  Win32 monitor detection (EnumDisplayMonitors)
  tui/
    picker.go                   Main TUI — project list with fuzzy filter → account selection → launch
    viewer.go                   Inline file viewer (markdown/code), reused by the dashboard
    dash.go                     Dashboard model: projects | sessions | context, engine wiring
    dash_view.go                Dashboard rendering (columns, status dots, snapshot strip)
    setup.go                    Setup wizard TUI (projects root, monitors, accounts)
    first_run.go                First-run flow (no config exists yet)
    accounts.go                 Account management TUI
    keys.go                     Key bindings
    styles.go                   Colors and lipgloss styles
    picker_test.go              Picker tests
```

## Config File

Location: `~/.qs/config.yaml` (legacy fallback: `~/.cc/config.yaml`)

```yaml
version: 5
projectsRoot: "C:/Users/username/dev"
defaultAccount: claude
lastAccount: claude
accounts:
  - id: claude
    label: Claude Code
    command: claude
    args: ["--dangerously-skip-permissions"]
    icon: "\U0001F7E0"
    enabled: true
monitors:
  - layout: full
    windows:
      - tool: claude
profiles:
  - id: personal
    label: Personal
    gitIdentity:
      name: Jane Dev
      email: jane@example.com
    accountConfigDirs:
      claude: "C:/Users/username/.claude-personal"
services:
  - id: github
    label: GitHub
    category: vcs-identity
    statusCmd: "gh auth status"
    requiresEnv: ["GH_TOKEN"]
    enabled: true
projects:
  - id: my-app
    label: My App
    path: "C:/Users/username/dev/my-app"
    profile: personal
    accounts: ["claude"]
    defaultAccount: claude
    services: ["github"]
```

Key points:
- `projectsRoot` is the directory containing project subdirectories
- `accounts` defines available AI tools — each has `id`, `label`, `command`, `args`, `authCmd`, `installCmd`, `icon`, `enabled`
- `monitors` defines window layout per physical monitor — each has `layout` (full/vertical/horizontal/grid) and a list of `windows` with a `tool` reference
- `profiles` define identity/secret contexts — each has `id`, `label`, optional `gitIdentity`, `accountConfigDirs` (per-tool isolated config dir), `color`, and `expectedEnv`. Profile *secrets* live in `~/.qs/keys.yaml` under `profile:<id>`, never in `config.yaml`
- `services` define probeable external dependencies — each has `id`, `label`, `category`, optional `statusCmd`, `requiresEnv`, `enabled`
- `projects` bind a directory to a `profile`, a set of `accounts`/`services`, and a `defaultAccount`. The dashboard (`qs dash`) and session engine resolve a project's env from its profile
- Config is always saved as version 5; older versions (v2/v3/v4) are migrated on load in memory (v4→v5 is purely additive)
- `DefaultConfigPath()` returns `~/.qs/config.yaml`
- `config.Load("")` tries `~/.qs/config.yaml` then falls back to `~/.cc/config.yaml`

## Key Architectural Patterns

### TUI (Bubble Tea)
All TUI views use the Elm architecture via charmbracelet/bubbletea:
- `Model` struct holds state
- `Init()` returns initial command
- `Update(msg)` handles input, returns updated model + command
- `View()` renders the UI string

The picker has three stages: `stageProject` → `stageCreate` (optional) → `stageAccount`. If only one account is enabled, account selection is skipped.

### Win32 API
Monitor detection and window positioning use direct Win32 syscalls via `syscall.NewLazyDLL`. This is Windows-only. The relevant DLLs are `user32.dll` and `kernel32.dll`.

### Config Migration
`config.Load()` peeks at the `version` field and routes to `migrateV2()`, `migrateV3()`, or `migrateV4()` as needed. v4→v5 is purely additive (the new `profiles`/`services`/`projects` slices stay empty), so it only bumps the version. Migration happens in memory only — the file is only rewritten on explicit `config.Save()`.

### Session engine + env firewall
`qs dash` opens the three-column session dashboard (projects | sessions | context). It builds a `session.LaunchResolver` over `config.ResolveSessionEnv` and constructs a `session.PsmuxEngine` (psmux-backed). The resolver is the **env firewall**: the engine sources every child process environment from it — a clean allowlisted base plus the project's profile layers (secrets, per-tool config dirs, git identity) — and never from `os.Environ()`. Service status dots are probed the same way via `config.ResolveProfileEnv` + `config.ProbeServiceStatus`.

### Authoring commands
- `qs profile add/list` — manage profiles (flags: `--id`, `--label`, `--git-name`, `--git-email`, `--color`, repeatable `--account-dir accountID=path`).
- `qs project add <path>/list` — bind a directory to a profile/accounts/services (flags: `--profile`, `--accounts a,b`, `--services x,y`, `--default-account`); the project ID defaults to the folder name.
- `qs keys set <profileID> <NAME> <VALUE>` / `qs keys list <profileID>` — manage per-profile secrets in `~/.qs/keys.yaml`. `list` masks every value; `set` never echoes the secret.

## Default Accounts

These are the built-in tool definitions (defined in `internal/config/accounts.go`):

| ID | Command | Args | InstallCmd | Enabled |
|----|---------|------|------------|---------|
| `claude` | `claude` | `--dangerously-skip-permissions` | `npm i -g @anthropic-ai/claude-code` | Yes |
| `codex` | `codex` | `--dangerously-bypass-approvals-and-sandbox` | `npm i -g @openai/codex` | Yes |
| `gemini` | `gemini` | `--yolo` | `npm i -g @google/gemini-cli` | Yes |
| `opencode` | `opencode` | (none) | `npm i -g opencode` | Yes |
| `cursor` | `agent` | (none) | (none) | Yes |

Claude-based accounts launch with only `--dangerously-skip-permissions`. Effort is intentionally **not** forced — Claude Code manages its own effort level (via `~/.claude/settings.json effortLevel` or ultracode), so the launcher passes each account's configured `Args` through unchanged (`Account.ResolvedArgs` in `internal/config/accounts.go`) and `DefaultAccountKeys` injects no `CLAUDE_CODE_EFFORT_LEVEL`. To pin effort for a specific account, add an explicit `--effort <level>` to its `Args` in `~/.qs/config.yaml`.

Users can add custom accounts through `qs setup` or `qs accounts`.

## User Setup Checklist (for agents helping users)

When helping a user get `qs` set up on their machine:

1. **Check Go is installed**: `go version` (needs 1.24+)
2. **Check Windows Terminal is available**: `where wt` (should be on PATH)
3. **Build and install**: run `.\install.ps1` from the repo root in PowerShell — this builds, copies to `~/.qs/bin/`, and adds to PATH
4. **First run**: `qs` will prompt the user to set their projects directory. The user can either:
   - Run the full setup wizard (configures projects dir, monitor layout, and accounts)
   - Just set the project path to get going immediately
5. **Verify AI tools are installed**: check which tools the user has (`where claude`, `where codex`, `where gemini`, etc.) and help them disable tools they don't have via `qs accounts` or by editing `~/.qs/config.yaml`
6. **After setup**: `qs` should show the project picker. If the projects directory is empty, the user can create folders from the picker UI

### Common issues
- **`qs` not found after install**: user needs to restart their terminal for PATH changes
- **No projects shown**: `projectsRoot` in config points to wrong directory, or directory has no subdirectories
- **Tool fails to launch**: the tool's CLI is not installed or not on PATH. Check with `where <command>`
- **Config not loading**: check `~/.qs/config.yaml` exists and is valid YAML. If it was manually edited incorrectly, delete it and run `qs setup`

## Development Notes

- The module path is `github.com/bcmister/qs` — do not change this
- Cobra is used for CLI routing; all commands are registered in `cmd/root.go` `init()`
- Styles are centralized in `tui/styles.go` — use the existing style variables, don't create ad-hoc styles
- The picker launches the selected tool via `tea.ExecProcess` which replaces the TUI with the child process
- `config.EnsureDefaults()` fills in missing fields — always call it after loading config
