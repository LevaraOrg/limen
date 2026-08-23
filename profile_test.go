package main

// Tests for inherited ADR profiles. The law under test is the same one that
// service.go follows: limen binds and verifies, it does not store. A profile's
// text lives in an Agent Plugins package; what belongs to the project is the
// declaration that it applies here, and proof that the materialised copy still
// matches.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pkg lays out a minimal Agent Plugins package and returns its directory.
func pkg(t *testing.T, name, version string, skills, adrs map[string]string) string {
	t.Helper()
	dir := tempDir(t)
	manifest := `{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "` + name + `",
  "version": "` + version + `",
  "repository": "https://github.com/LevaraOrg/` + name + `"
}
`
	write(t, filepath.Join(dir, "plugin.json"), manifest)
	for path, body := range skills {
		write(t, filepath.Join(dir, "skills", path), body)
	}
	for path, body := range adrs {
		write(t, filepath.Join(dir, "adr", path), body)
	}
	return dir
}

// baseline is the package this repository actually ships: three norms.
func baseline(t *testing.T) string {
	t.Helper()
	return pkg(t, "levara-baseline", "1.0.0",
		map[string]string{
			"english-only/SKILL.md": "---\nname: english-only\n---\nEnglish.\n",
			"tdd/SKILL.md":          "---\nname: tdd\n---\nTest first.\n",
		},
		map[string]string{
			"ADR-0001-english.md": "# ADR-0001\n",
		})
}

// store points the profile store at a temporary directory so a test never
// touches the real one.
func store(t *testing.T) string {
	t.Helper()
	dir := tempDir(t)
	t.Setenv("XDG_DATA_HOME", dir)
	return filepath.Join(dir, "limen", "profiles")
}

// ---------------------------------------------------------------- meta.yaml

func TestMetaProfilesAreRead(t *testing.T) {
	root, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"),
		"profiles: levara-baseline@1.0.0, house-style\n")

	ctx, ok := Discover(root)
	if !ok {
		t.Fatal("expected a context")
	}
	got := ctx.ProfileList()
	if len(got) != 2 {
		t.Fatalf("ProfileList() = %+v, want 2 entries", got)
	}
	if got[0].Name != "levara-baseline" || got[0].Version != "1.0.0" {
		t.Errorf("first = %+v", got[0])
	}
	// A bare name is a deliberate "any version": pinning must be a choice.
	if got[1].Name != "house-style" || got[1].Version != "" {
		t.Errorf("second = %+v", got[1])
	}
}

func TestMetaCannotBleedIntoTheDescriptor(t *testing.T) {
	// meta.yaml is committed project content; limen.yaml is machine-local
	// identity. A key that exists in both must never cross over, or the
	// repository would start dictating who you are.
	root, _ := project(t, "label: tessera\nactor: Matthias\ngithubUser: levaraleo\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"),
		"label: not-tessera\nactor: Somebody Else\ngithubUser: someone\nprofiles: levara-baseline\n")

	ctx, _ := Discover(root)
	if ctx.Label != "tessera" || ctx.Actor != "Matthias" || ctx.GithubUser != "levaraleo" {
		t.Errorf("meta.yaml leaked into the descriptor: label=%q actor=%q gh=%q",
			ctx.Label, ctx.Actor, ctx.GithubUser)
	}
	if len(ctx.ProfileList()) != 1 {
		t.Errorf("ProfileList() = %+v", ctx.ProfileList())
	}
}

func TestWithoutMetaThereAreNoProfiles(t *testing.T) {
	root, _ := project(t, "label: plain\n")
	ctx, _ := Discover(root)
	got := ctx.ProfileList()
	if got == nil {
		t.Fatal("ProfileList() = nil, want an empty slice so JSON renders []")
	}
	if len(got) != 0 {
		t.Errorf("ProfileList() = %+v", got)
	}
}

func TestMetaTargetsFallBackToConventionalPlaces(t *testing.T) {
	root, _ := project(t, "label: plain\n")
	ctx, _ := Discover(root)
	if ctx.SkillTarget() != ".claude/skills" {
		t.Errorf("SkillTarget() = %q", ctx.SkillTarget())
	}
	if ctx.ADRTarget() != "docs/adr" {
		t.Errorf("ADRTarget() = %q", ctx.ADRTarget())
	}

	root2, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root2, ".limen", "meta.yaml"),
		"skillTarget: .agents/skills\nadrTarget: .planning/adr\n")
	ctx2, _ := Discover(root2)
	if ctx2.SkillTarget() != ".agents/skills" || ctx2.ADRTarget() != ".planning/adr" {
		t.Errorf("targets = %q, %q", ctx2.SkillTarget(), ctx2.ADRTarget())
	}
}

// ------------------------------------------------------------ version match

func TestVersionConstraintMatchesOnDotBoundaries(t *testing.T) {
	cases := []struct {
		constraint, actual string
		want               bool
	}{
		{"", "1.0.0", true},      // unpinned takes whatever is installed
		{"1.0.0", "1.0.0", true}, // exact
		{"1.0", "1.0.3", true},   // prefix, on a dot boundary
		{"1", "1.4.0", true},
		{"1", "10.0.0", false}, // not a prefix match on digits alone
		{"1.0.0", "1.0.1", false},
		{"2", "1.0.0", false},
	}
	for _, c := range cases {
		if got := versionMatches(c.constraint, c.actual); got != c.want {
			t.Errorf("versionMatches(%q, %q) = %v, want %v", c.constraint, c.actual, got, c.want)
		}
	}
}

// -------------------------------------------------------------- the package

func TestPluginManifestIsRead(t *testing.T) {
	dir := baseline(t)
	p, err := readPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "levara-baseline" || p.Version != "1.0.0" {
		t.Errorf("plugin = %+v", p)
	}
	if p.Repository != "https://github.com/LevaraOrg/levara-baseline" {
		t.Errorf("Repository = %q", p.Repository)
	}
}

func TestADirectoryWithoutAManifestIsNotAPackage(t *testing.T) {
	if _, err := readPlugin(tempDir(t)); err == nil {
		t.Fatal("expected an error for a directory without plugin.json")
	}
}

// ------------------------------------------------------------------ install

func TestInstallFromALocalPathPopulatesTheStore(t *testing.T) {
	dst := store(t)
	src := baseline(t)

	var out strings.Builder
	if _, err := CmdProfile(&out, nil, []string{"install", src}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "levara-baseline", "plugin.json")); err != nil {
		t.Fatalf("package not in the store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "levara-baseline", "skills", "tdd", "SKILL.md")); err != nil {
		t.Fatalf("skills not copied: %v", err)
	}
}

func TestInstallRefusesADirectoryThatIsNotAPackage(t *testing.T) {
	store(t)
	var out strings.Builder
	if _, err := CmdProfile(&out, nil, []string{"install", tempDir(t)}); err == nil {
		t.Fatal("expected a refusal")
	}
}

// --------------------------------------------------------------------- sync

// synced sets up a project that declares the baseline, installs it, and syncs.
// It returns the store as well, because store() sets an environment variable —
// calling it twice in one test would silently point at the wrong directory.
func synced(t *testing.T) (root, dst string) {
	t.Helper()
	dst = store(t)
	src := baseline(t)
	root, _ = project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "profiles: levara-baseline@1.0.0\n")

	var out strings.Builder
	if _, err := CmdProfile(&out, nil, []string{"install", src}); err != nil {
		t.Fatal(err)
	}
	ctx, _ := Discover(root)
	if _, err := CmdProfile(&out, ctx, []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	return root, dst
}

func TestSyncMaterialisesSkillsAndADRs(t *testing.T) {
	root, _ := synced(t)

	for _, rel := range []string{
		".claude/skills/english-only/SKILL.md",
		".claude/skills/tdd/SKILL.md",
		"docs/adr/ADR-0001-english.md",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("not materialised: %s (%v)", rel, err)
		}
	}
}

func TestSyncWritesALockThatNamesEveryFile(t *testing.T) {
	root, _ := synced(t)

	body, err := os.ReadFile(filepath.Join(root, ".limen", "profiles.lock"))
	if err != nil {
		t.Fatal(err)
	}
	var lock Lock
	if err := json.Unmarshal(body, &lock); err != nil {
		t.Fatal(err)
	}
	entry, ok := lock.Profiles["levara-baseline"]
	if !ok {
		t.Fatalf("lock = %+v", lock)
	}
	if entry.Version != "1.0.0" {
		t.Errorf("Version = %q", entry.Version)
	}
	// The origin is taken from the manifest, so the lock says where the norm
	// came from without limen keeping a second registry of sources.
	if entry.Source != "https://github.com/LevaraOrg/levara-baseline" {
		t.Errorf("Source = %q", entry.Source)
	}
	if len(entry.Files) != 3 {
		t.Errorf("Files = %+v, want 3", entry.Files)
	}
	for rel, sum := range entry.Files {
		if !strings.HasPrefix(sum, "sha256:") {
			t.Errorf("%s: hash = %q", rel, sum)
		}
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	root, _ := synced(t)
	first, err := os.ReadFile(filepath.Join(root, ".limen", "profiles.lock"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, _ := Discover(root)
	var out strings.Builder
	if _, err := CmdProfile(&out, ctx, []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(root, ".limen", "profiles.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("a second sync changed the lock")
	}
}

func TestSyncDryRunWritesNothing(t *testing.T) {
	store(t)
	src := baseline(t)
	root, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "profiles: levara-baseline\n")

	var out strings.Builder
	if _, err := CmdProfile(&out, nil, []string{"install", src}); err != nil {
		t.Fatal(err)
	}
	ctx, _ := Discover(root)
	if _, err := CmdProfile(&out, ctx, []string{"sync", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Error("--dry-run materialised files")
	}
	if _, err := os.Stat(filepath.Join(root, ".limen", "profiles.lock")); !os.IsNotExist(err) {
		t.Error("--dry-run wrote a lock")
	}
	if !strings.Contains(out.String(), "SKILL.md") {
		t.Errorf("--dry-run said nothing about what it would write:\n%s", out.String())
	}
}

func TestSyncRemovesFilesTheProfileNoLongerCarries(t *testing.T) {
	// A norm that was withdrawn must disappear from the project, or the agent
	// keeps obeying a rule nobody holds any more.
	root, dst := synced(t)

	stale := filepath.Join(root, ".claude", "skills", "tdd", "SKILL.md")
	if _, err := os.Stat(stale); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dst, "levara-baseline", "skills", "tdd")); err != nil {
		t.Fatal(err)
	}

	ctx, _ := Discover(root)
	var out strings.Builder
	if _, err := CmdProfile(&out, ctx, []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("a withdrawn skill was left behind in the project")
	}
}

func TestSyncFailsOnAVersionTheStoreDoesNotHold(t *testing.T) {
	store(t)
	src := baseline(t)
	root, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "profiles: levara-baseline@2.0.0\n")

	var out strings.Builder
	if _, err := CmdProfile(&out, nil, []string{"install", src}); err != nil {
		t.Fatal(err)
	}
	ctx, _ := Discover(root)
	if _, err := CmdProfile(&out, ctx, []string{"sync"}); err == nil {
		t.Fatal("expected a version mismatch to fail")
	}
}

func TestSyncFailsWhenTheProfileIsNotInstalled(t *testing.T) {
	store(t)
	root, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "profiles: nowhere-to-be-found\n")

	ctx, _ := Discover(root)
	var out strings.Builder
	if _, err := CmdProfile(&out, ctx, []string{"sync"}); err == nil {
		t.Fatal("expected a missing profile to fail")
	}
}

// -------------------------------------------------------------------- check

func TestCheckPassesRightAfterSync(t *testing.T) {
	root, _ := synced(t)
	ctx, _ := Discover(root)
	var out strings.Builder
	code, err := CmdProfile(&out, ctx, []string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit %d right after sync:\n%s", code, out.String())
	}
}

func TestCheckDetectsAnEditedFile(t *testing.T) {
	root, _ := synced(t)
	edited := filepath.Join(root, ".claude", "skills", "tdd", "SKILL.md")
	write(t, edited, "---\nname: tdd\n---\nActually, skip the tests.\n")

	ctx, _ := Discover(root)
	var out strings.Builder
	code, err := CmdProfile(&out, ctx, []string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 {
		t.Error("an edited skill went unnoticed")
	}
	if !strings.Contains(out.String(), "tdd/SKILL.md") {
		t.Errorf("the drift was not named:\n%s", out.String())
	}
}

func TestCheckDetectsADeletedFile(t *testing.T) {
	root, _ := synced(t)
	if err := os.Remove(filepath.Join(root, "docs", "adr", "ADR-0001-english.md")); err != nil {
		t.Fatal(err)
	}
	ctx, _ := Discover(root)
	var out strings.Builder
	code, _ := CmdProfile(&out, ctx, []string{"check"})
	if code == 0 {
		t.Error("a deleted ADR went unnoticed")
	}
}

func TestCheckDetectsAProfileDeclaredButNeverSynced(t *testing.T) {
	root, _ := synced(t)
	write(t, filepath.Join(root, ".limen", "meta.yaml"),
		"profiles: levara-baseline@1.0.0, house-style\n")

	ctx, _ := Discover(root)
	var out strings.Builder
	code, _ := CmdProfile(&out, ctx, []string{"check"})
	if code == 0 {
		t.Error("a declared but unsynced profile went unnoticed")
	}
	if !strings.Contains(out.String(), "house-style") {
		t.Errorf("the missing profile was not named:\n%s", out.String())
	}
}

func TestCheckWithoutProfilesIsQuietAndPasses(t *testing.T) {
	store(t)
	root, _ := project(t, "label: plain\n")
	ctx, _ := Discover(root)
	var out strings.Builder
	code, err := CmdProfile(&out, ctx, []string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit %d for a project that declares nothing", code)
	}
}

// ------------------------------------------------------------------ reports

func TestProfilesAppearInJSON(t *testing.T) {
	root, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "profiles: levara-baseline@1.0.0\n")
	ctx, _ := Discover(root)

	var out strings.Builder
	if err := RenderJSON(&out, ctx, fixedResolver{}); err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out.String()), &v); err != nil {
		t.Fatal(err)
	}
	list, ok := v["profiles"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("profiles = %v", v["profiles"])
	}
	first := list[0].(map[string]any)
	if first["name"] != "levara-baseline" || first["version"] != "1.0.0" {
		t.Errorf("profiles[0] = %v", first)
	}
}

func TestJSONCarriesAnEmptyProfileListRatherThanNull(t *testing.T) {
	root, _ := project(t, "label: plain\n")
	ctx, _ := Discover(root)
	var out strings.Builder
	if err := RenderJSON(&out, ctx, fixedResolver{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"profiles":[]`) {
		t.Errorf("json = %s", out.String())
	}
}

func TestShowNamesTheProfiles(t *testing.T) {
	root, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "profiles: levara-baseline@1.0.0\n")
	ctx, _ := Discover(root)

	var out strings.Builder
	RenderShow(&out, ctx, fixedResolver{})
	if !strings.Contains(out.String(), "levara-baseline@1.0.0") {
		t.Errorf("show = %s", out.String())
	}
}

// --------------------------------------------------- several profiles at once

// The per-project question is not only "which norms" but "which of them here".
// A project declares the packages it wants; dropping one from meta.yaml has to
// take its files with it while leaving the others untouched.

func TestSeveralProfilesAreMaterialisedSideBySide(t *testing.T) {
	store(t)
	base := baseline(t)
	extra := pkg(t, "house-style", "2.1.0",
		map[string]string{"caveman/SKILL.md": "---\nname: caveman\n---\nUgh.\n"}, nil)

	root, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"),
		"profiles: levara-baseline@1.0.0, house-style@2\n")

	var out strings.Builder
	for _, src := range []string{base, extra} {
		if _, err := CmdProfile(&out, nil, []string{"install", src}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, _ := Discover(root)
	if _, err := CmdProfile(&out, ctx, []string{"sync"}); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		".claude/skills/tdd/SKILL.md",     // from levara-baseline
		".claude/skills/caveman/SKILL.md", // from house-style
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("not materialised: %s (%v)", rel, err)
		}
	}

	lock, err := readLock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Profiles) != 2 {
		t.Fatalf("lock holds %d profiles, want 2", len(lock.Profiles))
	}
	// The pin was `@2` against an installed 2.1.0 — a prefix match on a dot
	// boundary, which is what lets a project follow a minor release without
	// editing meta.yaml.
	if lock.Profiles["house-style"].Version != "2.1.0" {
		t.Errorf("house-style version = %q", lock.Profiles["house-style"].Version)
	}
}

func TestDroppingOneProfileLeavesTheOthersAlone(t *testing.T) {
	store(t)
	base := baseline(t)
	extra := pkg(t, "house-style", "2.1.0",
		map[string]string{"caveman/SKILL.md": "---\nname: caveman\n---\nUgh.\n"}, nil)

	root, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"),
		"profiles: levara-baseline@1.0.0, house-style@2\n")

	var out strings.Builder
	for _, src := range []string{base, extra} {
		if _, err := CmdProfile(&out, nil, []string{"install", src}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, _ := Discover(root)
	if _, err := CmdProfile(&out, ctx, []string{"sync"}); err != nil {
		t.Fatal(err)
	}

	// Deactivate house-style by removing it from the declaration.
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "profiles: levara-baseline@1.0.0\n")
	ctx, _ = Discover(root)
	if _, err := CmdProfile(&out, ctx, []string{"sync"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "caveman")); !os.IsNotExist(err) {
		t.Error("a deactivated profile left its skill behind")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "tdd", "SKILL.md")); err != nil {
		t.Errorf("deactivating one profile removed another's files: %v", err)
	}

	lock, _ := readLock(ctx)
	if _, still := lock.Profiles["house-style"]; still {
		t.Error("the lock still lists the deactivated profile")
	}
	code, _ := CmdProfile(&out, ctx, []string{"check"})
	if code != 0 {
		t.Errorf("check reports drift after a clean deactivation:\n%s", out.String())
	}
}

// ------------------------------------------------------------ paused skills

// A project inherits a package whole but does not always want every skill in
// it. Pausing is the fine-grained half of the same decision: the package stays
// declared, one skill stops being materialised. Absence is what deactivates —
// a skill that is not in .claude/skills/ cannot be loaded — so the declaration
// exists for the human, and the file system enforces it.

// twoSkills is a package carrying two skills and one ADR.
func twoSkills(t *testing.T) string {
	t.Helper()
	return pkg(t, "matt-pocock", "1.0.0",
		map[string]string{
			"caveman/SKILL.md":  "---\nname: caveman\n---\nUgh.\n",
			"zoom-out/SKILL.md": "---\nname: zoom-out\n---\nStep back.\n",
		},
		map[string]string{"ADR-0009-house.md": "# ADR-0009\n"})
}

// pausedProject installs twoSkills and declares it, with `paused` paused.
func pausedProject(t *testing.T, paused string) (root string) {
	t.Helper()
	store(t)
	src := twoSkills(t)
	root, _ = project(t, "label: tessera\n")
	meta := "profiles: matt-pocock@1.0.0\n"
	if paused != "" {
		meta += "pausedSkills: " + paused + "\n"
	}
	write(t, filepath.Join(root, ".limen", "meta.yaml"), meta)

	var out strings.Builder
	if _, err := CmdProfile(&out, nil, []string{"install", src}); err != nil {
		t.Fatal(err)
	}
	return root
}

func syncNow(t *testing.T, root string) *Context {
	t.Helper()
	ctx, _ := Discover(root)
	var out strings.Builder
	if _, err := CmdProfile(&out, ctx, []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestPausedSkillsAreRead(t *testing.T) {
	root, _ := project(t, "label: tessera\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"),
		"pausedSkills: zoom-out, grill-me\n")
	ctx, _ := Discover(root)

	got := ctx.PausedSkillList()
	if len(got) != 2 || got[0] != "zoom-out" || got[1] != "grill-me" {
		t.Errorf("PausedSkillList() = %+v", got)
	}
}

func TestWithoutTheFieldNothingIsPaused(t *testing.T) {
	root, _ := project(t, "label: plain\n")
	ctx, _ := Discover(root)
	if got := ctx.PausedSkillList(); got == nil || len(got) != 0 {
		t.Errorf("PausedSkillList() = %+v, want an empty non-nil slice", got)
	}
}

func TestSyncDoesNotMaterialiseAPausedSkill(t *testing.T) {
	root := pausedProject(t, "zoom-out")
	ctx := syncNow(t, root)

	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "caveman", "SKILL.md")); err != nil {
		t.Errorf("an unpaused skill is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "zoom-out")); !os.IsNotExist(err) {
		t.Error("a paused skill was materialised anyway")
	}

	// The lock must not claim a file it did not write, or check would demand
	// the presence of something pausing deliberately removed.
	lock, _ := readLock(ctx)
	for rel := range lock.Profiles["matt-pocock"].Files {
		if strings.Contains(rel, "zoom-out") {
			t.Errorf("the lock lists a paused skill: %s", rel)
		}
	}
}

func TestPausingIsNotADRsuppression(t *testing.T) {
	// Pausing touches skills only. The decision record explains why a norm
	// exists and stays readable even when its enforcement is switched off.
	root := pausedProject(t, "zoom-out")
	syncNow(t, root)
	if _, err := os.Stat(filepath.Join(root, "docs", "adr", "ADR-0009-house.md")); err != nil {
		t.Errorf("pausing a skill removed an ADR: %v", err)
	}
}

func TestPausingAfterTheFactRemovesTheSkill(t *testing.T) {
	root := pausedProject(t, "")
	syncNow(t, root)
	live := filepath.Join(root, ".claude", "skills", "zoom-out", "SKILL.md")
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("setup: %v", err)
	}

	write(t, filepath.Join(root, ".limen", "meta.yaml"),
		"profiles: matt-pocock@1.0.0\npausedSkills: zoom-out\n")
	ctx := syncNow(t, root)

	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Error("pausing left the skill on disk, where an agent would still load it")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "caveman", "SKILL.md")); err != nil {
		t.Errorf("pausing one skill removed another: %v", err)
	}
	if code, _ := CmdProfile(&strings.Builder{}, ctx, []string{"check"}); code != 0 {
		t.Error("check reports drift after a clean pause")
	}
}

func TestUnpausingBringsTheSkillBack(t *testing.T) {
	root := pausedProject(t, "zoom-out")
	syncNow(t, root)

	write(t, filepath.Join(root, ".limen", "meta.yaml"), "profiles: matt-pocock@1.0.0\n")
	syncNow(t, root)

	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "zoom-out", "SKILL.md")); err != nil {
		t.Errorf("unpausing did not restore the skill: %v", err)
	}
}

func TestCheckFlagsAPausedNameThatMatchesNothing(t *testing.T) {
	// The dangerous typo: you write zoom_out, believe the skill is off, and it
	// is quietly still there. Silence would be the worst possible answer.
	root := pausedProject(t, "zoom_out")
	ctx := syncNow(t, root)

	var out strings.Builder
	code, err := CmdProfile(&out, ctx, []string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 {
		t.Error("a paused name matching no skill went unreported")
	}
	if !strings.Contains(out.String(), "zoom_out") {
		t.Errorf("the bad name was not shown:\n%s", out.String())
	}
}

func TestJSONCarriesActiveAndPausedSkills(t *testing.T) {
	root := pausedProject(t, "zoom-out")
	ctx := syncNow(t, root)

	var out strings.Builder
	if err := RenderJSON(&out, ctx, fixedResolver{}); err != nil {
		t.Fatal(err)
	}
	var v struct {
		Skills struct {
			Active []string `json:"active"`
			Paused []string `json:"paused"`
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out.String()), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Skills.Active) != 1 || v.Skills.Active[0] != "caveman" {
		t.Errorf("active = %+v", v.Skills.Active)
	}
	if len(v.Skills.Paused) != 1 || v.Skills.Paused[0] != "zoom-out" {
		t.Errorf("paused = %+v", v.Skills.Paused)
	}
}

func TestJSONSkillListsAreEmptyRatherThanNull(t *testing.T) {
	root, _ := project(t, "label: plain\n")
	ctx, _ := Discover(root)
	var out strings.Builder
	if err := RenderJSON(&out, ctx, fixedResolver{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"skills":{"active":[],"paused":[]}`) {
		t.Errorf("json = %s", out.String())
	}
}

func TestShowNamesActiveAndPausedSkills(t *testing.T) {
	root := pausedProject(t, "zoom-out")
	ctx := syncNow(t, root)

	var out strings.Builder
	RenderShow(&out, ctx, fixedResolver{})
	s := out.String()
	if !strings.Contains(s, "caveman") {
		t.Errorf("show does not name the active skill:\n%s", s)
	}
	if !strings.Contains(s, "zoom-out") || !strings.Contains(s, "paused") {
		t.Errorf("show does not name the paused skill:\n%s", s)
	}
}

func TestCheckWarnsAboutAPausedSkillItDidNotWrite(t *testing.T) {
	// limen removes only what it wrote. A skill that was already lying in the
	// project before any profile was bound stays — deleting files it never
	// owned is not limen's business. But staying silent would mean a skill you
	// declared paused goes on loading, which is the failure the declaration
	// exists to prevent. So it is reported and not removed.
	root := pausedProject(t, "zoom-out")
	stray := filepath.Join(root, ".claude", "skills", "zoom-out", "SKILL.md")
	write(t, stray, "---\nname: zoom-out\n---\nDropped here by hand.\n")
	ctx := syncNow(t, root)

	var out strings.Builder
	code, err := CmdProfile(&out, ctx, []string{"check"})
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 {
		t.Error("a paused skill still present on disk went unreported")
	}
	if !strings.Contains(out.String(), "zoom-out") {
		t.Errorf("the skill was not named:\n%s", out.String())
	}
	if _, err := os.Stat(stray); err != nil {
		t.Error("limen deleted a file it never wrote")
	}
}
