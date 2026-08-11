package main

// CLI tests for `note`. Like cli_test.go these run the real binary: the
// contract with agents is stdout, files on disk, and exit codes.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLINoteAppendsUnderOneDateHeading(t *testing.T) {
	root, nested := project(t, fullConfig)

	if r := runLimen(t, nested, nil, "note", "Kundenbedürfnisse", "pro", "Phase", "aufschreiben"); r.code != 0 {
		t.Fatalf("note exit %d: %s", r.code, r.stderr)
	}
	runLimen(t, nested, nil, "note", "zweiter Gedanke")

	body, err := os.ReadFile(filepath.Join(root, ".limen", "notes.md"))
	if err != nil {
		t.Fatal(".limen/notes.md was not created:", err)
	}
	text := string(body)
	today := time.Now().Format("2006-01-02")
	for _, want := range []string{
		"# LIMEN — rollierende Notizen zu tessera",
		"## " + today,
		"- Kundenbedürfnisse pro Phase aufschreiben",
		"- zweiter Gedanke",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("LIMEN.md missing %q:\n%s", want, text)
		}
	}
	// Two notes on the same day share one heading.
	if strings.Count(text, "## ") != 1 {
		t.Errorf("expected exactly one date heading:\n%s", text)
	}
}

func TestCLINoteOpensANewHeadingWhenTheDayChanged(t *testing.T) {
	root, nested := project(t, fullConfig)
	write(t, filepath.Join(root, ".limen", "notes.md"),
		"# LIMEN — rollierende Notizen zu tessera\n\n## 2001-01-01\n- alter Eintrag\n")

	runLimen(t, nested, nil, "note", "neuer Eintrag")
	body, _ := os.ReadFile(filepath.Join(root, ".limen", "notes.md"))
	text := string(body)
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(text, "## "+today) {
		t.Errorf("missing today's heading:\n%s", text)
	}
	// Appended, never rewritten: the old day survives above the new one.
	if !strings.Contains(text, "## 2001-01-01\n- alter Eintrag") {
		t.Errorf("existing log was rewritten:\n%s", text)
	}
}

func TestCLINoteRoutesByLabelFromAnywhere(t *testing.T) {
	root, nested := project(t, fullConfig)
	env := sharedState(t)
	runLimen(t, nested, env, "register")

	// From a directory with no context at all, case-insensitive on the label.
	if r := runLimen(t, tempDir(t), env, "note", "--at", "TESSERA", "aus der Ferne"); r.code != 0 {
		t.Fatalf("note --at exit %d: %s", r.code, r.stderr)
	}
	body, err := os.ReadFile(filepath.Join(root, ".limen", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "- aus der Ferne") {
		t.Errorf("note did not land in the target root:\n%s", body)
	}
}

func TestCLINoteFailuresNameTheWayOut(t *testing.T) {
	// No context and no --at: the error must point at --at.
	if r := runLimen(t, tempDir(t), nil, "note", "verloren"); r.code == 0 || !strings.Contains(r.stderr, "--at") {
		t.Errorf("expected a hint at --at: exit %d stderr %q", r.code, r.stderr)
	}

	// Unknown label: the error must name what is registered.
	_, nested := project(t, fullConfig)
	env := sharedState(t)
	runLimen(t, nested, env, "register")
	r := runLimen(t, nested, env, "note", "--at", "nirgendwo", "text")
	if r.code == 0 || !strings.Contains(r.stderr, "tessera") {
		t.Errorf("expected the known labels in the error: exit %d stderr %q", r.code, r.stderr)
	}

	// No text at all.
	if r := runLimen(t, nested, nil, "note"); r.code == 0 {
		t.Error("note without text should fail")
	}
}
