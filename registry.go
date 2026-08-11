package main

// The registry is what turns Limen from a lookup into an inventory. Discover
// only ever walks upward from where it stands; an agent that wants to route a
// note to "the product-strategy directory" needs the opposite: every root this
// machine knows. The registry is that list — one absolute path per line in a
// state file, fed by the shell hook crossing each threshold and by an explicit
// `limen register`. Reading a context stays as cheap as before; the semantic
// matching against purpose/topics happens outside the binary.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// registryPath places the roots file under XDG_STATE_HOME (state, not config:
// the file records what happened on this machine, nothing a user edits).
func registryPath() (string, error) {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "limen", "roots"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "limen", "roots"), nil
}

// loadRoots returns the registered roots, deduplicated, in file order. A
// missing file is an empty registry, not an error.
func loadRoots() ([]string, error) {
	path, err := registryPath()
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	roots := []string{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		roots = append(roots, line)
	}
	return roots, nil
}

// RegisterRoot records a root if it is not already known. Appending with
// O_APPEND keeps concurrent shells from clobbering each other; a rare
// duplicate from a race is harmless because loadRoots deduplicates.
func RegisterRoot(root string) (added bool, err error) {
	roots, err := loadRoots()
	if err != nil {
		return false, err
	}
	for _, r := range roots {
		if r == root {
			return false, nil
		}
	}
	path, err := registryPath()
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.WriteString(root + "\n"); err != nil {
		return false, err
	}
	return true, nil
}

// registeredContexts loads every live registered context. A root whose
// descriptor vanished (directory deleted or moved) is dropped and the file is
// rewritten without it — the registry describes the machine as it is, so a
// read is allowed to shed entries that no longer exist. The guard is
// ctx.Root == root: with the descriptor gone, Discover would climb on and
// report some parent's context, which is not the entry that was registered.
func registeredContexts() ([]*Context, error) {
	roots, err := loadRoots()
	if err != nil {
		return nil, err
	}
	live := []*Context{}
	kept := []string{}
	for _, root := range roots {
		ctx, ok := Discover(root)
		if !ok || ctx.Root != root {
			continue
		}
		live = append(live, ctx)
		kept = append(kept, root)
	}
	if len(kept) < len(roots) {
		if path, err := registryPath(); err == nil {
			os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
		}
	}
	return live, nil
}

// CmdRegister records the context root above each given directory, or above
// the working directory when none is given — for trees one never enters by
// shell, which the hook therefore never sees.
func CmdRegister(w io.Writer, dirs []string, cwd string) error {
	if len(dirs) == 0 {
		dirs = []string{cwd}
	}
	for _, dir := range dirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		ctx, ok := Discover(abs)
		if !ok {
			return fmt.Errorf("kein .limen.yaml und kein .orca/ oberhalb von %s", abs)
		}
		added, err := RegisterRoot(ctx.Root)
		if err != nil {
			return err
		}
		if added {
			fmt.Fprintf(w, "registriert: %s (%s)\n", ctx.Root, ctx.Label)
		} else {
			fmt.Fprintf(w, "bereits registriert: %s (%s)\n", ctx.Root, ctx.Label)
		}
	}
	return nil
}

// listView is the per-context shape of `limen list --json`. Deliberately
// smaller than the json command's view: this is the routing inventory for
// agents, so it carries location and meaning, never identity or key state.
type listView struct {
	Root    string   `json:"root"`
	Label   string   `json:"label"`
	Purpose string   `json:"purpose"`
	Topics  []string `json:"topics"`
	Source  string   `json:"source"`
}

// CmdList prints every live registered context, sorted by label so the
// output is stable regardless of registration order.
func CmdList(w io.Writer, jsonOut bool) error {
	ctxs, err := registeredContexts()
	if err != nil {
		return err
	}
	sort.Slice(ctxs, func(i, j int) bool { return ctxs[i].Label < ctxs[j].Label })

	if jsonOut {
		views := []listView{}
		for _, c := range ctxs {
			views = append(views, listView{
				Root:    c.Root,
				Label:   c.Label,
				Purpose: c.Purpose,
				Topics:  c.TopicList(),
				Source:  string(c.Source),
			})
		}
		b, err := json.Marshal(views)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	}

	if len(ctxs) == 0 {
		fmt.Fprintln(w, "Keine Kontexte registriert. Der Shell-Hook registriert beim Betreten;")
		fmt.Fprintln(w, "sofort:  limen register <pfad…>")
		return nil
	}
	width := 0
	for _, c := range ctxs {
		if len(c.Label) > width {
			width = len(c.Label)
		}
	}
	for _, c := range ctxs {
		fmt.Fprintf(w, "%-*s  %s", width, c.Label, c.Root)
		if c.Purpose != "" {
			fmt.Fprintf(w, "  — %s", c.Purpose)
		}
		fmt.Fprintln(w)
	}
	return nil
}
