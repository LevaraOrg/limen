package main

// CLI tests for `backlog`. Like cli_test.go these run the real binary: the
// contract with agents is stdout, files on disk, and exit codes.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIBacklogListsOpenNotesAcrossContexts(t *testing.T) {
	env := sharedState(t)
	rootA, nestedA := project(t, "label: alpha\n")
	rootB, nestedB := project(t, "label: beta\n")
	runLimen(t, nestedA, env, "register")
	runLimen(t, nestedB, env, "register")

	runLimen(t, nestedA, env, "note", "first open task")
	runLimen(t, nestedA, env, "note", "second open task")
	runLimen(t, nestedB, env, "note", "about to be checked off")
	// Ticking a line is the one sanctioned in-place edit: `- ` becomes `- ✓ `.
	notesB := filepath.Join(rootB, ".limen", "notes.md")
	body, err := os.ReadFile(notesB)
	if err != nil {
		t.Fatal(err)
	}
	write(t, notesB, strings.Replace(string(body),
		"- about to be checked off", "- ✓ about to be checked off", 1))

	r := runLimen(t, tempDir(t), env, "backlog")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	today := time.Now().Format("2006-01-02")
	for _, want := range []string{
		"alpha — 2 open", rootA,
		today + "  first open task", "second open task",
		"2 open in 1 contexts", "1 done ✓",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("backlog missing %q:\n%s", want, r.stdout)
		}
	}
	// beta has nothing open — it must not clutter the human view, and the
	// ticked line must not reappear as open work.
	for _, unwanted := range []string{"beta —", "about to be checked off"} {
		if strings.Contains(r.stdout, unwanted) {
			t.Errorf("backlog should not show %q:\n%s", unwanted, r.stdout)
		}
	}
}

func TestCLIBacklogJSONCarriesOpenAndDone(t *testing.T) {
	env := sharedState(t)
	root, nested := project(t, "label: alpha\n")
	runLimen(t, nested, env, "register")
	runLimen(t, nested, env, "note", "left open")
	write(t, filepath.Join(root, ".limen", "notes.md"),
		"# Notes\n\n## 2026-08-01\n- ✓ long since done\n\n## 2026-08-11\n- left open\n")

	r := runLimen(t, tempDir(t), env, "backlog", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{
		`"root":"` + root + `"`, `"label":"alpha"`,
		`"date":"2026-08-11"`, `"text":"left open"`, `"done":1`,
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("backlog --json missing %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "long since done") {
		t.Errorf("ticked entries must not appear as open:\n%s", r.stdout)
	}
}

func TestCLIBacklogWithNothingOpenIsSafe(t *testing.T) {
	// Empty register, and a registered context without notes: both must not
	// error — the command runs from scripts and agents unconditionally.
	dir := tempDir(t)
	if r := runLimen(t, dir, nil, "backlog", "--json"); r.code != 0 || strings.TrimSpace(r.stdout) != "[]" {
		t.Errorf("empty backlog --json: exit %d out %q, want 0 and []", r.code, r.stdout)
	}

	env := sharedState(t)
	_, nested := project(t, "label: leer\n")
	runLimen(t, nested, env, "register")
	r := runLimen(t, dir, env, "backlog")
	if r.code != 0 || !strings.Contains(r.stdout, "Nothing open") {
		t.Errorf("expected the nothing-open message: exit %d\n%s", r.code, r.stdout)
	}
}
