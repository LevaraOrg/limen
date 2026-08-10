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

// Migrate writes a .limen.yaml for one directory.
//
// Two sources, in order: an existing .orca/ tree, whose values are carried over
// verbatim, or — when there is none — the directory name as label and nothing
// else. Nothing is invented: an empty `model` is better than a guessed one,
// because the status line would then state something untrue. Even the remote
// owner only appears as a commented hint; see remoteOwner.
func Migrate(w io.Writer, dir string, dryRun bool) MigrateResult {
	res := MigrateResult{Dir: dir}
	target := filepath.Join(dir, ".limen.yaml")

	if _, err := os.Stat(target); err == nil {
		res.Action = "skipped"
		res.Reason = ".limen.yaml existiert bereits"
		return res
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
		b.WriteString("# limen — übernommen aus .orca/ am Stand der Migration.\n")
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
			res.Warning = "apiKey NICHT übernommen — mit `limen keychain-import` in den Schlüsselbund verschieben, dann aus .orca/config.yaml löschen"
		}
	} else {
		b.WriteString("# limen — neu angelegt. Nur das Label ist gesetzt; alles andere\n")
		b.WriteString("# müsste geraten werden und stünde dann falsch in der Statuszeile.\n")
		line("label", filepath.Base(dir))
		b.WriteString("actor:\nprovider:\nmodel:\n")
		if owner := remoteOwner(dir); owner != "" {
			// A hint, not a value. See remoteOwner for why.
			fmt.Fprintf(&b, "\n# githubUser:   # origin gehört \"%s\" — Organisation oder\n"+
				"#               # anderes Konto? Bitte selbst eintragen.\n", owner)
		} else {
			b.WriteString("githubUser:\n")
		}
	}
	b.WriteString("\n# Zeigt ANTHROPIC_BASE_URL auf ein lokales Nuncio. Leer = echte API.\ngateway:\n")

	res.Fields = fields
	if dryRun {
		res.Action = "would-write"
		return res
	}
	if err := os.WriteFile(target, []byte(b.String()), 0o644); err != nil {
		res.Action = "skipped"
		res.Reason = err.Error()
		return res
	}
	res.Action = "written"
	if err := ignoreLimenFile(io.Discard, dir); err != nil {
		res.Warning = strings.TrimSpace(res.Warning + " .gitignore nicht angepasst: " + err.Error())
	}
	return res
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
			fmt.Fprintf(w, "  ?  %s: kein Verzeichnis\n", dir)
			continue
		}
		r := Migrate(w, abs, dryRun)
		switch r.Action {
		case "written":
			written++
			fmt.Fprintf(w, "  +  %-32s %d Felder\n", filepath.Base(abs), r.Fields)
		case "would-write":
			written++
			fmt.Fprintf(w, "  ~  %-32s %d Felder (dry-run)\n", filepath.Base(abs), r.Fields)
		default:
			skipped++
			fmt.Fprintf(w, "  =  %-32s %s\n", filepath.Base(abs), r.Reason)
		}
		if r.Warning != "" {
			fmt.Fprintf(w, "     ! %s\n", r.Warning)
		}
	}

	verb := "geschrieben"
	if dryRun {
		verb = "würden geschrieben"
	}
	fmt.Fprintf(w, "\n%d %s, %d übersprungen\n", written, verb, skipped)
	return nil
}
