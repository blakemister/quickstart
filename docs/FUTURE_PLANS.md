# Quickstart - Future Plans & Ideas

This document tracks ideas and features for future development. These are intentionally deferred to keep the initial proof of concept focused.

---

## Phase 2: Configuration System

### Named Profiles
```bash
quickstart dev        # Full 7-terminal dev setup
quickstart focus      # Just 2 terminals for deep work
quickstart review     # 3 terminals for code review
quickstart streaming  # Layout optimized for streaming
```

### Monitor Nicknames
Instead of "Monitor 1", "Monitor 2", users can name their displays:
- "TV", "Main", "Laptop"
- "Left", "Center", "Right"
- "Vertical", "Ultrawide"

Config detects monitors by resolution/position and matches to nicknames.

### Layout Presets
```yaml
layouts:
  grid:      # 2x2, 3x3, etc.
  vertical:  # Side by side columns
  horizontal: # Stacked rows
  primary-secondary: # One big, rest small
  custom:    # User-defined coordinates
```

---

## Phase 3: Enhanced UX

### Project Favorites
Pin frequently-used projects to the top of the picker:
```yaml
favorites:
  - api-server
  - frontend
  - mobile-app
```

### Project Groups
Organize projects into categories:
```yaml
groups:
  work:
    - company-api
    - company-frontend
  personal:
    - side-project
    - blog
  learning:
    - rust-practice
    - go-tutorials
```

### Recent Projects
Track recently opened projects, show at top of picker.

### Project-Specific Commands
Different projects might need different commands:
```yaml
projects:
  api-server:
    command: "claude --dangerously-skip-permissions"
  legacy-app:
    command: "code ."  # Just open VS Code for old projects
  docs:
    command: "nvim ."  # Open in Neovim
```

---

## Phase 4: Session Management

### Save Session
```bash
quickstart save my-session
# Saves which projects are open on which monitors
```

### Restore Session
```bash
quickstart restore my-session
# Opens the exact same projects in the same positions
```

### Auto-Save
Option to automatically save session state on exit.

### Session Sharing
Export/import session configs for team sharing:
```bash
quickstart export my-session > session.yaml
quickstart import < session.yaml
```

---

## Phase 5: Advanced Features

### Workspace Templates
Pre-defined project combinations:
```yaml
templates:
  fullstack:
    TV:
      - frontend
      - mobile-app
    Main:
      - api-server
      - database-tools
    Laptop:
      - docs
```

```bash
quickstart template fullstack
# Opens all those projects automatically
```

### Hot Reload Config
Watch config file for changes, apply without restart.

### Terminal Profiles
Support different terminal emulators:
- Windows Terminal (default)
- Alacritty
- Wezterm
- PowerShell directly
- CMD

### Pre-Run Scripts
Execute scripts before opening terminals:
```yaml
preRun:
  - "docker-compose up -d"
  - "node scripts/setup-env.js"
```

### Health Checks
Verify environment before launching:
```yaml
healthChecks:
  - command: "docker --version"
    message: "Docker is required"
  - command: "node --version"
    message: "Node.js is required"
```

---

## Phase 6: Cross-Platform

### Linux Support
- Use `xdotool` or similar for window positioning
- Support for various terminal emulators (Kitty, Alacritty, GNOME Terminal)
- X11 and Wayland support

### macOS Support
- AppleScript for window positioning
- Support for iTerm2, Terminal.app, Alacritty
- Spaces/Mission Control integration

---

## Phase 7: Integrations

### Git Integration
- Show git status in project picker
- Filter by dirty/clean repos
- Show branch names

### Claude Code Integration
- Auto-detect if Claude Code is installed
- Suggest installation if missing
- Support for Claude Code flags/options

### VS Code Integration
- Option to open projects in VS Code instead
- Multi-root workspace support

### tmux/Zellij Integration
- Option to create tmux sessions instead of windows
- Persistent sessions across reboots

---

## Community Ideas

### Plugin System
Allow community plugins for:
- Custom pickers (beyond fzf)
- Custom layouts
- Custom commands
- Integrations with other tools

### Themes
Visual customization:
- Terminal color schemes per monitor
- Custom window titles/formatting

### Telemetry (Opt-in)
Anonymous usage stats to improve the tool:
- Most used features
- Common configurations
- Error tracking

---

## Technical Debt to Address

- [ ] Proper error handling with user-friendly messages
- [ ] Logging system with debug mode
- [ ] Unit tests for core functionality
- [ ] Integration tests for Windows API calls
- [ ] CI/CD pipeline for releases
- [ ] Automated builds for Windows/Linux/macOS
- [ ] Documentation site
- [ ] Contributing guidelines
- [ ] Code of conduct

---

## Ideas Parking Lot

Things that might be interesting but need more thought:

- **Voice control**: "Hey quickstart, open my dev setup"
- **Scheduled launches**: Auto-open at certain times
- **Monitor presence detection**: Different layouts when monitors are connected/disconnected
- **Remote terminals**: SSH into remote machines as part of setup
- **Container terminals**: Open terminals inside Docker containers
- **WSL integration**: Mix Windows and WSL terminals
- **Notification on ready**: Alert when all terminals are set up
- **Screenshot layouts**: Visual editor for window positions
- **AI suggestions**: Suggest projects based on recent git activity

---

## Versioning Plan

- `0.1.0` - Proof of concept (basic functionality)
- `0.2.0` - Configuration system
- `0.3.0` - Polish and error handling
- `0.4.0` - Session management
- `0.5.0` - Advanced features
- `1.0.0` - Stable release with full documentation

---

*This document is a living brainstorm. Not all ideas will be implemented. Priorities will be driven by community feedback and practical value.*
