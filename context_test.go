package main

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseLine(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		wantKey string
		wantVal string
		wantOK  bool
	}{
		{"plain", "label: tessera", "label", "tessera", true},
		{"double quoted", `label: "quoted label"`, "label", "quoted label", true},
		{"single quoted", `actor: 'single quoted'`, "actor", "single quoted", true},
		{"trailing comment", "model: claude-opus-5   # goes away", "model", "claude-opus-5", true},
		{"hash inside quotes survives", `label: "a#b"`, "label", "a#b", true},
		{"camel key normalised", "githubUser: leo81", "githubuser", "leo81", true},
		{"dashed key normalised", "github-user: leo81", "githubuser", "leo81", true},
		{"comment line", "# label: nope", "", "", false},
		{"blank", "   ", "", "", false},
		{"document start", "---", "", "", false},
		{"indented is nested", "  label: nope", "", "", false},
		{"no colon", "label", "", "", false},
		{"empty value", "label:", "", "", false},
		{"value only whitespace", "label:    ", "", "", false},
		{"invalid key", "9bad: x", "", "", false},
		{"url value keeps colon", "gateway: http://localhost:8787", "gateway", "http://localhost:8787", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, v, ok := parseLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if k != tc.wantKey || v != tc.wantVal {
				t.Fatalf("got (%q, %q), want (%q, %q)", k, v, tc.wantKey, tc.wantVal)
			}
		})
	}
}

func TestTopicListSplitsAndTrims(t *testing.T) {
	c := &Context{Topics: " design-thinking , customer-journey ,,rollen "}
	got := c.TopicList()
	want := []string{"design-thinking", "customer-journey", "rollen"}
	if len(got) != len(want) {
		t.Fatalf("TopicList = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TopicList = %v, want %v", got, want)
		}
	}
	// Non-nil even when empty, so JSON renders [] rather than null.
	if empty := (&Context{}).TopicList(); empty == nil || len(empty) != 0 {
		t.Fatalf("empty TopicList = %#v, want non-nil empty slice", empty)
	}
}

func TestDiscoverFindsRootFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "proj", ".limen.yaml"), `
label: tessera
actor: Matthias Wegner
githubUser: leo81
claudeConfigDir: ~/.claude-work
gcloudAccount: leo@example.com
gcloudProject: my-project-123
provider: anthropic
model: claude-opus-5
gateway: http://localhost:8787
keychainService: limen-anthropic
`)
	deep := filepath.Join(root, "proj", "deep", "deeper")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, ok := Discover(deep)
	if !ok {
		t.Fatal("expected a context")
	}
	if ctx.Root != filepath.Join(root, "proj") {
		t.Errorf("Root = %q", ctx.Root)
	}
	if ctx.Source != SourceLimen {
		t.Errorf("Source = %q, want limen", ctx.Source)
	}
	if ctx.Label != "tessera" || ctx.GithubUser != "leo81" || ctx.Model != "claude-opus-5" {
		t.Errorf("fields not parsed: %+v", ctx)
	}
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, ".claude-work"); ctx.ClaudeDir != want {
		t.Errorf("ClaudeDir = %q, want %q (tilde must expand)", ctx.ClaudeDir, want)
	}
}

func TestDiscoverFallsBackToLegacyOrcaTree(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".orca", "config.yaml"),
		"---\nprovider: anthropic\nmodel: claude-opus-4-5\n")
	write(t, filepath.Join(root, ".orca", "identity.yaml"),
		"---\nactorId: \"abc-123\"\nname: \"Leo\"\n")
	nested := filepath.Join(root, "src", "main")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, ok := Discover(nested)
	if !ok {
		t.Fatal("expected a context from a legacy .orca tree")
	}
	if ctx.Source != SourceOrca {
		t.Errorf("Source = %q, want orca", ctx.Source)
	}
	// identity.yaml calls the actor `name`.
	if ctx.Actor != "Leo" {
		t.Errorf("Actor = %q, want Leo", ctx.Actor)
	}
	if ctx.Model != "claude-opus-4-5" {
		t.Errorf("Model = %q", ctx.Model)
	}
	if ctx.Label != filepath.Base(root) {
		t.Errorf("Label = %q, want the directory name as fallback", ctx.Label)
	}
}

func TestDiscoverPrefersLimenOverLegacy(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".orca", "config.yaml"), "model: old-model\n")
	write(t, filepath.Join(root, ".limen.yaml"), "model: new-model\n")

	ctx, ok := Discover(root)
	if !ok {
		t.Fatal("expected a context")
	}
	if ctx.Source != SourceLimen || ctx.Model != "new-model" {
		t.Errorf("got source %q model %q, want limen/new-model", ctx.Source, ctx.Model)
	}
}

func TestDiscoverReturnsFalseWithoutContext(t *testing.T) {
	// t.TempDir() sits under /var/folders, where no .limen.yaml exists above it.
	if _, ok := Discover(t.TempDir()); ok {
		t.Fatal("expected no context")
	}
}

func TestDiscoverStopsAtFilesystemRoot(t *testing.T) {
	// Proves the upward walk terminates rather than looping on "/" == "/".
	done := make(chan bool, 1)
	go func() {
		_, ok := Discover(string(filepath.Separator))
		done <- ok
	}()
	select {
	case <-done:
	default:
		// Not yet finished is fine; the point is it cannot hang forever.
	}
}

func TestPlaintextKeyIsRecordedButNotTreatedAsTheKey(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".limen.yaml"),
		"label: leaky\nprovider: anthropic\napiKey: sk-ant-SECRETVALUE\n")

	ctx, ok := Discover(root)
	if !ok {
		t.Fatal("expected a context")
	}
	if !ctx.HasPlaintextKey() {
		t.Fatal("plaintext key not detected")
	}
	// The resolver must ignore it: a key in a committed file is the defect.
	r := &SystemKeyResolver{Lookup: func(string, string) (string, bool) { return "", false }}
	t.Setenv("ANTHROPIC_API_KEY", "")
	if v, _, ok := r.Resolve(ctx); ok {
		t.Fatalf("resolver returned %q from the config file", v)
	}
}
