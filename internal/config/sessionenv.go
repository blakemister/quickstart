package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// BaseEnvAllowlist is the set of Windows environment variable NAMES that are
// copied from the host environment into a clean session env. These are the
// essentials a console process needs to run (path resolution, temp dirs, user
// identity for display). It contains ZERO secret-shaped names: no API keys, no
// tokens, no per-tool config dirs. Anything not on this list is dropped, so a
// secret leaked into the launching shell never reaches a launched tool unless a
// profile explicitly provides it.
var BaseEnvAllowlist = []string{
	"SYSTEMROOT",
	"SYSTEMDRIVE",
	"WINDIR",
	"PATH",
	"PATHEXT",
	"COMSPEC",
	"TEMP",
	"TMP",
	"USERPROFILE",
	"HOMEDRIVE",
	"HOMEPATH",
	"APPDATA",
	"LOCALAPPDATA",
	"PROGRAMDATA",
	"PROGRAMFILES",
	"PROGRAMFILES(X86)",
	"PROGRAMW6432",
	"NUMBER_OF_PROCESSORS",
	"PROCESSOR_ARCHITECTURE",
	"OS",
	"COMPUTERNAME",
	"USERNAME",
	"USERDOMAIN",
	"SESSIONNAME",
	"PUBLIC",
}

// accountConfigDirEnvVars maps a profile AccountConfigDirs key to the env var
// name the corresponding tool reads for its isolated config directory.
var accountConfigDirEnvVars = map[string]string{
	"claude": "CLAUDE_CONFIG_DIR",
	"gh":     "GH_CONFIG_DIR",
	"gws":    "GOOGLE_WORKSPACE_CLI_CONFIG_DIR",
}

// LaunchSpec describes how to launch a tool for a resolved session.
type LaunchSpec struct {
	Command string
	Args    []string
	Dir     string
}

// envLayer accumulates env vars with case-insensitive keys (Windows semantics).
// Keys are stored upper-cased for compare/dedup, with the canonical (emitted)
// name and value tracked per key. Later writes override earlier ones.
type envLayer struct {
	canonical map[string]string // UPPER -> canonical name to emit
	values    map[string]string // UPPER -> value
}

func newEnvLayer() *envLayer {
	return &envLayer{
		canonical: make(map[string]string),
		values:    make(map[string]string),
	}
}

// set writes a key/value, overriding any existing entry for the same key
// (case-insensitive). The canonical name from the most recent set wins.
func (l *envLayer) set(name, value string) {
	if name == "" {
		return
	}
	up := strings.ToUpper(name)
	l.canonical[up] = name
	l.values[up] = value
}

// setAll applies every entry in m via set.
func (l *envLayer) setAll(m map[string]string) {
	for k, v := range m {
		l.set(k, v)
	}
}

// render returns a sorted []string of "KEY=VALUE", one entry per key, using the
// canonical name for each.
func (l *envLayer) render() []string {
	out := make([]string, 0, len(l.values))
	for up, val := range l.values {
		out = append(out, l.canonical[up]+"="+val)
	}
	sort.Strings(out)
	return out
}

// cleanBaseEnv returns an envLayer pre-populated with only the allowlisted host
// environment variables, copied by name from os.Environ(). This is the clean
// base every session is built from — it NEVER includes the full host env.
func cleanBaseEnv() *envLayer {
	allowed := make(map[string]bool, len(BaseEnvAllowlist))
	for _, name := range BaseEnvAllowlist {
		allowed[strings.ToUpper(name)] = true
	}

	layer := newEnvLayer()
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		name := kv[:eq]
		if allowed[strings.ToUpper(name)] {
			layer.set(name, kv[eq+1:])
		}
	}
	return layer
}

// applyProfileLayers layers a profile's secrets, per-tool config dirs, and git
// identity onto the given env layer (in that low->high precedence order).
func applyProfileLayers(layer *envLayer, keys AccountKeys, profile *Profile) {
	if profile == nil {
		return
	}

	// Profile secrets (profile:<id> namespace).
	layer.setAll(KeysForProfile(keys, profile.ID))

	// Per-company config dirs override profile secrets.
	for tool, dir := range profile.AccountConfigDirs {
		if envVar, ok := accountConfigDirEnvVars[tool]; ok && dir != "" {
			layer.set(envVar, dir)
		}
	}

	// Git identity is the highest profile-level layer.
	if profile.GitIdentity.Name != "" {
		layer.set("GIT_AUTHOR_NAME", profile.GitIdentity.Name)
		layer.set("GIT_COMMITTER_NAME", profile.GitIdentity.Name)
	}
	if profile.GitIdentity.Email != "" {
		layer.set("GIT_AUTHOR_EMAIL", profile.GitIdentity.Email)
		layer.set("GIT_COMMITTER_EMAIL", profile.GitIdentity.Email)
	}
}

// ResolveSessionEnv builds the isolated environment and launch spec for running
// the given account inside the given project. The env is built from a CLEAN
// allowlisted base (never the full host env) and then layered, low to high:
//
//	clean base
//	  < legacy account keys (KeysForAccount)
//	  < profile secrets (KeysForProfile)
//	  < per-company config dirs (profile.AccountConfigDirs)
//	  < git identity (profile.GitIdentity)
//
// Windows env keys are case-insensitive, so keys are normalized for compare/
// dedup and each key is emitted exactly once with a canonical name. The result
// is a sorted []string of KEY=VALUE. This function NEVER calls
// append(os.Environ(), ...).
//
// projectID may be "" for an unbound launch (clean base + account keys only).
func ResolveSessionEnv(cfg *Config, keys AccountKeys, projectID, accountID string) ([]string, LaunchSpec, error) {
	if cfg == nil {
		return nil, LaunchSpec{}, fmt.Errorf("resolve session env: nil config")
	}

	account := AccountByID(cfg.Accounts, accountID)
	if account == nil {
		return nil, LaunchSpec{}, fmt.Errorf("resolve session env: unknown account %q", accountID)
	}

	var project *Project
	if projectID != "" {
		project = ProjectByID(cfg.Projects, projectID)
		if project == nil {
			return nil, LaunchSpec{}, fmt.Errorf("resolve session env: unknown project %q", projectID)
		}
	}

	layer := cleanBaseEnv()

	// Legacy account-scoped keys (lowest secret layer).
	layer.setAll(KeysForAccount(keys, accountID))

	// Profile layers (secrets, config dirs, git identity).
	if project != nil && project.Profile != "" {
		profile := ProfileByID(cfg, project.Profile)
		applyProfileLayers(layer, keys, profile)
	}

	dir := ""
	if project != nil {
		dir = project.Path
	}

	spec := LaunchSpec{
		Command: account.Command,
		Args:    account.ResolvedArgs(),
		Dir:     dir,
	}
	return layer.render(), spec, nil
}

// ResolveProfileEnv builds the isolated environment for a project's profile
// WITHOUT any account command — used for service probing. It applies the clean
// base plus the profile layers (secrets, config dirs, git identity). projectID
// may be "" for an unbound (clean base only) environment.
func ResolveProfileEnv(cfg *Config, keys AccountKeys, projectID string) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("resolve profile env: nil config")
	}

	layer := cleanBaseEnv()

	if projectID != "" {
		project := ProjectByID(cfg.Projects, projectID)
		if project == nil {
			return nil, fmt.Errorf("resolve profile env: unknown project %q", projectID)
		}
		if project.Profile != "" {
			profile := ProfileByID(cfg, project.Profile)
			applyProfileLayers(layer, keys, profile)
		}
	}

	return layer.render(), nil
}
