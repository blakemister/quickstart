---
description: Full guided setup — builds qs, detects tools, configures accounts, walks through auth
allowed-tools: ["Read", "Bash", "Grep", "Glob", "Write", "AskUserQuestion"]
---

# Onboard — Guided qs Setup

Walk the user through a complete qs setup: build, install, project folder, tool detection, authentication, and verification. Be friendly and conversational. Use AskUserQuestion for every decision point — never assume.

## Step 1: Welcome + Prerequisites

Print a brief welcome:

> **Welcome to qs!** I'll walk you through the full setup. Here's what we'll do:
> 1. Build and install the `qs` binary
> 2. Set your projects folder
> 3. Detect which AI coding tools you have installed
> 4. Walk through authentication for each tool
> 5. (Optional) Configure multi-monitor layouts
>
> Let's get started.

Then check prerequisites by running these commands:

1. `go version` — parse the output. Need Go 1.21+. If Go is missing or too old, tell the user to install it from https://go.dev/dl/ and **stop here**.

2. `where wt` — check for Windows Terminal. If not found, tell the user to install Windows Terminal from the Microsoft Store and **stop here**.

3. `where npm` — check for npm. If not found, **warn** the user that npm is needed to install AI tools later, but continue. They may already have the tools they want.

## Step 2: Build & Install

Run these commands sequentially from the repo root:

```bash
go mod tidy
go test ./...
```

If tests fail, investigate and fix before continuing.

Then build and install:

```bash
go build -o qs-new.exe .
mkdir -p ~/.qs/bin/
```

If `~/.qs/bin/qs.exe` already exists, rename it out of the way:
```bash
mv ~/.qs/bin/qs.exe ~/.qs/bin/qs-old.exe 2>/dev/null
```

Then install:
```bash
cp qs-new.exe ~/.qs/bin/qs.exe
rm qs-new.exe
rm ~/.qs/bin/qs-old.exe 2>/dev/null
```

Check if `~/.qs/bin/` is in the user's PATH:
```bash
echo $PATH | tr ':' '\n' | grep -i '\.qs/bin'
```

If not found, warn the user:
> `~/.qs/bin/` is not in your PATH yet. You'll need to restart your terminal after setup, or add it manually.

Verify the install:
```bash
~/.qs/bin/qs.exe version
```

## Step 3: Set Projects Folder

Check if a config already exists:
```bash
cat ~/.qs/config.yaml 2>/dev/null
```

If it exists, read the current `projectsRoot` and tell the user what it is. Ask if they want to keep it or change it.

If no config exists, ask the user:

> Where do your project folders live? This is the parent directory that contains your project subdirectories.
>
> Default: `~/.1dev`

Options to present:
- Use the default (`~/.1dev`)
- Enter a custom path
- Use current working directory's parent

Whatever path they choose, validate it:
```bash
ls -d "<chosen_path>" 2>/dev/null
```

If it doesn't exist, ask if they want to create it:
```bash
mkdir -p "<chosen_path>"
```

Store the resolved absolute path for Step 7.

## Step 4: Detect Installed AI Tools

Read `internal/config/accounts.go` to get the current `DefaultAccounts` list. For each unique command, check if it's installed:

```bash
where claude 2>/dev/null
where codex 2>/dev/null
where gemini 2>/dev/null
where opencode 2>/dev/null
where agent 2>/dev/null
```

Present the results as a table:

```
Tool              Command     Status
─────────────────────────────────────
Claude Code       claude      ✓ Found
OpenAI Codex      codex       ✗ Not found
Gemini CLI        gemini      ✓ Found
OpenCode (z.ai)   opencode    ✗ Not found
Cursor Agent      agent       ✗ Not found
```

### Install missing tools

For each tool that was NOT found, check if npm is available. If it is, ask:

> Would you like me to install any of the missing tools?

Present the missing tools with their install commands (from `DefaultAccounts[].InstallCmd`):
- Claude Code: `npm i -g @anthropic-ai/claude-code`
- OpenAI Codex: `npm i -g @openai/codex`
- Gemini CLI: `npm i -g @google/gemini-cli`
- OpenCode: `npm i -g opencode`

For each tool the user wants installed, run the install command and verify with `where <command>`.

If npm is not available and tools are missing, tell the user how to install them manually.

### Choose which tools to enable

Ask the user which of the **found** (or newly installed) tools they want enabled in qs. Present them as a multi-select.

All found tools should default to enabled. Tools not installed should default to disabled.

### AMA Claude (multi-auth)

Only bring this up if `claude` is installed. Explain:

> qs includes an "AMA Claude" account — this is a second Claude Code instance that uses a separate config directory (`~/.claude-ama`). This is useful if you have multiple Anthropic organizations or want to keep work/personal accounts separate. Would you like to enable it?

Default: disabled. If enabled, note it for Step 5 auth.

## Step 5: Walk Through Authentication

For each enabled account that has an auth command, offer to authenticate now. Go through them one at a time.

**Important**: Auth commands are interactive — they typically open a browser for OAuth. Warn the user before running each one:

> I'm about to run `<auth command>`. This will likely open your browser for login. Ready?

Auth commands by tool (read from `DefaultAccounts` in `internal/config/accounts.go`):
- **claude**: `claude /login`
- **codex**: `codex login`
- **gemini**: `gemini` (first run triggers auth)
- **opencode**: `opencode auth login`
- **cursor**: `agent login`

### AMA Claude special handling

If ama-claude is enabled, it needs the `CLAUDE_CONFIG_DIR` env var set during auth. Read `internal/config/keys.go` `DefaultAccountKeys()` to get the correct path (should be `~/.claude-ama`).

Run:
```bash
CLAUDE_CONFIG_DIR=~/.claude-ama claude auth login
```

### Probe auth status

After authenticating Claude accounts, probe the status:
```bash
claude auth status 2>/dev/null
```

For ama-claude:
```bash
CLAUDE_CONFIG_DIR=~/.claude-ama claude auth status 2>/dev/null
```

Parse the output to show the user which email/org they're logged in as.

### Handle failures gracefully

If any auth command fails or the user skips it, that's fine. Tell them:

> No problem — you can authenticate later by running `qs accounts` and pressing `l` on any account.

## Step 6: Configure Monitors (Optional)

Ask the user:

> Would you like to configure multi-monitor window layouts? This lets qs spawn and arrange terminal windows across your displays when you run `qs all`.
>
> If you skip this, qs will use a single fullscreen window (which is what most people want).

If they say **no**: use the default config (1 monitor, "full" layout, 1 window with the default tool). Skip to Step 7.

If they say **yes**:

1. Run `~/.qs/bin/qs.exe monitors` to detect connected displays
2. For each monitor, ask:
   - Layout: full, vertical (side-by-side), horizontal (stacked), or grid
   - How many windows on this monitor
   - Which tool for each window slot (from enabled accounts)

Build the monitors config from their answers.

## Step 7: Save Config

Build the config YAML from everything gathered and write it to `~/.qs/config.yaml`.

The config format (version 4):

```yaml
version: 4
projectsRoot: "<projects_path>"
defaultAccount: "<first_enabled_account_id>"
lastAccount: "<first_enabled_account_id>"
accounts:
  - id: <id>
    label: <label>
    command: <command>
    args: [<args>]
    authCmd: <auth_cmd>
    installCmd: <install_cmd>
    icon: "<icon>"
    enabled: <true/false>
    authUser: "<email if probed>"
  # ... all accounts from DefaultAccounts, with enabled set per user choice
monitors:
  - layout: <layout>
    windows:
      - tool: <account_id>
```

Read the current `DefaultAccounts` from `internal/config/accounts.go` to get the full list of accounts with their fields. Set `enabled` based on user choices from Step 4. Set `authUser` if probed in Step 5.

Write the file:
```bash
# Use the Write tool to create ~/.qs/config.yaml with the YAML content
```

### Keys file (if ama-claude is enabled)

If ama-claude was enabled, write `~/.qs/keys.yaml`:

```yaml
ama-claude:
    CLAUDE_CONFIG_DIR: "<home>/.claude-ama"
```

Where `<home>` is the user's home directory. After writing, set permissions:
```bash
chmod 600 ~/.qs/keys.yaml
```

## Step 8: Verify & Summary

Verify the installation:
```bash
~/.qs/bin/qs.exe version
```

Print a summary:

> **Setup complete!** Here's what was configured:
>
> **Projects folder**: `<path>`
>
> **Enabled tools**:
> - ✓ Claude Code (claude) — logged in as user@example.com
> - ✓ Gemini CLI (gemini) — auth pending
> - ✗ OpenAI Codex — disabled
> ...
>
> **Monitor layout**: <description>
>
> **Next steps**:
> - Run `qs` to launch the project picker
> - Run `qs accounts` to manage tools later
> - Run `qs setup` for the full interactive wizard

If PATH was not set, remind them:
> Remember to restart your terminal so `qs` is available on your PATH.
