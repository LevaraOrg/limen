package main

// CLI tests for the registry: `list`, `register`, and the shell hook
// recording roots. Like cli_test.go these run the real binary: the contract
// with agents is stdout, files on disk, and exit codes.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sharedState returns an env that pins the registry to one directory, so
// several limen invocations in a test see the same register.
func sharedState(t *testing.T) []string {
	t.Helper()
	return []string{"XDG_STATE_HOME=" + tempDir(t)}
}

func TestCLIShellRegistersTheRootItCrossed(t *testing.T) {
	root, nested := project(t, fullConfig)
	env := sharedState(t)

	// The hook path: `shell` runs on cd and must record the root as a side
	// effect, so `list` knows it without anyone maintaining an index.
	if r := runLimen(t, nested, env, "shell"); r.code != 0 {
		t.Fatalf("shell exit %d: %s", r.code, r.stderr)
	}
	r := runLimen(t, tempDir(t), env, "list", "--json")
	if r.code != 0 {
		t.Fatalf("list exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{`"root":"` + root + `"`, `"label":"tessera"`} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("list --json missing %q:\n%s", want, r.stdout)
		}
	}

	// Crossing the same threshold twice must not duplicate the entry.
	runLimen(t, nested, env, "shell")
	r = runLimen(t, tempDir(t), env, "list", "--json")
	if strings.Count(r.stdout, root) != 1 {
		t.Errorf("root registered twice:\n%s", r.stdout)
	}
}

func TestCLIListJSONCarriesTheRoutingFields(t *testing.T) {
	_, nested := project(t, fullConfig)
	env := sharedState(t)
	runLimen(t, nested, env, "register")

	r := runLimen(t, tempDir(t), env, "list", "--json")
	for _, want := range []string{
		`"purpose":"Product strategy — role design and presentations"`,
		`"topics":["design-thinking","customer-journey"]`,
		`"language":"english"`,
		`"source":"limen"`,
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("list --json missing %q:\n%s", want, r.stdout)
		}
	}
	// The inventory is for routing, never for identity or key state.
	for _, leak := range []string{"api_key", "gcloud", "keychain", "leo81"} {
		if strings.Contains(r.stdout, leak) {
			t.Errorf("list --json must not carry %q:\n%s", leak, r.stdout)
		}
	}
}

func TestCLIRegisterListsAndPrunesVanishedRoots(t *testing.T) {
	env := sharedState(t)
	rootA, nestedA := project(t, "label: alpha\npurpose: erstes Projekt\n")
	rootB, _ := project(t, "label: beta\n")

	// register from a nested directory records the discovered root, and
	// register with explicit paths covers trees a shell never enters.
	if r := runLimen(t, nestedA, env, "register"); !strings.Contains(r.stdout, rootA) {
		t.Fatalf("register did not record the root: %s%s", r.stdout, r.stderr)
	}
	if r := runLimen(t, tempDir(t), env, "register", rootB); !strings.Contains(r.stdout, rootB) {
		t.Fatalf("register by path failed: %s%s", r.stdout, r.stderr)
	}

	r := runLimen(t, tempDir(t), env, "list")
	for _, want := range []string{"alpha", "beta", rootA, rootB, "erstes Projekt"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("list missing %q:\n%s", want, r.stdout)
		}
	}

	// A vanished descriptor must disappear from the listing and from the
	// registry file itself — the register describes the machine as it is.
	if err := os.Remove(filepath.Join(rootB, ".limen", "limen.yaml")); err != nil {
		t.Fatal(err)
	}
	r = runLimen(t, tempDir(t), env, "list")
	if strings.Contains(r.stdout, "beta") {
		t.Errorf("vanished context still listed:\n%s", r.stdout)
	}
	stateDir := strings.TrimPrefix(env[0], "XDG_STATE_HOME=")
	body, err := os.ReadFile(filepath.Join(stateDir, "limen", "roots"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), rootB) {
		t.Errorf("registry file still carries the vanished root:\n%s", body)
	}
	if !strings.Contains(string(body), rootA) {
		t.Errorf("pruning dropped a live root:\n%s", body)
	}
}

func TestCLIRegisterWithoutContextFails(t *testing.T) {
	if r := runLimen(t, tempDir(t), nil, "register"); r.code == 0 {
		t.Error("register without a context should fail rather than record nothing")
	}
}

func TestCLIListWithEmptyRegistryIsSafe(t *testing.T) {
	dir := tempDir(t)
	if r := runLimen(t, dir, nil, "list", "--json"); r.code != 0 || strings.TrimSpace(r.stdout) != "[]" {
		t.Errorf("empty list --json: exit %d out %q, want 0 and []", r.code, r.stdout)
	}
	if r := runLimen(t, dir, nil, "list"); r.code != 0 || !strings.Contains(r.stdout, "limen register") {
		t.Errorf("empty list should point at register: exit %d\n%s", r.code, r.stdout)
	}
}
