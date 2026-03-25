---
description: Add a new AI tool account to qs — gathers tool info, updates code, builds, installs, and runs auth setup
argument-hint: Optional tool name or description (e.g. "aider" or "second Claude account for work")
allowed-tools: ["Read", "Edit", "Write", "Bash", "Grep", "Glob", "Agent", "AskUserQuestion"]
---

# Add Account to qs

Add a new AI coding tool account to the qs launcher. This covers everything: code changes, build, install, and auth.

There are two patterns depending on what the user needs:

- **Simple account** (new tool like aider, amp, etc.) — needs an entry in `DefaultAccounts` and `SuggestedEnvVars`
- **Multi-auth account** (second login for an existing tool, like a work vs personal Claude) — also needs env var isolation via `DefaultAccountKeys` using `CLAUDE_CONFIG_DIR` or equivalent

## Step 1: Gather info

Ask the user for the following. If they provided a tool name in `$ARGUMENTS`, research it first (check if it's installed with `where <command>`, look for common CLI patterns) and pre-fill what you can.

Required fields:
- **Label**: Human-readable name (e.g. "Aider", "AMA Claude")
- **ID**: URL-safe lowercase identifier, auto-generated from label (e.g. "aider", "ama-claude")
- **Command**: The CLI binary name (e.g. "aider", "claude")
- **Args**: Default command-line arguments as a list (e.g. `["--yes-always"]`)
- **Auth command**: How to log in (e.g. "aider auth login"). Can be empty.
- **Install command**: How to install it (e.g. "pip install aider-chat"). Can be empty.
- **Icon**: Unicode emoji (suggest one based on what's already used — see existing accounts for taken icons)
- **Enabled**: Default true

Also determine:
- **Is this a second account for an existing tool?** If yes, this is the multi-auth pattern. Ask what env var controls the config directory (for Claude it's `CLAUDE_CONFIG_DIR`). The env var value should be a new directory path like `~/.claude-<suffix>`.
- **API key env var name**: What env var does this tool use for API keys? (e.g. `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`). This goes in `SuggestedEnvVars`.

Present a summary table to the user and confirm before making any code changes.

## Step 2: Read current state

Read these files to understand what exists:
1. `internal/config/accounts.go` — find `DefaultAccounts` and `SuggestedEnvVars`
2. `internal/config/keys.go` — find `DefaultAccountKeys()` (only if multi-auth pattern)
3. `internal/config/config_test.go` — find the test that checks `len(DefaultAccounts)`

Note the current number of default accounts and the existing icons/IDs to avoid collisions.

## Step 3: Make code changes

### 3a. Add to DefaultAccounts in `internal/config/accounts.go`

Insert a new Account struct in the `DefaultAccounts` slice, before the `cursor` entry (which should stay last). Follow the exact struct format of existing entries:

```go
{
    ID:         "<id>",
    Label:      "<label>",
    Command:    "<command>",
    Args:       []string{<args>},
    AuthCmd:    "<auth command>",
    InstallCmd: "<install command>",
    Icon:       "<unicode escape>",
    Enabled:    true,
},
```

### 3b. Add to SuggestedEnvVars in `internal/config/accounts.go`

If the tool's command isn't already in the map, add it:
```go
"<command>": {"<ENV_VAR_NAME>"},
```

### 3c. Add to DefaultAccountKeys in `internal/config/keys.go` (multi-auth only)

If this is a multi-auth account needing env var isolation, add an entry to the `DefaultAccountKeys()` function's return map:
```go
"<account-id>": {
    "<ENV_VAR>": filepath.Join(homeDir, ".<config-dir-name>"),
},
```

### 3d. Update tests

In `internal/config/config_test.go`, find the test checking `len(DefaultAccounts)` and increment the expected count by 1.

### 3e. Update CLAUDE.md

Update the Default Accounts table in `CLAUDE.md` to include the new account.

## Step 4: Build, test, and install

Run these sequentially:
```bash
go test ./...
go build -o qs-new.exe .
```

If tests fail, fix them. Then install:
```bash
mv ~/.qs/bin/qs.exe ~/.qs/bin/qs-old.exe 2>/dev/null
cp qs-new.exe ~/.qs/bin/qs.exe
rm qs-new.exe
rm ~/.qs/bin/qs-old.exe 2>/dev/null
```

## Step 5: Auth setup (if applicable)

If the account has an auth command:

- **Simple account**: Run the auth command directly (e.g. `aider auth login`)
- **Multi-auth account**: Run the auth command with the env var set (e.g. `CLAUDE_CONFIG_DIR=~/.claude-ama claude auth login`). Then if the default account was re-authed, help the user re-auth it too.

Verify auth worked by checking status if the tool supports it.

## Step 6: Verify

Run `qs` or confirm the new account appears in the config. The new account will be auto-merged into the user's existing config by `mergeNewDefaults()` on next launch.

Tell the user what was done and how to select the new account in the picker.
