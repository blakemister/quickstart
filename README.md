# Quickstart

**Launch multiple terminal windows across your monitors, each ready for vibe coding with Claude.**

```
quickstart
```

One command → Multiple terminals → Project picker in each → Claude Code ready to go.

---

## The Problem

You have multiple monitors. You want to work on multiple projects simultaneously with Claude Code. Setting up your workspace manually is tedious:

1. Open terminal
2. Navigate to project
3. Run Claude
4. Repeat for each window
5. Manually position windows

## The Solution

Quickstart does all of this with a single command. Configure once, launch instantly.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Your Monitors                                   │
├─────────────────────┬─────────────────────┬─────────────────────────────────┤
│      TV (4 windows) │  Monitor (2 windows)│         Laptop (1 window)       │
│  ┌───────┬───────┐  │  ┌───────┬───────┐  │  ┌─────────────────────────────┐│
│  │Claude │Claude │  │  │Claude │Claude │  │  │         Claude Code         ││
│  │Code   │Code   │  │  │Code   │Code   │  │  │                             ││
│  ├───────┼───────┤  │  │       │       │  │  │    Your main workspace      ││
│  │Claude │Claude │  │  │       │       │  │  │                             ││
│  │Code   │Code   │  │  │       │       │  │  │                             ││
│  └───────┴───────┘  │  └───────┴───────┘  │  └─────────────────────────────┘│
└─────────────────────┴─────────────────────┴─────────────────────────────────┘
```

---

## Quick Start (PowerShell - No Install Required)

### 1. Run the interactive setup:

```powershell
.\scripts\quickstart.ps1 -Init
```

This will ask you:
- Where your projects folder is
- How many terminal windows per monitor

### 2. Or run with parameters:

```powershell
# Specify your projects folder and window layout
.\scripts\quickstart.ps1 -ProjectsDir "C:\dev" -Windows "1,2,4"

# This means: 1 window on monitor 1, 2 on monitor 2, 4 on monitor 3
```

### 3. Optional: Install fzf for better project selection

```powershell
winget install junegunn.fzf
```

Without fzf, you'll get a simple numbered menu. With fzf, you get fuzzy search.

---

## Building from Source (Go)

### Prerequisites

- Go 1.21+
- Windows 10/11
- Windows Terminal

### Build

```bash
go mod tidy
go build -o quickstart.exe .
```

### Install

```bash
# Copy to a directory in your PATH
copy quickstart.exe C:\Users\you\bin\
```

### Usage

```bash
# First time setup
quickstart init

# Launch default profile
quickstart

# Launch specific profile
quickstart dev

# List monitors
quickstart monitors
```

---

## Configuration

Config file location: `~/.quickstart/config.yaml`

```yaml
version: 1
profiles:
  default:
    projectsDir: "C:/Users/you/.1dev"
    postSelectCommand: "claude --dangerously-skip-permissions"
    monitors:
      - name: "1"
        windows: 4
        layout: "grid"
      - name: "2"
        windows: 2
        layout: "vertical"
      - name: "3"
        windows: 1
        layout: "full"

  focus:
    projectsDir: "C:/Users/you/.1dev"
    postSelectCommand: "claude --dangerously-skip-permissions"
    monitors:
      - name: "1"
        windows: 1
        layout: "full"
```

### Layout Options

| Layout | Description |
|--------|-------------|
| `grid` | 2x2, 3x3, etc. based on window count |
| `vertical` | Side-by-side columns |
| `horizontal` | Stacked rows |
| `full` | Single fullscreen window |

---

## How It Works

1. **Monitor Detection**: Uses Windows API (`EnumDisplayMonitors`) to detect all connected monitors and their positions
2. **Window Spawning**: Launches Windows Terminal (`wt.exe`) with specific titles for each window
3. **Window Positioning**: Uses `SetWindowPos` to move each window to its calculated position
4. **Project Selection**: Each terminal runs a picker (fzf or fallback menu) showing your project directories
5. **Claude Launch**: After selection, automatically runs `claude --dangerously-skip-permissions` in that project

---

## Project Structure

```
quickstart/
├── main.go                 # Entry point
├── go.mod                  # Go modules
├── internal/
│   ├── cmd/                # CLI commands (Cobra)
│   │   └── root.go
│   ├── config/             # Configuration handling
│   │   └── config.go
│   ├── monitor/            # Monitor detection (Win32 API)
│   │   └── monitor.go
│   └── window/             # Window management (Win32 API)
│       └── window.go
├── scripts/
│   └── quickstart.ps1      # PowerShell proof-of-concept
├── README.md
├── RESEARCH.md             # Technical research & decisions
└── FUTURE_PLANS.md         # Roadmap & ideas
```

---

## Requirements

- **Windows 10/11**
- **Windows Terminal** (installed by default on Windows 11, or via Microsoft Store)
- **fzf** (optional, for fuzzy project selection)
- **Claude Code** (`claude` CLI)

---

## Troubleshooting

### Windows don't position correctly

The script needs to find windows by their title. If Windows Terminal is slow to open, try increasing the sleep delay in the script.

### fzf not found

Install it: `winget install junegunn.fzf`

Or the script will fall back to a simple numbered menu.

### Monitors not detected

Run `quickstart monitors` (Go version) or check the output at the start of the PowerShell script. The script should show all detected monitors with their positions.

### Wrong monitor order

Monitors are sorted left-to-right by their X coordinate. If your physical arrangement doesn't match, adjust the monitor indices in the config.

---

## Contributing

This is an open-source project for the vibe coding community. Contributions welcome!

1. Fork the repo
2. Create a feature branch
3. Make your changes
4. Submit a PR

See `FUTURE_PLANS.md` for ideas on what to build next.

---

## License

MIT

---

## Acknowledgments

- [Windows Terminal](https://github.com/microsoft/terminal) - The terminal that makes this possible
- [fzf](https://github.com/junegunn/fzf) - The best fuzzy finder
- [Cobra](https://github.com/spf13/cobra) - CLI framework for Go
- [Claude Code](https://claude.ai/claude-code) - The AI assistant we're launching
- The vibe coding community - For the inspiration
