package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const initTemplate = `# limen — Kontext dieses Verzeichnisbaums.
# Flaches YAML: ein key: value je Zeile. Alle Felder sind optional.
label: %s
actor:
githubUser:
claudeConfigDir:
gcloudAccount:
gcloudProject:
provider: anthropic
model: claude-opus-5

# Zeigt ANTHROPIC_BASE_URL auf ein lokales Nuncio, damit die Modellroute zum
# Projekt gehört und nicht zur Shell. Leer lassen für die echte API.
gateway:

# Schlüsselbund statt Klartext. keychainAccount fällt auf actor zurück.
keychainService: limen-anthropic
keychainAccount:
`

// CmdInit scaffolds a .limen.yaml. It refuses to overwrite: this file carries
// per-machine identity and clobbering it silently would be the worst outcome.
func CmdInit(w io.Writer, dir string) error {
	target := filepath.Join(dir, ".limen.yaml")
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s existiert bereits", target)
	}
	body := fmt.Sprintf(initTemplate, filepath.Base(dir))
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "angelegt: %s\n", target)

	// This file carries per-machine identity — githubUser, claudeConfigDir,
	// gcloudAccount — and may end up holding an apiKey that someone pasted in.
	// Committing it leaks one machine's setup into a shared repository, so the
	// ignore entry is part of creating it, not a separate thing to remember.
	if err := ignoreLimenFile(w, dir); err != nil {
		fmt.Fprintf(w, "Hinweis: .gitignore nicht angepasst (%v)\n", err)
		fmt.Fprintln(w, "Bitte selbst eintragen:  echo .limen.yaml >> .gitignore")
	}
	return nil
}

// ignoreLimenFile adds .limen.yaml to an existing .gitignore. It does not create
// one: a directory without .gitignore may not be a git repository at all, and
// planting files there would be presumptuous.
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
		if strings.TrimSpace(line) == ".limen.yaml" {
			fmt.Fprintln(w, ".limen.yaml steht bereits in .gitignore.")
			return nil
		}
	}

	entry := ".limen.yaml\n"
	if len(body) > 0 && !strings.HasSuffix(string(body), "\n") {
		entry = "\n" + entry
	}
	entry = "\n# limen: maschinenlokale Identität, gehört nicht ins Repository\n" + strings.TrimPrefix(entry, "\n")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(entry); err != nil {
		return err
	}
	fmt.Fprintln(w, ".limen.yaml in .gitignore eingetragen.")
	return nil
}

// excludeLimenFile hides .limen.yaml via .git/info/exclude — the per-checkout
// ignore list, which is not part of the repository content.
//
// Only reached when there is no .gitignore. Outside a work tree there is nothing
// to protect against, so a missing .git is not an error.
func excludeLimenFile(w io.Writer, dir string) error {
	gitDir := filepath.Join(dir, ".git")
	st, err := os.Stat(gitDir)
	if err != nil || !st.IsDir() {
		fmt.Fprintln(w, "Kein Git-Repository — .limen.yaml braucht keinen Ignore-Eintrag.")
		return nil
	}

	path := filepath.Join(gitDir, "info", "exclude")
	body, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == ".limen.yaml" {
			fmt.Fprintln(w, ".limen.yaml steht bereits in .git/info/exclude.")
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := "# limen: maschinenlokale Identität, gehört nicht ins Repository\n.limen.yaml\n"
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
	fmt.Fprintln(w, ".limen.yaml in .git/info/exclude eingetragen (keine .gitignore vorhanden).")
	return nil
}

// CmdKeychainImport moves a plaintext key out of the config into the keychain.
func CmdKeychainImport(w io.Writer, c *Context) error {
	if c == nil {
		return fmt.Errorf("kein Kontext gefunden")
	}
	if !c.HasPlaintextKey() {
		return fmt.Errorf("kein Klartextschlüssel in der Konfiguration")
	}
	if _, err := exec.LookPath("security"); err != nil {
		return fmt.Errorf("security(1) nur auf macOS")
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
		return fmt.Errorf("actor oder keychainAccount muss gesetzt sein")
	}
	cmd := exec.Command("security", "add-generic-password",
		"-U", "-s", service, "-a", account, "-w", c.PlaintextKey)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security: %v: %s", err, out)
	}
	fmt.Fprintf(w, "im Schlüsselbund abgelegt: service=%s account=%s\n", service, account)
	fmt.Fprintln(w, "Jetzt die Zeile apiKey: aus der Konfiguration entfernen.")
	return nil
}

const zshHook = `# limen — in .zshrc einbinden:  eval "$(limen hook zsh)"
# Ein Aufruf je Verzeichniswechsel; LIMEN_SEGMENT kommt aus derselben Ausgabe.
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
# Optional in der Statuszeile:
#   RPROMPT='%F{244}${LIMEN_SEGMENT}%f'
`

const bashHook = `# limen — in .bashrc einbinden:  eval "$(limen hook bash)"
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
		return fmt.Errorf("unbekannte Shell %q (zsh, bash)", shell)
	}
	return nil
}
