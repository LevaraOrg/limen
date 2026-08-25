package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const initTemplate = `# limen — the context of this directory tree.
# Flat YAML: one key: value per line. Every field is optional.
label: %s

# What this tree is for, in one line — agents route notes and tasks by it
# (limen list, limen note). topics: is comma-separated.
purpose:
topics:

actor:
githubUser:
claudeConfigDir:
gcloudAccount:
gcloudProject:
provider: anthropic
model: claude-opus-5

# Points ANTHROPIC_BASE_URL at a local Nuncio, so the model route belongs to
# the project rather than to the shell it was started from. Leave empty for
# the real API.
gateway:

# The port this tree's service listens on while you work on it. It is exported
# as PORT (and LIMEN_DEV_PORT), and  limen ports --caddy  turns it into a
# reverse-proxy site — so the service and the proxy read the same line.
# devHost defaults to <label>.localhost;  limen ports  shows the allocation
# across every registered context and flags a port claimed twice.
devPort:
devHost:

# Keychain instead of plaintext. keychainAccount falls back to actor.
keychainService: limen-anthropic
keychainAccount:
`

// CmdInit scaffolds .limen/limen.yaml. It refuses to overwrite: this file
// carries per-machine identity and clobbering it silently would be the worst
// outcome. A legacy flat .limen.yaml is also a refusal — lifting it is
// migrate's job, and a second descriptor would leave two truths.
func CmdInit(w io.Writer, dir string) error {
	target := filepath.Join(dir, ".limen", "limen.yaml")
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s already exists", target)
	}
	if _, err := os.Stat(filepath.Join(dir, ".limen.yaml")); err == nil {
		return fmt.Errorf(".limen.yaml (old layout) already exists — `limen migrate` lifts it into .limen/")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".limen"), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf(initTemplate, filepath.Base(dir))
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "created: %s\n", target)

	// This file carries per-machine identity — githubUser, claudeConfigDir,
	// gcloudAccount — and may end up holding an apiKey that someone pasted in.
	// Committing it leaks one machine's setup into a shared repository, so the
	// ignore entry is part of creating it, not a separate thing to remember.
	if err := ignoreLimenFile(w, dir); err != nil {
		fmt.Fprintf(w, "Note: .gitignore not adjusted (%v)\n", err)
		fmt.Fprintln(w, "Add it yourself:  echo "+ignoreEntry+" >> .gitignore")
	}
	return nil
}

// ignoreEntry is what goes into .gitignore: only the descriptor. The rest of
// .limen/ (notes.md, meta.yaml) is project content and belongs in the
// repository — ignoring the whole directory would hide it.
const ignoreEntry = ".limen/limen.yaml"

// ignoreLimenFile adds the descriptor to an existing .gitignore. It does not
// create one: a directory without .gitignore may not be a git repository at
// all, and planting files there would be presumptuous.
func ignoreLimenFile(w io.Writer, dir string) error {
	path := filepath.Join(dir, ".gitignore")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No .gitignore to append to. Creating one would add a tracked file
			// to someone else's repository just to hide a machine-local one, so
			// use .git/info/exclude instead: same effect, nothing to commit.
			return excludeLimenFile(w, dir)
		}
		return err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == ignoreEntry {
			fmt.Fprintln(w, ignoreEntry+" already in .gitignore.")
			return nil
		}
	}

	entry := ignoreEntry + "\n"
	if len(body) > 0 && !strings.HasSuffix(string(body), "\n") {
		entry = "\n" + entry
	}
	entry = "\n# limen: machine-local identity, does not belong in the repository\n" + strings.TrimPrefix(entry, "\n")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(entry); err != nil {
		return err
	}
	fmt.Fprintln(w, ignoreEntry+" added to .gitignore.")
	return nil
}

// excludeLimenFile hides the descriptor via .git/info/exclude — the
// per-checkout ignore list, which is not part of the repository content.
//
// Only reached when there is no .gitignore. Outside a work tree there is nothing
// to protect against, so a missing .git is not an error.
func excludeLimenFile(w io.Writer, dir string) error {
	gitDir := filepath.Join(dir, ".git")
	st, err := os.Stat(gitDir)
	if err != nil || !st.IsDir() {
		fmt.Fprintln(w, "Not a git repository — "+ignoreEntry+" needs no ignore entry.")
		return nil
	}

	path := filepath.Join(gitDir, "info", "exclude")
	body, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == ignoreEntry {
			fmt.Fprintln(w, ignoreEntry+" already in .git/info/exclude.")
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := "# limen: machine-local identity, does not belong in the repository\n" + ignoreEntry + "\n"
	if len(body) > 0 && !strings.HasSuffix(string(body), "\n") {
		entry = "\n" + entry
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(entry); err != nil {
		return err
	}
	fmt.Fprintln(w, ignoreEntry+" added to .git/info/exclude (no .gitignore present).")
	return nil
}

// CmdKeychainImport moves a plaintext key out of the config into the keychain.
func CmdKeychainImport(w io.Writer, c *Context) error {
	if c == nil {
		return fmt.Errorf("no context found")
	}
	if !c.HasPlaintextKey() {
		return fmt.Errorf("no plaintext key in the configuration")
	}
	if _, err := exec.LookPath("security"); err != nil {
		return fmt.Errorf("security(1) is macOS only")
	}
	service := c.KeychainService
	if service == "" {
		provider := c.Provider
		if provider == "" {
			provider = "anthropic"
		}
		service = "limen-" + provider
	}
	account := c.KeychainAccount
	if account == "" {
		account = c.Actor
	}
	if account == "" {
		return fmt.Errorf("actor or keychainAccount must be set")
	}
	cmd := exec.Command("security", "add-generic-password",
		"-U", "-s", service, "-a", account, "-w", c.PlaintextKey)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security: %v: %s", err, out)
	}
	fmt.Fprintf(w, "stored in the keychain: service=%s account=%s\n", service, account)
	fmt.Fprintln(w, "Now remove the apiKey: line from the configuration.")
	return nil
}

const zshHook = `# limen — add to .zshrc:  eval "$(limen hook zsh)"
# One call per directory change; LIMEN_SEGMENT rides along in the same output.
_limen_apply() {
  local out
  out="$(limen shell 2>/dev/null)" || return 0
  if [[ -n "$out" ]]; then
    eval "$out"
  else
    unset LIMEN_ROOT LIMEN_LABEL LIMEN_ACTOR LIMEN_GH_USER LIMEN_SEGMENT
  fi
}
autoload -Uz add-zsh-hook
add-zsh-hook chpwd _limen_apply
_limen_apply
# Optional, in the status line:
#   RPROMPT='%F{244}${LIMEN_SEGMENT}%f'
`

const bashHook = `# limen — add to .bashrc:  eval "$(limen hook bash)"
_limen_apply() {
  local out
  out="$(limen shell 2>/dev/null)" || return 0
  if [[ -n "$out" ]]; then
    eval "$out"
  else
    unset LIMEN_ROOT LIMEN_LABEL LIMEN_ACTOR LIMEN_GH_USER LIMEN_SEGMENT
  fi
}
PROMPT_COMMAND="_limen_apply${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
_limen_apply
`

// CmdHook prints the shell integration.
func CmdHook(w io.Writer, shell string) error {
	switch shell {
	case "", "zsh":
		io.WriteString(w, zshHook)
	case "bash":
		io.WriteString(w, bashHook)
	default:
		return fmt.Errorf("unknown shell %q (zsh, bash)", shell)
	}
	return nil
}
