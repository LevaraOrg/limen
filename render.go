package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Prompt is the one-line status segment. It must not touch the keychain: it is
// redrawn on every prompt, and a 20 ms security(1) call there is felt.
func Prompt(c *Context) string {
	if c == nil {
		return ""
	}
	parts := []string{c.Label}
	if c.GithubUser != "" {
		parts = append(parts, c.GithubUser)
	}
	if c.Model != "" {
		parts = append(parts, c.Model)
	}
	if c.HasPlaintextKey() {
		parts = append(parts, "!key-in-config")
	}
	return strings.Join(parts, " · ")
}

// skillView is what an agent reads to know which skills this project carries
// and which it deliberately does without. Active comes from the lock — what was
// written — so it cannot claim a skill the directory does not hold.
type skillView struct {
	Active []string `json:"active"`
	Paused []string `json:"paused"`
}

type jsonView struct {
	Root            string    `json:"root"`
	Label           string    `json:"label"`
	Actor           string    `json:"actor"`
	GithubUser      string    `json:"github_user"`
	ClaudeConfigDir string    `json:"claude_config_dir"`
	GcloudAccount   string    `json:"gcloud_account"`
	GcloudProject   string    `json:"gcloud_project"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	Gateway         string    `json:"gateway"`
	Purpose         string    `json:"purpose"`
	Topics          []string  `json:"topics"`
	Profiles        []Profile `json:"profiles"`
	Skills          skillView `json:"skills"`
	Service         *Service  `json:"service"`
	Source          string    `json:"source"`
	APIKeyPresent   bool      `json:"api_key_present"`
	APIKeyInConfig  bool      `json:"api_key_in_config"`
}

// RenderJSON writes the machine-readable view. Without a context it writes `{}`
// so consumers can call it unconditionally.
func RenderJSON(w io.Writer, c *Context, r KeyResolver) error {
	if c == nil {
		_, err := fmt.Fprintln(w, "{}")
		return err
	}
	_, _, present := r.Resolve(c)
	active, paused := SkillState(c)
	v := jsonView{
		Root:            c.Root,
		Label:           c.Label,
		Actor:           c.Actor,
		GithubUser:      c.GithubUser,
		ClaudeConfigDir: c.ClaudeDir,
		GcloudAccount:   c.GcloudAccount,
		GcloudProject:   c.GcloudProject,
		Provider:        c.Provider,
		Model:           c.Model,
		Gateway:         c.Gateway,
		Purpose:         c.Purpose,
		Topics:          c.TopicList(),
		Profiles:        c.ProfileList(),
		Skills:          skillView{Active: active, Paused: paused},
		Service:         c.Service,
		Source:          string(c.Source),
		APIKeyPresent:   present,
		APIKeyInConfig:  c.HasPlaintextKey(),
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

// RenderShow writes the human-readable overview.
func RenderShow(w io.Writer, c *Context, r KeyResolver) {
	fmt.Fprintf(w, "root:         %s\n", c.Root)
	fmt.Fprintf(w, "label:        %s\n", c.Label)
	line := func(caption, value string) {
		if value != "" {
			fmt.Fprintf(w, "%-13s %s\n", caption+":", value)
		}
	}
	line("actor", c.Actor)
	line("github user", c.GithubUser)
	line("claude dir", c.ClaudeDir)
	line("gcloud acct", c.GcloudAccount)
	line("gcloud proj", c.GcloudProject)
	line("provider", c.Provider)
	line("model", c.Model)
	line("gateway", c.Gateway)
	line("purpose", c.Purpose)
	line("topics", c.Topics)
	if c.HasService() {
		line("service", fmt.Sprintf("%s (%s, %s)", c.Service.Kind, c.Service.APIVersion, c.Service.File))
	}
	if profiles := c.ProfileList(); len(profiles) > 0 {
		names := make([]string, len(profiles))
		for i, p := range profiles {
			names[i] = p.String()
		}
		line("profiles", strings.Join(names, ", "))
	}
	if active, paused := SkillState(c); len(active) > 0 || len(paused) > 0 {
		line("skills", strings.Join(active, ", "))
		if len(paused) > 0 {
			line("paused", strings.Join(paused, ", "))
		}
	}
	fmt.Fprintf(w, "api key:      %s\n", KeySource(c, r))
	switch {
	case c.Source == SourceOrca:
		fmt.Fprintln(w, "source:       .orca/ (legacy)")
	case c.flatFile:
		fmt.Fprintln(w, "source:       .limen.yaml (old — `limen migrate` lifts it into .limen/)")
	default:
		fmt.Fprintln(w, "source:       .limen/limen.yaml")
	}
	if c.HasPlaintextKey() {
		fmt.Fprintln(w, "\nWarning: a plaintext key is still sitting in the configuration.")
		fmt.Fprintln(w, "Move it:      limen keychain-import")
	}
}

// shellQuote wraps a value in single quotes for eval, escaping embedded quotes.
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// RenderShell writes export lines for `eval "$(limen shell)"`.
//
// Empty values are omitted on purpose: moving into a project without
// gcloudProject must not export an empty CLOUDSDK_CORE_PROJECT and confuse
// gcloud. Without a context it writes nothing at all, so the call is safe from
// a shell startup file.
func RenderShell(w io.Writer, c *Context, r KeyResolver) {
	if c == nil {
		return
	}
	emit := func(name, value string) {
		if value != "" {
			fmt.Fprintf(w, "export %s=%s\n", name, shellQuote(value))
		}
	}
	emit("LIMEN_ROOT", c.Root)
	emit("LIMEN_LABEL", c.Label)
	emit("LIMEN_ACTOR", c.Actor)
	emit("LIMEN_GH_USER", c.GithubUser)
	emit("LIMEN_PROVIDER", c.Provider)
	emit("LIMEN_MODEL", c.Model)
	emit("CLAUDE_CONFIG_DIR", c.ClaudeDir)
	emit("CLOUDSDK_CORE_ACCOUNT", c.GcloudAccount)
	emit("CLOUDSDK_CORE_PROJECT", c.GcloudProject)
	// Pointing at a local Nuncio makes the model route a property of the
	// project rather than of the shell it was started from.
	emit("ANTHROPIC_BASE_URL", c.Gateway)
	// The segment rides along so the chpwd hook needs a single call.
	emit("LIMEN_SEGMENT", Prompt(c))
	if key, _, ok := r.Resolve(c); ok {
		emit("ANTHROPIC_API_KEY", key)
	}
}
