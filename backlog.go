package main

// `limen backlog` answers the question the notes files cannot answer one by
// one: where is something open? It walks the register, reads every context's
// notes and lists the bullets nobody has ticked off yet — the jumping-off
// point for "cd dorthin und die Aufgabe erledigen".
//
// Done is a leading ✓ on the bullet: `- ✓ …`. Ticking a line is the one
// sanctioned in-place edit of a notes file; everything else stays append-only,
// because the file is a log and rewriting a log invites losing it.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// BacklogEntry is one open bullet, carrying the date heading it sits under.
type BacklogEntry struct {
	Date string `json:"date"`
	Text string `json:"text"`
}

// BacklogView is one context's backlog as `backlog --json` reports it.
type BacklogView struct {
	Root  string         `json:"root"`
	Label string         `json:"label"`
	Open  []BacklogEntry `json:"open"`
	Done  int            `json:"done"`
}

// parseNotes splits a notes file into open entries and a done count. Only
// `- `-bullets count; the `## `-heading above them provides the date.
func parseNotes(body string) (open []BacklogEntry, done int) {
	open = []BacklogEntry{}
	date := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			date = strings.TrimSpace(line[3:])
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		text := strings.TrimSpace(line[2:])
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "✓") {
			done++
			continue
		}
		open = append(open, BacklogEntry{Date: date, Text: text})
	}
	return open, done
}

// CmdBacklog prints every registered context that has notes. Human view shows
// only contexts with open entries — that is the "wo müsste jemand ran"-list;
// JSON carries the ticked-off count too, so agents can see progress.
func CmdBacklog(w io.Writer, jsonOut bool) error {
	ctxs, err := registeredContexts()
	if err != nil {
		return err
	}
	sort.Slice(ctxs, func(i, j int) bool { return ctxs[i].Label < ctxs[j].Label })

	views := []BacklogView{}
	for _, c := range ctxs {
		body, err := os.ReadFile(c.NotesFile())
		if err != nil {
			continue
		}
		open, done := parseNotes(string(body))
		if len(open) == 0 && done == 0 {
			continue
		}
		views = append(views, BacklogView{Root: c.Root, Label: c.Label, Open: open, Done: done})
	}

	if jsonOut {
		b, err := json.Marshal(views)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	}

	totalOpen, totalDone, contexts := 0, 0, 0
	for _, v := range views {
		totalDone += v.Done
		if len(v.Open) == 0 {
			continue
		}
		contexts++
		totalOpen += len(v.Open)
		fmt.Fprintf(w, "%s — %d offen\n", v.Label, len(v.Open))
		fmt.Fprintf(w, "  %s\n", v.Root)
		for _, e := range v.Open {
			fmt.Fprintf(w, "  %s  %s\n", e.Date, truncate(e.Text, 100))
		}
		fmt.Fprintln(w)
	}
	if totalOpen == 0 {
		fmt.Fprintln(w, "Nichts offen — keine unabgehakten Notizen in den registrierten Kontexten.")
		return nil
	}
	fmt.Fprintf(w, "%d offen in %d Kontexten", totalOpen, contexts)
	if totalDone > 0 {
		fmt.Fprintf(w, ", %d abgehakt ✓", totalDone)
	}
	fmt.Fprintln(w)
	return nil
}

// truncate shortens to n runes, not bytes — a note may carry umlauts or ✓,
// and cutting mid-rune would print garbage.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
