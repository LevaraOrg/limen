package main

// Profiles are inherited norms: "documentation is English", "work test-first",
// "spend tokens sparingly". They are decided once and apply to many projects,
// which is exactly the shape limen already handles for identity — so it
// handles them the same way, and for the same reason.
//
// The rule is the one service.go established: limen binds and verifies, it
// does not store. A norm's text lives in an Agent Plugins package
// (agent-plugins.org, spec 1.0.0) — a directory with a plugin.json, skills/,
// and here also adr/. That format was chosen unchanged rather than adapted:
// its whole value is that Codex, Cursor, Copilot, Kiro and VS Code read it too,
// and a private dialect would trade that away for nothing. What limen adds is
// the two things the spec deliberately leaves to the client: which project a
// package applies to, and whether the copy in the project is still the copy
// that was approved.
//
// Nothing here runs from the shell hook. `limen shell` costs 5.6 ms because it
// touches no network and copies no files; a profile is materialised by an
// explicit verb, never by crossing a threshold.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Plugin is the subset of plugin.json limen needs. The spec makes only
// $schema and name required; everything read here is optional and tolerated
// when absent, because a package that omits a version is still a package.
type Plugin struct {
	Schema      string `json:"$schema"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Repository  string `json:"repository"`
	// Extensions carries client-specific data under reverse-DNS namespaces.
	// limen's own is org.levara.limen; it is passed through untouched so a
	// package stays readable by clients that have never heard of limen.
	Extensions map[string]json.RawMessage `json:"extensions"`
}

// readPlugin loads the manifest of an Agent Plugins package.
func readPlugin(dir string) (*Plugin, error) {
	body, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return nil, fmt.Errorf("kein Agent-Plugins-Paket (plugin.json fehlt in %s)", dir)
	}
	var p Plugin
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("plugin.json in %s ist kein gültiges JSON: %v", dir, err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("plugin.json in %s hat kein name-Feld", dir)
	}
	return &p, nil
}

// versionMatches decides whether an installed version satisfies what the
// project pinned. An empty constraint takes anything. Otherwise it is a
// prefix match on dot boundaries, so `1` covers 1.4.0 but not 10.0.0 —
// treating a bare digit as a string prefix would silently accept a major
// version nobody asked for.
func versionMatches(constraint, actual string) bool {
	if constraint == "" {
		return true
	}
	if constraint == actual {
		return true
	}
	return strings.HasPrefix(actual, constraint+".")
}

// ------------------------------------------------------------------- store

// profileStorePath is where installed packages live: machine state under
// XDG_DATA_HOME, alongside the registry's roots file in spirit. Data, not
// config — it is a cache of things fetched from elsewhere.
func profileStorePath() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "limen", "profiles"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "limen", "profiles"), nil
}

// installedProfile loads a package from the store by name.
func installedProfile(name string) (dir string, p *Plugin, err error) {
	store, err := profileStorePath()
	if err != nil {
		return "", nil, err
	}
	dir = filepath.Join(store, name)
	p, err = readPlugin(dir)
	if err != nil {
		return "", nil, fmt.Errorf("Profil %q ist nicht installiert — limen profile install <pfad|git-url>", name)
	}
	return dir, p, nil
}

// installedProfiles lists what the store holds, sorted by name.
func installedProfiles() ([]*Plugin, error) {
	store, err := profileStorePath()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(store)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := []*Plugin{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := readPlugin(filepath.Join(store, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// --------------------------------------------------------------- the lock

// Lock records what was materialised, so drift is provable rather than
// assumed. It is committed: the question it answers — does this checkout still
// carry the norms it claims to — belongs to the repository, not to a machine.
type Lock struct {
	Version  int                  `json:"version"`
	Profiles map[string]LockEntry `json:"profiles"`
}

// LockEntry pins one profile: the version that was materialised, where it came
// from, and a hash per written file. Source comes from the package's own
// `repository` field, so limen keeps no second registry of origins that could
// disagree with the manifest.
type LockEntry struct {
	Version string            `json:"version"`
	Source  string            `json:"source"`
	Files   map[string]string `json:"files"`
}

func lockPath(c *Context) string {
	return filepath.Join(c.Root, ".limen", "profiles.lock")
}

func readLock(c *Context) (*Lock, error) {
	body, err := os.ReadFile(lockPath(c))
	if os.IsNotExist(err) {
		return &Lock{Version: 1, Profiles: map[string]LockEntry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var l Lock
	if err := json.Unmarshal(body, &l); err != nil {
		return nil, fmt.Errorf("%s ist kein gültiges JSON: %v", lockPath(c), err)
	}
	if l.Profiles == nil {
		l.Profiles = map[string]LockEntry{}
	}
	return &l, nil
}

func writeLock(c *Context, l *Lock) error {
	// Indented and newline-terminated because this file is reviewed in diffs.
	// Go marshals map keys in sorted order, so the output is stable and a
	// re-sync that changed nothing produces no diff at all.
	body, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath(c)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(lockPath(c), append(body, '\n'), 0o644)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// ----------------------------------------------------------- what a package carries

// payload is one file a profile contributes: where it sits in the package, and
// where it lands in the project.
type payload struct {
	from string // absolute, in the store
	to   string // relative to the context root
}

// collect walks the two directories a profile can carry. skills/ is the Agent
// Plugins location and is copied whole, subdirectories included. adr/ is not
// part of the spec — the spec permits other files — and holds the decision
// records the skills enforce: the skill is how the agent behaves, the ADR is
// why, and a project that inherits a rule should be able to read its reasoning
// without fetching anything.
func collect(dir string, c *Context) ([]payload, error) {
	out := []payload{}
	pairs := []struct{ sub, target string }{
		{"skills", c.SkillTarget()},
		{"adr", c.ADRTarget()},
	}
	for _, pair := range pairs {
		base := filepath.Join(dir, pair.sub)
		if st, err := os.Stat(base); err != nil || !st.IsDir() {
			continue
		}
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			out = append(out, payload{from: path, to: filepath.Join(pair.target, rel)})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].to < out[j].to })
	return out, nil
}

func copyFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	body, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, body, 0o644)
}

// copyTree duplicates a package into the store, leaving .git behind: what is
// installed is the package, not the checkout it arrived in.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyFile(path, filepath.Join(dst, rel))
	})
}

// ---------------------------------------------------------------- commands

// CmdProfile dispatches the profile verbs. It returns an exit code as well as
// an error because `check` has a third outcome: it ran fine and found drift,
// which must fail a pre-commit hook without looking like a crash.
func CmdProfile(w io.Writer, c *Context, args []string) (int, error) {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "status":
		return 0, profileStatus(w, c)
	case "list":
		return 0, profileStoreList(w)
	case "install":
		return 0, profileInstall(w, args[1:])
	case "sync":
		dry := false
		for _, a := range args[1:] {
			if a == "--dry-run" || a == "-n" {
				dry = true
			}
		}
		return 0, profileSync(w, c, dry)
	case "check":
		return profileCheck(w, c)
	default:
		return 2, fmt.Errorf("unbekannter Unterbefehl %q (status, list, install, sync, check)", sub)
	}
}

func needContext(c *Context) error {
	if c == nil {
		return fmt.Errorf("kein Kontext gefunden — limen init")
	}
	return nil
}

// profileStatus answers the everyday question: what applies here, and is it
// current? It never writes.
func profileStatus(w io.Writer, c *Context) error {
	if err := needContext(c); err != nil {
		return err
	}
	declared := c.ProfileList()
	if len(declared) == 0 {
		fmt.Fprintln(w, "Keine Profile deklariert.")
		fmt.Fprintf(w, "Eintragen in %s:  profiles: levara-baseline@1.0.0\n",
			filepath.Join(".limen", "meta.yaml"))
		return nil
	}
	lock, err := readLock(c)
	if err != nil {
		return err
	}
	for _, p := range declared {
		entry, synced := lock.Profiles[p.Name]
		switch {
		case !synced:
			fmt.Fprintf(w, "%-24s deklariert, nicht materialisiert — limen profile sync\n", p.String())
		default:
			drift := driftIn(c, entry)
			if len(drift) == 0 {
				fmt.Fprintf(w, "%-24s aktuell (%d Dateien, %s)\n", p.String(), len(entry.Files), entry.Version)
			} else {
				fmt.Fprintf(w, "%-24s %d Datei(en) abgewichen — limen profile check\n", p.String(), len(drift))
			}
		}
	}
	return nil
}

func profileStoreList(w io.Writer) error {
	plugins, err := installedProfiles()
	if err != nil {
		return err
	}
	if len(plugins) == 0 {
		store, _ := profileStorePath()
		fmt.Fprintf(w, "Kein Profil installiert (%s ist leer).\n", store)
		fmt.Fprintln(w, "Holen:  limen profile install <pfad|git-url>")
		return nil
	}
	for _, p := range plugins {
		version := p.Version
		if version == "" {
			version = "—"
		}
		fmt.Fprintf(w, "%-24s %-10s %s\n", p.Name, version, p.Description)
	}
	return nil
}

// profileInstall puts a package into the store, from a local directory or a
// git remote. git is shelled out to rather than vendored, the same call that
// keychain.go makes to security(1): a context tool has no business carrying a
// git implementation.
func profileInstall(w io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("limen profile install <pfad|git-url>")
	}
	src := args[0]

	dir := src
	if isRemote(src) {
		tmp, err := os.MkdirTemp("", "limen-profile-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		clone := filepath.Join(tmp, "pkg")
		cmd := exec.Command("git", "clone", "--depth", "1", "--quiet", src, clone)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git clone %s: %v: %s", src, err, strings.TrimSpace(string(out)))
		}
		dir = clone
	} else {
		abs, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		dir = abs
	}

	p, err := readPlugin(dir)
	if err != nil {
		return err
	}
	store, err := profileStorePath()
	if err != nil {
		return err
	}
	target := filepath.Join(store, p.Name)
	// Replaced wholesale rather than merged: a file the new version dropped
	// must not survive in the store, or `sync` would keep handing it out.
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := copyTree(dir, target); err != nil {
		return err
	}
	version := p.Version
	if version == "" {
		version = "ohne Version"
	}
	fmt.Fprintf(w, "installiert: %s %s → %s\n", p.Name, version, target)
	return nil
}

func isRemote(src string) bool {
	return strings.Contains(src, "://") || strings.HasPrefix(src, "git@")
}

// profileSync materialises every declared profile into the project and records
// what it wrote. Files a profile no longer carries are removed, because a
// withdrawn norm that stays on disk is worse than one that was never there:
// the agent goes on obeying a rule nobody holds.
func profileSync(w io.Writer, c *Context, dry bool) error {
	if err := needContext(c); err != nil {
		return err
	}
	declared := c.ProfileList()
	if len(declared) == 0 {
		fmt.Fprintln(w, "Keine Profile deklariert — nichts zu tun.")
		return nil
	}
	lock, err := readLock(c)
	if err != nil {
		return err
	}
	next := &Lock{Version: 1, Profiles: map[string]LockEntry{}}

	for _, p := range declared {
		dir, plugin, err := installedProfile(p.Name)
		if err != nil {
			return err
		}
		if !versionMatches(p.Version, plugin.Version) {
			return fmt.Errorf("Profil %s: installiert ist %s, deklariert %s — limen profile install <quelle>",
				p.Name, orDash(plugin.Version), p.Version)
		}
		items, err := collect(dir, c)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return fmt.Errorf("Profil %s trägt weder skills/ noch adr/", p.Name)
		}

		entry := LockEntry{Version: plugin.Version, Source: plugin.Repository, Files: map[string]string{}}
		if entry.Source == "" {
			entry.Source = dir
		}
		for _, item := range items {
			if dry {
				fmt.Fprintf(w, "würde schreiben: %s\n", item.to)
				continue
			}
			abs := filepath.Join(c.Root, item.to)
			if err := copyFile(item.from, abs); err != nil {
				return err
			}
			sum, err := hashFile(abs)
			if err != nil {
				return err
			}
			entry.Files[item.to] = sum
		}
		next.Profiles[p.Name] = entry

		// Anything the previous lock listed for this profile and the new one
		// does not is a file the profile has withdrawn.
		for rel := range lock.Profiles[p.Name].Files {
			if _, still := entry.Files[rel]; still {
				continue
			}
			if dry {
				fmt.Fprintf(w, "würde entfernen: %s\n", rel)
				continue
			}
			os.Remove(filepath.Join(c.Root, rel))
			pruneEmptyDirs(c.Root, filepath.Dir(rel))
			fmt.Fprintf(w, "entfernt: %s\n", rel)
		}
	}

	// A profile dropped from meta.yaml takes its files with it.
	for name, entry := range lock.Profiles {
		if _, still := next.Profiles[name]; still {
			continue
		}
		for rel := range entry.Files {
			if dry {
				fmt.Fprintf(w, "würde entfernen: %s (Profil %s nicht mehr deklariert)\n", rel, name)
				continue
			}
			os.Remove(filepath.Join(c.Root, rel))
			pruneEmptyDirs(c.Root, filepath.Dir(rel))
		}
		if !dry {
			fmt.Fprintf(w, "Profil %s entfernt (nicht mehr in meta.yaml)\n", name)
		}
	}

	if dry {
		fmt.Fprintln(w, "\n--dry-run: nichts geschrieben.")
		return nil
	}
	if err := writeLock(c, next); err != nil {
		return err
	}
	count := 0
	for _, e := range next.Profiles {
		count += len(e.Files)
	}
	fmt.Fprintf(w, "%d Datei(en) aus %d Profil(en) materialisiert, %s geschrieben.\n",
		count, len(next.Profiles), filepath.Join(".limen", "profiles.lock"))
	return nil
}

// pruneEmptyDirs walks back up removing directories the removal emptied,
// stopping at the root. Leaving an empty .claude/skills/tdd/ behind would
// suggest a skill is installed when it is not.
func pruneEmptyDirs(root, rel string) {
	for rel != "." && rel != "" && rel != string(filepath.Separator) {
		dir := filepath.Join(root, rel)
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
		rel = filepath.Dir(rel)
	}
}

// driftIn returns the files of one locked profile that no longer match.
func driftIn(c *Context, entry LockEntry) []string {
	out := []string{}
	for rel, want := range entry.Files {
		got, err := hashFile(filepath.Join(c.Root, rel))
		if err != nil || got != want {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

// profileCheck proves the project still carries what it claims. Exit 1 on
// drift so it can stand in a pre-commit hook or a CI step; the point is that
// an edited norm is caught by a machine rather than by whoever happens to
// reread the file.
func profileCheck(w io.Writer, c *Context) (int, error) {
	if err := needContext(c); err != nil {
		return 1, err
	}
	declared := c.ProfileList()
	if len(declared) == 0 {
		return 0, nil
	}
	lock, err := readLock(c)
	if err != nil {
		return 1, err
	}
	bad := 0
	for _, p := range declared {
		entry, synced := lock.Profiles[p.Name]
		if !synced {
			fmt.Fprintf(w, "%s: deklariert, aber nie materialisiert — limen profile sync\n", p.String())
			bad++
			continue
		}
		if !versionMatches(p.Version, entry.Version) {
			fmt.Fprintf(w, "%s: materialisiert ist %s — limen profile sync\n", p.String(), orDash(entry.Version))
			bad++
		}
		for _, rel := range driftIn(c, entry) {
			if _, err := os.Stat(filepath.Join(c.Root, rel)); os.IsNotExist(err) {
				fmt.Fprintf(w, "%s: fehlt — %s\n", rel, p.Name)
			} else {
				fmt.Fprintf(w, "%s: verändert gegenüber %s\n", rel, p.Name)
			}
			bad++
		}
	}
	// A profile in the lock that meta.yaml no longer declares is drift too:
	// its files are still on disk, still being read by an agent.
	for name := range lock.Profiles {
		found := false
		for _, p := range declared {
			if p.Name == name {
				found = true
			}
		}
		if !found {
			fmt.Fprintf(w, "%s: materialisiert, aber nicht mehr deklariert — limen profile sync\n", name)
			bad++
		}
	}
	if bad > 0 {
		fmt.Fprintf(w, "\n%d Abweichung(en).\n", bad)
		return 1, nil
	}
	fmt.Fprintf(w, "%d Profil(e) unverändert.\n", len(declared))
	return 0, nil
}

func orDash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}
