package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// fixedResolver stands in for the keychain so tests never touch it.
type fixedResolver struct {
	value  string
	source string
	ok     bool
}

func (f fixedResolver) Resolve(*Context) (string, string, bool) {
	return f.value, f.source, f.ok
}

func sampleContext() *Context {
	return &Context{
		Root:            "/tmp/proj",
		Source:          SourceLimen,
		Label:           "tessera",
		Actor:           "Matthias Wegner",
		GithubUser:      "leo81",
		ClaudeDir:       "/Users/x/.claude-work",
		GcloudAccount:   "leo@example.com",
		GcloudProject:   "my-project-123",
		Provider:        "anthropic",
		Model:           "claude-opus-5",
		Gateway:         "http://localhost:8787",
		KeychainService: "limen-anthropic",
	}
}

func TestPromptSegment(t *testing.T) {
	if got, want := Prompt(sampleContext()), "tessera · leo81 · claude-opus-5"; got != want {
		t.Errorf("Prompt() = %q, want %q", got, want)
	}
	c := sampleContext()
	c.PlaintextKey = "sk-ant-SECRET"
	if got := Prompt(c); !strings.Contains(got, "!key-in-config") {
		t.Errorf("Prompt() = %q, want the leak marker", got)
	}
	if got := Prompt(nil); got != "" {
		t.Errorf("Prompt(nil) = %q, want empty", got)
	}
}

func TestPromptDoesNotContainTheSecret(t *testing.T) {
	c := sampleContext()
	c.PlaintextKey = "sk-ant-SECRETVALUE"
	if strings.Contains(Prompt(c), "SECRETVALUE") {
		t.Fatal("prompt leaked the key")
	}
}

func TestRenderJSONShape(t *testing.T) {
	var buf bytes.Buffer
	c := sampleContext()
	c.PlaintextKey = "sk-ant-SECRETVALUE"
	if err := RenderJSON(&buf, c, fixedResolver{ok: true, source: "keychain limen-anthropic", value: "sk-live"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "SECRETVALUE") || strings.Contains(buf.String(), "sk-live") {
		t.Fatal("json leaked a key")
	}

	var v map[string]any
	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{
		"root", "label", "actor", "github_user", "claude_config_dir",
		"gcloud_account", "gcloud_project", "provider", "model", "gateway",
		"language", "source", "api_key_present", "api_key_in_config",
	} {
		if _, ok := v[key]; !ok {
			t.Errorf("missing key %q", key)
		}
	}
	if v["language"] != "english" {
		t.Errorf("language = %v, want the english default", v["language"])
	}
	if v["api_key_present"] != true {
		t.Error("api_key_present should be true when the resolver succeeds")
	}
	if v["api_key_in_config"] != true {
		t.Error("api_key_in_config should be true when the file carries one")
	}
}

func TestRenderJSONWithoutContextIsEmptyObject(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderJSON(&buf, nil, fixedResolver{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "{}" {
		t.Fatalf("got %q, want {}", buf.String())
	}
}

func TestRenderShellOmitsEmptyValues(t *testing.T) {
	var buf bytes.Buffer
	c := &Context{Root: "/tmp/sparse", Label: "sparse", Provider: "anthropic", Source: SourceLimen}
	RenderShell(&buf, c, fixedResolver{})
	out := buf.String()

	if !strings.Contains(out, "export LIMEN_LABEL='sparse'") {
		t.Error("label must be exported")
	}
	// An empty CLOUDSDK_CORE_PROJECT would confuse gcloud, so it must be absent.
	for _, unwanted := range []string{"CLAUDE_CONFIG_DIR", "CLOUDSDK", "ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("%s must not be exported when empty:\n%s", unwanted, out)
		}
	}
}

func TestRenderShellFullSet(t *testing.T) {
	var buf bytes.Buffer
	RenderShell(&buf, sampleContext(), fixedResolver{value: "sk-live", source: "keychain", ok: true})
	out := buf.String()

	want := []string{
		"export LIMEN_ROOT='/tmp/proj'",
		"export LIMEN_LABEL='tessera'",
		"export LIMEN_GH_USER='leo81'",
		"export CLAUDE_CONFIG_DIR='/Users/x/.claude-work'",
		"export CLOUDSDK_CORE_ACCOUNT='leo@example.com'",
		"export CLOUDSDK_CORE_PROJECT='my-project-123'",
		"export ANTHROPIC_BASE_URL='http://localhost:8787'",
		"export LIMEN_SEGMENT='tessera · leo81 · claude-opus-5'",
		"export ANTHROPIC_API_KEY='sk-live'",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing line %q in:\n%s", w, out)
		}
	}
}

func TestRenderShellWithoutContextWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	RenderShell(&buf, nil, fixedResolver{})
	if buf.Len() != 0 {
		t.Fatalf("expected no output, got %q", buf.String())
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	// A label like  it's  must survive eval intact.
	if got, want := shellQuote("it's"), `'it'\''s'`; got != want {
		t.Fatalf("shellQuote = %q, want %q", got, want)
	}
}

func TestRenderShowIncludesTheWarningAndNotTheKey(t *testing.T) {
	var buf bytes.Buffer
	c := sampleContext()
	c.PlaintextKey = "sk-ant-SECRETVALUE"
	RenderShow(&buf, c, fixedResolver{source: "keychain limen-anthropic", ok: true})
	out := buf.String()

	if strings.Contains(out, "SECRETVALUE") {
		t.Fatal("show leaked the key")
	}
	if !strings.Contains(out, "plaintext key") {
		t.Error("show must warn about a key in the config")
	}
	if !strings.Contains(out, "source:       .limen/limen.yaml") {
		t.Error("show must name the source")
	}
}

func TestRenderShowNamesTheLanguage(t *testing.T) {
	var buf bytes.Buffer
	RenderShow(&buf, sampleContext(), fixedResolver{})
	if !strings.Contains(buf.String(), "language:     english") {
		t.Errorf("show must name the working language, got:\n%s", buf.String())
	}
}

func TestRenderShowMarksLegacySource(t *testing.T) {
	var buf bytes.Buffer
	c := sampleContext()
	c.Source = SourceOrca
	RenderShow(&buf, c, fixedResolver{})
	if !strings.Contains(buf.String(), "legacy") {
		t.Error("a legacy .orca tree must be labelled as such")
	}
}
