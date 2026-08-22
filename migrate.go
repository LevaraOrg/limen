package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// MigrateResult says what happened to one directory, so the caller can summarise
// a run over many without re-deriving anything.
type MigrateResult struct {
	Dir     string
	Action  string // "written", "skipped", "would-write"
	Reason  string
	Fields  int
	Warning string
}

var githubRemote = regexp.MustCompile(`github\.com[:/]([^/]+)/`)

// remoteOwner reads the owner out of the origin remote.
//
// Offered as a COMMENT in the generated file, never as a value. Checked against
// the five projects whose real githubUser was known from .orca/, it was wrong in
// four: the remote owner is frequently an organisation (LevaraOrg) rather than
// the account in use, the same person uses different accounts per project
// (levaraleo, tgmatthias), and for a fork it is someone else entirely (palamim).
//
// A wrong account name in the status line is worse than none, because telling a
// work checkout from a private one is the entire job of that field.
func remoteOwner(dir string) string {
	cmd := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	m := githubRemote.FindStringSubmatch(strings.TrimSpace(string(out)))
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// Migrate brings one directory to the .limen/ layout.
//
// Three sources, in order: a flat pre-0.4 .limen.yaml, which is lifted into
// .limen/ together with its LIMEN.md and LIMEN-META.yaml companions; an
// existing .orca/ tree, whose values are carried over verbatim; or — when
// there is neither — the directory name as label and nothing else. Nothing is
// invented: an empty `model` is better than a guessed one, because the status
// line would then state something untrue. Even the remote owner only appears
// as a commented hint; see remoteOwner.
func Migrate(w io.Writer, dir string, dryRun bool) MigrateResult {
	res := MigrateResult{Dir: dir}
	target := filepath.Join(dir, ".limen", "limen.yaml")

	if _, err := os.Stat(target); err == nil {
		res.Action = "skipped"
		res.Reason = ".limen/limen.yaml already exists"
		return res
	}

	if fileExists(filepath.Join(dir, ".limen.yaml")) {
		return liftFlatLayout(dir, dryRun)
	}

	// Only this directory, not an inherited one from a parent: migrating a
	// subdirectory because its parent has a context would scatter files.
	var src *Context
	if fileExists(filepath.Join(dir, ".orca", "config.yaml")) ||
		fileExists(filepath.Join(dir, ".orca", "identity.yaml")) {
		ctx := &Context{Root: dir, Source: SourceOrca}
		ctx.applyFile(filepath.Join(dir, ".orca", "identity.yaml"))
		ctx.applyFile(filepath.Join(dir, ".orca", "config.yaml"))
		ctx.finish()
		src = ctx
	}

	var b strings.Builder
	fields := 0
	line := func(key, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "%s: %s\n", key, value)
		fields++
	}

	if src != nil {
		b.WriteString("# limen — taken over from .orca/ as it stood at migration time.\n")
		line("label", src.Label)
		line("actor", src.Actor)
		line("githubUser", src.GithubUser)
		line("claudeConfigDir", src.ClaudeDir)
		line("gcloudAccount", src.GcloudAccount)
		line("gcloudProject", src.GcloudProject)
		line("provider", src.Provider)
		line("model", src.Model)
		if src.HasPlaintextKey() {
			// Deliberately not carried over: copying it would spread the problem
			// into a second file instead of moving it out of both.
			res.Warning = "apiKey NOT carried over — move it with `limen keychain-import`, then delete it from .orca/config.yaml"
		}
	} else {
		b.WriteString("# limen — newly created. Only the label is set; everything else\n")
		b.WriteString("# would have to be guessed, and would then be wrong in the status line.\n")
		line("label", filepath.Base(dir))
		b.WriteString("actor:\nprovider:\nmodel:\n")
		if owner := remoteOwner(dir); owner != "" {
			// A hint, not a value. See remoteOwner for why.
			fmt.Fprintf(&b, "\n# githubUser:   # origin belongs to \"%s\" — an organisation or\n"+
				"#               # another account? Please fill this in yourself.\n", owner)
		} else {
			b.WriteString("githubUser:\n")
		}
	}
	b.WriteString("\n# Points ANTHROPIC_BASE_URL at a local Nuncio. Empty = the real API.\ngateway:\n")

	res.Fields = fields
	if dryRun {
		res.Action = "would-write"
		return res
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		res.Action = "skipped"
		res.Reason = err.Error()
		return res
	}
	if err := os.WriteFile(target, []byte(b.String()), 0o644); err != nil {
		res.Action = "skipped"
		res.Reason = err.Error()
		return res
	}
	res.Action = "written"
	if err := ignoreLimenFile(io.Discard, dir); err != nil {
		res.Warning = strings.TrimSpace(res.Warning + " .gitignore not adjusted: " + err.Error())
	}
	return res
}

// liftFlatLayout moves a pre-0.4 flat layout into .limen/: the descriptor, the
// notes and the meta file travel together, and the ignore entry is retargeted.
// Contents are moved verbatim — a layout migration must not edit anything.
func liftFlatLayout(dir string, dryRun bool) MigrateResult {
	res := MigrateResult{Dir: dir}
	moves := [][2]string{{".limen.yaml", filepath.Join(".limen", "limen.yaml")}}
	if fileExists(filepath.Join(dir, "LIMEN.md")) {
		moves = append(moves, [2]string{"LIMEN.md", filepath.Join(".limen", "notes.md")})
	}
	if fileExists(filepath.Join(dir, "LIMEN-META.yaml")) {
		moves = append(moves, [2]string{"LIMEN-META.yaml", filepath.Join(".limen", "meta.yaml")})
	}

	names := make([]string, len(moves))
	for i, m := range moves {
		names[i] = m[0]
	}
	res.Reason = "lifted into .limen/: " + strings.Join(names, ", ")
	res.Fields = len(moves)
	if dryRun {
		res.Action = "would-write"
		return res
	}

	if err := os.MkdirAll(filepath.Join(dir, ".limen"), 0o755); err != nil {
		res.Action = "skipped"
		res.Reason = err.Error()
		return res
	}
	for _, m := range moves {
		if err := os.Rename(filepath.Join(dir, m[0]), filepath.Join(dir, m[1])); err != nil {
			res.Action = "skipped"
			res.Reason = m[0] + ": " + err.Error()
			return res
		}
	}
	res.Action = "written"
	if warn := retargetIgnore(dir); warn != "" {
		res.Warning = warn
	}
	return res
}

// retargetIgnore rewrites the old `.limen.yaml` ignore line to the new
// descriptor path — in .gitignore and .git/info/exclude, whichever carries it.
// When neither does, the normal init path adds a fresh entry.
func retargetIgnore(dir string) (warning string) {
	replaced := false
	for _, p := range []string{
		filepath.Join(dir, ".gitignore"),
		filepath.Join(dir, ".git", "info", "exclude"),
	} {
		body, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(body), "\n")
		changed := false
		for i, l := range lines {
			if strings.TrimSpace(l) == ".limen.yaml" {
				lines[i] = ignoreEntry
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return "ignore entry not adjusted: " + err.Error()
		}
		replaced = true
	}
	if !replaced {
		if err := ignoreLimenFile(io.Discard, dir); err != nil {
			return ".gitignore not adjusted: " + err.Error()
		}
	}
	return ""
}

// CmdMigrate runs Migrate over the given directories and prints one line each.
func CmdMigrate(w io.Writer, dirs []string, dryRun bool) error {
	if len(dirs) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		dirs = []string{cwd}
	}

	written, skipped := 0, 0
	for _, dir := range dirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintf(w, "  ?  %s: %v\n", dir, err)
			continue
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			fmt.Fprintf(w, "  ?  %s: not a directory\n", dir)
			continue
		}
		r := Migrate(w, abs, dryRun)
		detail := fmt.Sprintf("%d fields", r.Fields)
		if r.Reason != "" {
			detail = r.Reason
		}
		switch r.Action {
		case "written":
			written++
			fmt.Fprintf(w, "  +  %-32s %s\n", filepath.Base(abs), detail)
		case "would-write":
			written++
			fmt.Fprintf(w, "  ~  %-32s %s (dry-run)\n", filepath.Base(abs), detail)
		default:
			skipped++
			fmt.Fprintf(w, "  =  %-32s %s\n", filepath.Base(abs), r.Reason)
		}
		if r.Warning != "" {
			fmt.Fprintf(w, "     ! %s\n", r.Warning)
		}
	}

	verb := "written"
	if dryRun {
		verb = "would be written"
	}
	fmt.Fprintf(w, "\n%d %s, %d skipped\n", written, verb, skipped)
	return nil
}
