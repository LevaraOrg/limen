package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateWithoutASourceWritesOnlyALabel(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if r := Migrate(&out, dir, false); r.Action != "written" {
		t.Fatalf("action = %q", r.Action)
	}
	body, _ := os.ReadFile(filepath.Join(dir, ".limen", "limen.yaml"))
	got := string(body)
	if !strings.Contains(got, "label: "+filepath.Base(dir)) {
		t.Errorf("label missing:\n%s", got)
	}
	for _, forbidden := range []string{"provider: anthropic", "model: claude"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("invented %q:\n%s", forbidden, got)
		}
	}
}

func TestMigrateOffersTheRemoteOwnerOnlyAsAComment(t *testing.T) {
	// Checked against five projects with a known githubUser, the remote owner was
	// wrong in four: organisations, per-project accounts, and a fork's upstream.
	// So it must never land as a value.
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", "https://github.com/LevaraOrg/example.git"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if err := cmd.Run(); err != nil {
			t.Skipf("git unavailable: %v", err)
		}
	}

	var out bytes.Buffer
	Migrate(&out, dir, false)
	body, _ := os.ReadFile(filepath.Join(dir, ".limen", "limen.yaml"))
	got := string(body)

	if strings.Contains(got, "\ngithubUser: LevaraOrg") {
		t.Error("the remote owner was written as a value")
	}
	if !strings.Contains(got, "LevaraOrg") {
		t.Errorf("the hint should still mention it:\n%s", got)
	}
	// And the parser must not pick the commented line up as a field.
	ctx, ok := Discover(dir)
	if !ok {
		t.Fatal("no context after migrating")
	}
	if ctx.GithubUser != "" {
		t.Errorf("GithubUser = %q, want empty — the hint leaked into the value", ctx.GithubUser)
	}
}

func TestMigrateIsIdempotentAndNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".limen", "limen.yaml"), "label: handgeschrieben\n")

	var out bytes.Buffer
	r := Migrate(&out, dir, false)
	if r.Action != "skipped" {
		t.Fatalf("action = %q, want skipped", r.Action)
	}
	body, _ := os.ReadFile(filepath.Join(dir, ".limen", "limen.yaml"))
	if !strings.Contains(string(body), "handgeschrieben") {
		t.Error("an existing file was overwritten")
	}
}

func TestMigrateLiftsTheFlatLayoutIntoLimenDir(t *testing.T) {
	// A layout migration moves files verbatim: descriptor, notes and meta
	// travel together, the ignore entry is retargeted, nothing is edited.
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".limen.yaml"), "label: handgeschrieben\npurpose: bleibt erhalten\n")
	write(t, filepath.Join(dir, "LIMEN.md"), "# Notizen\n\n## 2026-08-11\n- alter Eintrag\n")
	write(t, filepath.Join(dir, "LIMEN-META.yaml"), "bounded_context:\n  name: handgeschrieben\n")
	write(t, filepath.Join(dir, ".gitignore"), "target/\n.limen.yaml\n")

	var out bytes.Buffer
	r := Migrate(&out, dir, false)
	if r.Action != "written" {
		t.Fatalf("action = %q, reason %q", r.Action, r.Reason)
	}
	for path, want := range map[string]string{
		filepath.Join(".limen", "limen.yaml"): "bleibt erhalten",
		filepath.Join(".limen", "notes.md"):   "alter Eintrag",
		filepath.Join(".limen", "meta.yaml"):  "bounded_context",
	} {
		body, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || !strings.Contains(string(body), want) {
			t.Errorf("%s: err=%v, missing %q:\n%s", path, err, want, body)
		}
	}
	for _, gone := range []string{".limen.yaml", "LIMEN.md", "LIMEN-META.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s should have moved away", gone)
		}
	}

	// The ignore entry follows the descriptor instead of being duplicated.
	body, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(body), ".limen/limen.yaml") {
		t.Errorf("ignore entry not retargeted:\n%s", body)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == ".limen.yaml" {
			t.Errorf("stale flat ignore entry survived:\n%s", body)
		}
	}

	// Idempotent, and the lifted context reads back unchanged.
	if r2 := Migrate(&out, dir, false); r2.Action != "skipped" {
		t.Errorf("second run: action = %q, want skipped", r2.Action)
	}
	ctx, ok := Discover(dir)
	if !ok || ctx.Label != "handgeschrieben" || ctx.Purpose != "bleibt erhalten" {
		t.Fatalf("lifted context reads differently: %+v", ctx)
	}
	if ctx.NotesFile() != filepath.Join(dir, ".limen", "notes.md") {
		t.Errorf("NotesFile = %q", ctx.NotesFile())
	}
}

func TestMigrateDryRunLiftsNothing(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".limen.yaml"), "label: x\n")
	write(t, filepath.Join(dir, "LIMEN.md"), "- eintrag\n")

	var out bytes.Buffer
	if r := Migrate(&out, dir, true); r.Action != "would-write" {
		t.Fatalf("action = %q", r.Action)
	}
	if _, err := os.Stat(filepath.Join(dir, ".limen.yaml")); err != nil {
		t.Error("dry run moved the descriptor")
	}
	if _, err := os.Stat(filepath.Join(dir, "LIMEN.md")); err != nil {
		t.Error("dry run moved the notes")
	}
}

func TestMigrateDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()

	var out bytes.Buffer
	r := Migrate(&out, dir, true)
	if r.Action != "would-write" {
		t.Fatalf("action = %q", r.Action)
	}
	if _, err := os.Stat(filepath.Join(dir, ".limen")); !os.IsNotExist(err) {
		t.Error("dry run created a file")
	}
}

func TestMigrateAddsTheGitignoreEntry(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".gitignore"), "target/\n")

	var out bytes.Buffer
	Migrate(&out, dir, false)
	body, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(body), ".limen/limen.yaml") {
		t.Errorf("no ignore entry:\n%s", body)
	}
	if !strings.Contains(string(body), "target/") {
		t.Error("existing content lost")
	}
}

func TestMigrateUsesGitInfoExcludeWhenThereIsNoGitignore(t *testing.T) {
	// Creating a .gitignore would add a tracked file to the repository in order
	// to hide an untracked one. .git/info/exclude achieves the same and stays out
	// of the repository content.
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	var out bytes.Buffer
	Migrate(&out, dir, false)

	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Error("a .gitignore was created; it would become a tracked file")
	}
	body, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("no exclude file: %v", err)
	}
	if !strings.Contains(string(body), ".limen/limen.yaml") {
		t.Errorf("no entry:\n%s", body)
	}

	// git itself must agree — the entry is worthless if it does not match.
	if err := exec.Command("git", "-C", dir, "check-ignore", "-q", ".limen/limen.yaml").Run(); err != nil {
		t.Error("git does not consider .limen/limen.yaml ignored")
	}
}

func TestMigrateOutsideAGitRepoStillWritesTheFile(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if r := Migrate(&out, dir, false); r.Action != "written" || r.Warning != "" {
		t.Fatalf("action=%q warning=%q", r.Action, r.Warning)
	}
}

func TestCLIMigrateReportsEachDirectoryAndASummary(t *testing.T) {
	fresh := tempDir(t)
	already, _ := project(t, "label: done\n")
	missing := filepath.Join(tempDir(t), "nope")

	r := runLimen(t, tempDir(t), nil, "migrate", fresh, already, missing)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{"  +  ", "  =  ", "not a directory", "1 written, 1 skipped"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("migrate output missing %q:\n%s", want, r.stdout)
		}
	}
	if _, err := os.Stat(filepath.Join(fresh, ".limen", "limen.yaml")); err != nil {
		t.Error("migrate did not write the descriptor:", err)
	}
}

func TestCLIMigrateDryRunAnnouncesButWritesNothing(t *testing.T) {
	fresh := tempDir(t)

	r := runLimen(t, tempDir(t), nil, "migrate", "--dry-run", fresh)
	if r.code != 0 || !strings.Contains(r.stdout, "would be written") {
		t.Errorf("dry-run: exit %d\n%s", r.code, r.stdout)
	}
	if _, err := os.Stat(filepath.Join(fresh, ".limen", "limen.yaml")); err == nil {
		t.Error("dry-run wrote the descriptor anyway")
	}
}
