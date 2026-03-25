# Quickstart - Research & Technical Documentation

## Project Vision
A CLI tool for developers that launches multiple terminal windows across multiple monitors, each opening in a projects directory with an interactive picker to select a project, then auto-runs Claude Code.

## Research Summary (January 2025)

---

## Tech Stack Recommendation: **Go (Golang)**

### Why Go Over Rust?

| Factor | Go | Rust |
|--------|-----|------|
| Learning curve | Gentle, beginner-friendly | Steep, complex ownership model |
| Compile to single binary | ✅ Yes | ✅ Yes |
| Cross-compilation | Dead simple (`GOOS=windows`) | More complex, needs toolchains |
| Windows API access | Good (lxn/win, w32 libraries) | Excellent (official windows-rs) |
| CLI frameworks | Cobra (industry standard) | Clap (excellent but verbose) |
| Build speed | Very fast | Slower |
| Community CLI tools | Huge (docker, k8s, gh, hugo) | Growing |

**Verdict**: Go is the pragmatic choice for rapid prototyping and community contribution. Rust would be overkill for this project's scope.

### Sources:
- [Go vs Rust CLI Comparison](https://cuchi.me/posts/go-vs-rust)
- [JetBrains: Rust vs Go 2025](https://blog.jetbrains.com/rust/2025/06/12/rust-vs-go/)
- [Go CLI Solutions](https://go.dev/solutions/clis)
- [Slant: Best Languages for CLI Utilities](https://www.slant.co/topics/2469/~best-languages-for-writing-command-line-utilities)

---

## Core Dependencies

### 1. Windows Terminal (`wt.exe`)
Modern terminal with excellent CLI support.

**Key capabilities:**
- `-w <name>` - Target specific window by name
- `-d <path>` - Set starting directory
- `--title <title>` - Set tab title
- `-p <profile>` - Use specific profile
- `new-tab` / `split-pane` - Create tabs and panes
- Position arguments (in preview builds)

**Example:**
```powershell
wt -w quickstart -d "C:\dev\project1" --title "Project 1" cmd /k "fzf-picker.bat"
```

**Sources:**
- [Microsoft: Windows Terminal CLI Arguments](https://learn.microsoft.com/en-us/windows/terminal/command-line-arguments)
- [SS64: WT.exe Reference](https://ss64.com/nt/wt.html)

### 2. Window Positioning (Win32 API)

**Required Windows APIs:**
- `EnumDisplayMonitors` - Get list of all monitors
- `GetMonitorInfo` - Get monitor bounds/resolution
- `FindWindow` / `EnumWindows` - Find window handles
- `SetWindowPos` / `MoveWindow` - Position windows

**Go Libraries:**
- `github.com/lxn/win` - Comprehensive Win32 wrapper
- `github.com/JamesHovious/w32` - Lighter alternative
- `golang.org/x/sys/windows` - Official syscall package

**Sources:**
- [Go Wiki: Calling Windows DLLs](https://go.dev/wiki/WindowsDLLs)
- [lxn/win user32.go](https://github.com/lxn/win/blob/master/user32.go)
- [Enumerating Monitors in Rust (concepts apply)](https://patriksvensson.se/posts/2020/06/enumerating-monitors-in-rust-using-win32-api)

### 3. Interactive Picker (`fzf`)

**fzf** is a portable fuzzy finder that works on Windows.
- No dependencies
- Blazing fast
- Works in PowerShell
- PSFzf module for PowerShell integration

**Alternative:** Build a simple picker in Go using `bubbletea` or `promptui`

**Sources:**
- [fzf GitHub](https://github.com/junegunn/fzf)
- [fzf PowerShell Integration](https://dev.to/kevinnitro/fzf-advanced-integration-in-powershell-53p0)

---

## Architecture Design

```
┌─────────────────────────────────────────────────────────────┐
│                        quickstart CLI                        │
├─────────────────────────────────────────────────────────────┤
│  Config Parser    │  Monitor Detector  │  Window Manager    │
│  (YAML/JSON)      │  (Win32 API)       │  (Win32 API)       │
├─────────────────────────────────────────────────────────────┤
│                    Windows Terminal (wt.exe)                 │
├─────────────────────────────────────────────────────────────┤
│  Terminal 1       │  Terminal 2        │  Terminal N        │
│  ┌─────────────┐  │  ┌─────────────┐   │  ┌─────────────┐   │
│  │ fzf picker  │  │  │ fzf picker  │   │  │ fzf picker  │   │
│  │     ↓       │  │  │     ↓       │   │  │     ↓       │   │
│  │ claude code │  │  │ claude code │   │  │ claude code │   │
│  └─────────────┘  │  └─────────────┘   │  └─────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## Configuration Design

### First-Time Setup Flow

```
$ quickstart init

Welcome to Quickstart! Let's set up your development environment.

? Where are your projects located?
  > C:\Users\<username>\dev

? How many monitors do you have? 3

? Let's name your monitors:
  Monitor 1 (1920x1080 at 0,0): TV
  Monitor 2 (2560x1440 at 1920,0): Main
  Monitor 3 (1920x1080 at 4480,0): Laptop

? How many terminal windows on "TV"? 4
? How many terminal windows on "Main"? 2
? How many terminal windows on "Laptop"? 1

? What command should run after selecting a project?
  > claude --dangerously-skip-permissions

Config saved to ~/.quickstart/config.yaml
```

### Config File (`~/.quickstart/config.yaml`)

```yaml
version: 1
projectsDir: "C:/Users/<username>/dev"
postSelectCommand: "claude --dangerously-skip-permissions"

monitors:
  - name: "TV"
    windows: 4
    layout: "grid"  # 2x2 grid

  - name: "Main"
    windows: 2
    layout: "vertical"  # side by side

  - name: "Laptop"
    windows: 1
    layout: "full"  # single fullscreen
```

---

## Multi-Monitor Coordinate System

```
         Monitor 1 (TV)              Monitor 2 (Main)         Monitor 3 (Laptop)
    ┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
    │                     │    │                     │    │                     │
    │   (0,0)             │    │   (1920,0)          │    │   (4480,0)          │
    │                     │    │                     │    │                     │
    │        1920x1080    │    │        2560x1440    │    │        1920x1080    │
    │                     │    │                     │    │                     │
    └─────────────────────┘    └─────────────────────┘    └─────────────────────┘

Windows uses a virtual desktop where:
- Primary monitor typically starts at (0,0)
- Other monitors extend left/right/up/down
- Negative coordinates are possible (monitors to the left)
```

---

## Similar/Related Projects

| Project | What it does | Why it's different |
|---------|-------------|-------------------|
| [Komorebi](https://github.com/LGUG2Z/komorebi) | Tiling window manager | General-purpose, not terminal-focused |
| [GlazeWM](https://glazewm.com/) | Tiling window manager | Similar to Komorebi |
| [PowerToys FancyZones](https://learn.microsoft.com/en-us/windows/powertoys/fancyzones) | Window snap zones | No CLI, manual arrangement |
| [tmux](https://github.com/tmux/tmux) | Terminal multiplexer | Single window, not multi-monitor |
| [Zellij](https://zellij.dev/) | Terminal workspace | Single window, not multi-monitor |

**Our differentiator:** Single command → multiple monitors → project picker → Claude Code ready

---

## Existing Tools We Can Leverage

1. **Windows Terminal** - For spawning terminals
2. **fzf** - For interactive project selection
3. **lxn/win** - For Windows API calls in Go
4. **Cobra** - For CLI framework
5. **Viper** - For config management

---

## Technical Challenges & Solutions

### Challenge 1: Window Positioning Timing
**Problem:** Window needs time to spawn before we can position it.
**Solution:** Poll for window handle with retry, use window title to identify.

### Challenge 2: Monitor Detection Accuracy
**Problem:** Virtual desktop coordinates can be confusing.
**Solution:** Use `EnumDisplayMonitors` + `GetMonitorInfo` to get exact bounds.

### Challenge 3: fzf Integration
**Problem:** Need to run fzf, get selection, then run Claude.
**Solution:** Create a batch/PowerShell wrapper script that chains the commands.

### Challenge 4: Cross-Session Persistence
**Problem:** User might want to save which projects are open where.
**Solution:** Future feature - save/restore session state.

---

## Development Phases

### Phase 1: Proof of Concept (Current)
- [ ] Detect monitors and their positions
- [ ] Spawn multiple Windows Terminal windows
- [ ] Position windows on correct monitors
- [ ] Open each in projects directory
- [ ] Run fzf picker → Claude command

### Phase 2: Configuration
- [ ] YAML config file support
- [ ] `quickstart init` setup wizard
- [ ] Named monitor support
- [ ] Layout presets (grid, vertical, horizontal)

### Phase 3: Polish
- [ ] Better error handling
- [ ] Logging and debug mode
- [ ] Custom layouts per profile
- [ ] Pre-assigned projects option

### Phase 4: Advanced Features
- [ ] Session save/restore
- [ ] Project-specific commands
- [ ] Integration with other terminals (Alacritty, etc.)
- [ ] Linux/macOS support

---

## References

### Windows API Documentation
- [EnumDisplayMonitors](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-enumdisplaymonitors)
- [GetMonitorInfo](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getmonitorinfow)
- [SetWindowPos](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setwindowpos)
- [Multi-Monitor Programming Guide](https://www.realtimesoft.com/multimon/programming/)

### Go Libraries
- [lxn/win](https://github.com/lxn/win) - Windows API wrapper
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration
- [bubbletea](https://github.com/charmbracelet/bubbletea) - TUI framework (alternative to fzf)

### Inspiration
- [Positioning Windows with PowerShell](https://dadoverflow.com/2018/11/18/positioning-windows-with-powershell/)
- [Microsoft Terminal GitHub](https://github.com/microsoft/terminal)
