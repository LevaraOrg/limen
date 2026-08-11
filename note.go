package main

// .limen/notes.md is the rolling free-text companion of the descriptor. The
// YAML stays hard fact — identity, interfaces, routing — and is never appended
// to by tooling; loose thoughts ("denke bitte an …") land here instead, dated,
// in order of arrival. Unlike the descriptor the notes are project content,
// not machine-local identity, so they belong in the repository.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CmdNote appends one dated entry. Without --at it targets the context above
// the working directory; with --at it routes by label through the registry,
// so a note can be filed from anywhere — which is exactly what an agent
// sorting a voice memo needs.
func CmdNote(w io.Writer, ctx *Context, found bool, args []string, now time.Time) error {
	atLabel := ""
	words := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--at" {
			if i+1 >= len(args) {
				return fmt.Errorf("--at braucht ein Label")
			}
			atLabel = args[i+1]
			i++
			continue
		}
		words = append(words, args[i])
	}
	text := strings.TrimSpace(strings.Join(words, " "))
	if text == "" {
		return fmt.Errorf("keine Notiz angegeben:  limen note [--at label] \"text\"")
	}

	target := ctx
	if atLabel != "" {
		var err error
		if target, err = contextByLabel(atLabel); err != nil {
			return err
		}
	} else if !found {
		return fmt.Errorf("kein Kontext oberhalb des Arbeitsverzeichnisses — Ziel mit --at <label> wählen")
	}

	path := target.NotesFile()
	if err := appendNote(path, target.Label, text, now); err != nil {
		return err
	}
	fmt.Fprintf(w, "notiert in %s\n", path)
	return nil
}

// contextByLabel resolves a label through the registry. Case-insensitive,
// because the label is typed by hand here rather than read from a file.
func contextByLabel(label string) (*Context, error) {
	ctxs, err := registeredContexts()
	if err != nil {
		return nil, err
	}
	matches := []*Context{}
	known := []string{}
	for _, c := range ctxs {
		known = append(known, c.Label)
		if strings.EqualFold(c.Label, label) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		if len(known) == 0 {
			return nil, fmt.Errorf("kein Kontext %q — das Register ist leer, erst  limen register <pfad>", label)
		}
		return nil, fmt.Errorf("kein Kontext %q — registriert sind: %s", label, strings.Join(known, ", "))
	default:
		roots := []string{}
		for _, m := range matches {
			roots = append(roots, m.Root)
		}
		return nil, fmt.Errorf("Label %q ist mehrdeutig: %s", label, strings.Join(roots, ", "))
	}
}

// appendNote adds `- text` under today's date heading, opening a new heading
// only when the day changed. The file is only ever appended to — it is a log,
// and rewriting a log invites losing it.
func appendNote(path, label, text string, now time.Time) error {
	date := now.Format("2006-01-02")

	body, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	entry := &strings.Builder{}
	if len(body) == 0 {
		fmt.Fprintf(entry, "# LIMEN — rollierende Notizen zu %s\n", label)
	} else if !strings.HasSuffix(string(body), "\n") {
		entry.WriteString("\n")
	}
	if lastHeading(string(body)) != date {
		fmt.Fprintf(entry, "\n## %s\n", date)
	}
	fmt.Fprintf(entry, "- %s\n", text)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry.String())
	return err
}

// lastHeading returns the date of the last `## ` heading, or "" when none.
func lastHeading(body string) string {
	last := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			last = strings.TrimSpace(line[3:])
		}
	}
	return last
}
