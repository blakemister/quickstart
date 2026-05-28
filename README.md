# qs

[![CI](https://github.com/bcmister/qs/actions/workflows/ci.yml/badge.svg)](https://github.com/bcmister/qs/actions/workflows/ci.yml)
[![Release](https://github.com/bcmister/qs/actions/workflows/release.yml/badge.svg)](https://github.com/bcmister/qs/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/bcmister/qs)](https://goreportcard.com/report/github.com/bcmister/qs)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Quickly open any project with your AI coding tool of choice.**

```
qs
```

Pick a project, pick a tool, start coding.

---

## What It Does

`qs` is a terminal launcher that gets you into a project with an AI coding agent in two keystrokes. No more `cd`-ing around, no more remembering CLI flags.

1. Run `qs`
2. Fuzzy-search your projects, hit Enter
3. Pick your AI tool (or skip if you only have one enabled)
4. You're coding

It also creates new project folders inline - select "+ create new folder" from the picker.

---

## Install

Requires **Go 1.24+** and **Windows Terminal**.

```powershell
git clone https://github.com/bcmister/qs.git
cd qs
.\install.ps1
```

This builds `qs.exe`, installs it to `~/.qs/bin/`, and adds it to your PATH.

**Using Claude Code?** Run `/onboard` after cloning for a fully guided setup.

Or build manually:

```bash
go build -o qs.exe .
```

---

## Usage

```bash
qs                # Launch project picker
qs dash           # Open the session dashboard (command center)
qs setup          # Run the setup wizard
qs accounts       # Manage AI tool accounts
qs profile        # Manage profiles (identity/secret contexts)
qs project        # Manage project bindings
qs keys           # Manage per-profile secrets
qs monitors       # List detected monitors
qs version        # Print version
```

### First Run

On first launch, `qs` prompts you to either run the full setup wizard or quickly set your projects directory.

### Project Picker

The main TUI lists your project folders with fuzzy search. Type to filter, arrow keys to navigate, Enter to select.

### Account Selection

After picking a project, choose which AI coding tool to launch. If only one tool is enabled, it launches automatically.

### Session Dashboard

`qs dash` opens a three-column command center: **projects** | **sessions** | **context**. Start a persistent session for a project (`s`), attach to it (`Enter`), stop/kill it (`x`/`k`), and watch a read-only snapshot of the focused session — without ever leaving the dashboard. The context column shows the selected project's docs (CLAUDE.md / README.md / `.claude/*`) and bound-service status dots.

Sessions are launched in an isolated environment: a clean allowlisted base plus the project profile's secrets, per-tool config dirs, and git identity. The host shell's secrets never leak into a launched tool.

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | Cycle focus across columns |
| `↑` / `↓` | Navigate / scroll the focused column |
| `s` | Start a session for the selected project |
| `enter` | Attach to the focused session |
| `x` / `k` | Stop / kill the focused session |
| `r` | Refresh projects + sessions |
| `c` | Cycle the context document |
| `q` | Quit |

#### Profiles, projects, and secrets

Bind a directory to a profile and services, then launch isolated sessions from the dashboard:

```bash
qs profile add --id work --label Work --git-name "Jane Dev" --git-email jane@work.com \
  --account-dir claude=C:/Users/you/.claude-work
qs keys set work GH_TOKEN ghp_xxx        # stored in ~/.qs/keys.yaml, masked on list
qs project add C:/Users/you/dev/my-app --profile work --accounts claude --services github
qs profile list && qs project list && qs keys list work
```

---

## Supported Tools

| Tool | Command | Default |
|------|---------|---------|
| Claude Code | `claude --dangerously-skip-permissions` | Enabled |
| OpenAI Codex | `codex --dangerously-bypass-approvals-and-sandbox` | Enabled |
| Gemini CLI | `gemini --yolo` | Enabled |
| OpenCode (z.ai) | `opencode` | Enabled |
| AMA Claude | `claude --dangerously-skip-permissions --effort max` | Enabled |
| Cursor Agent | `agent` | Enabled |

Add custom tools through the setup wizard or `qs accounts`.

---

## Multi-Monitor Mode

If you have multiple monitors and want to vibe code on several projects at once, `qs` can spawn and auto-arrange terminal windows across all your displays.

Configure window counts and layouts per monitor in the setup wizard:

```
┌─────────────────────┬─────────────────────┬───────────────────┐
│   Monitor 1 (2x2)   │   Monitor 2 (split) │  Laptop (full)    │
│ ┌────────┬────────┐ │ ┌────────┬────────┐ │ ┌───────────────┐ │
│ │ Claude │ Codex  │ │ │ Claude │ Gemini │ │ │  Claude Code   │ │
│ ├────────┼────────┤ │ │  Code  │  CLI   │ │ │               │ │
│ │ Claude │ Claude │ │ │        │        │ │ │               │ │
│ │  Code  │  Code  │ │ │        │        │ │ │               │ │
│ └────────┴────────┘ │ └────────┴────────┘ │ └───────────────┘ │
└─────────────────────┴─────────────────────┴───────────────────┘
```

### Layouts

| Layout | Description |
|--------|-------------|
| `full` | Single fullscreen window |
| `vertical` | Side-by-side columns |
| `horizontal` | Stacked rows |
| `grid` | 2x2, 3x3, etc. based on window count |

---

## Configuration

Config file: `~/.qs/config.yaml`

```yaml
version: 5
projectsRoot: "C:/Users/you/dev"
defaultAccount: claude
lastAccount: claude
accounts:
  - id: claude
    label: Claude Code
    command: claude
    args: ["--dangerously-skip-permissions"]
    enabled: true
  - id: codex
    label: OpenAI Codex
    command: codex
    args: ["--dangerously-bypass-approvals-and-sandbox"]
    enabled: true
monitors:
  - layout: full
    windows:
      - tool: claude
profiles:
  - id: work
    label: Work
    gitIdentity:
      name: Jane Dev
      email: jane@work.com
    accountConfigDirs:
      claude: "C:/Users/you/.claude-work"
services:
  - id: github
    label: GitHub
    category: vcs-identity
    statusCmd: "gh auth status"
    enabled: true
projects:
  - id: my-app
    label: My App
    path: "C:/Users/you/dev/my-app"
    profile: work
    accounts: ["claude"]
    defaultAccount: claude
    services: ["github"]
```

`profiles`, `services`, and `projects` power `qs dash` and the env firewall — profiles supply per-context git identity, isolated tool config dirs, and secrets (secret *values* live in `~/.qs/keys.yaml`, never here). Older config files (v2/v3/v4) are migrated to v5 automatically on load; the v4→v5 step is additive, so existing setups keep working unchanged.

The setup wizard (`qs setup`) walks through all of this interactively:

1. **Projects folder** - where your project directories live
2. **Monitor layout** - how many windows per monitor
3. **AI tool accounts** - which tools to enable, add custom ones

---

## Requirements

- **Windows 10/11**
- **Windows Terminal** (default on Windows 11, or install from Microsoft Store)
- **Go 1.24+** (to build from source)
- At least one AI coding tool installed (`claude`, `codex`, `gemini`, etc.)

---

## Built With

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- Win32 API - Monitor detection + window positioning

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `qs` not found after install | Restart your terminal for PATH changes to take effect |
| No projects shown | Check `projectsRoot` in `~/.qs/config.yaml` points to the right directory |
| Tool fails to launch | Verify the tool's CLI is installed: `where claude`, `where codex`, etc. |
| Config won't load | Delete `~/.qs/config.yaml` and run `qs setup` to reconfigure |

---

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before participating.

---

## License

MIT
