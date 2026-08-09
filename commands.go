package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
