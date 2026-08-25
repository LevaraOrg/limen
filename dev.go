package main

// The development port: which port this tree's service listens on while you
// work on it, and under which hostname a local reverse proxy should reach it.
//
// It is here rather than in a docker-compose or a .env because the question it
// answers is not "how do I start this project" but "which of the projects on
// this machine owns 8080". That is a question across trees, and the register
// is the only place that already knows every tree. `limen ports` is therefore
// the allocation table, and `limen ports --caddy` is the same table in the
// shape Caddy reads — one truth, two renderings, so the proxy cannot drift
// away from what the project actually binds.
//
// Declared in either half of the context, and for the usual reason:
//
//	.limen/meta.yaml   devPort: 8080   — committed, what the project agrees on
//	.limen/limen.yaml  devPort: 18080  — machine-local, what this laptop is free to
//
// The descriptor wins. A collision is a property of one machine, and only the
// machine-local half can resolve it without asking everyone else to re-clone.

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Dev is the resolved development endpoint of one context. Nil when nothing is
// declared — most directories are not services and must not pretend to be.
type Dev struct {
	Port int    `json:"port"`
	Host string `json:"host"`
	// Upstream is what a proxy connects to: the loopback address and the port.
	// Loopback rather than 0.0.0.0 on purpose — a development service that is
	// reachable from the coffee-shop network is a defect, not a convenience.
	Upstream string `json:"upstream"`
}

// DevPortRaw is the port exactly as declared, descriptor over meta. Kept as
// written so an unreadable value can be reported instead of silently dropped.
func (c *Context) DevPortRaw() string {
	if c == nil {
		return ""
	}
	if c.devPort != "" {
		return c.devPort
	}
	if c.Meta != nil {
		return c.Meta.DevPort
	}
	return ""
}

// DevPort is the declared port, or 0 when none is declared or the declaration
// is not a port number.
func (c *Context) DevPort() int {
	n, err := strconv.Atoi(c.DevPortRaw())
	if err != nil || n < 1 || n > 65535 {
		return 0
	}
	return n
}

// DevHost is the hostname a proxy should answer to. Declared explicitly, or
// derived from the label: `circlead` becomes `circlead.localhost`, which needs
// no /etc/hosts entry and which Caddy will serve over its internal TLS.
//
// Empty without a port: a hostname with nothing behind it is not an endpoint.
func (c *Context) DevHost() string {
	if c == nil || c.DevPort() == 0 {
		return ""
	}
	if c.devHost != "" {
		return c.devHost
	}
	if c.Meta != nil && c.Meta.DevHost != "" {
		return c.Meta.DevHost
	}
	return hostSafe(c.Label) + ".localhost"
}

// DevView is the resolved endpoint, or nil when there is none.
func (c *Context) DevView() *Dev {
	port := c.DevPort()
	if port == 0 {
		return nil
	}
	return &Dev{
		Port:     port,
		Host:     c.DevHost(),
		Upstream: fmt.Sprintf("127.0.0.1:%d", port),
	}
}

// hostSafe turns a free-text label into one hostname label. A label may carry
// spaces, capitals and umlauts; a site address in a Caddyfile may not.
func hostSafe(s string) string {
	var b strings.Builder
	lastDash := true // leading dashes are dropped
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// portEntry is one line of the allocation table. Invalid carries a declaration
// that could not be read, so the table reports a typo rather than an absence.
type portEntry struct {
	Root     string `json:"root"`
	Label    string `json:"label"`
	Port     int    `json:"port"`
	Host     string `json:"host"`
	Upstream string `json:"upstream"`
	Invalid  string `json:"invalid,omitempty"`
	Conflict string `json:"conflict,omitempty"`
}

// portTable collects every registered context that says something about a dev
// port, sorted by port so the table reads as an allocation, and marks the
// entries that cannot both be true.
func portTable(ctxs []*Context) []portEntry {
	entries := []portEntry{}
	for _, c := range ctxs {
		raw := c.DevPortRaw()
		if raw == "" {
			continue
		}
		e := portEntry{Root: c.Root, Label: c.Label}
		if dev := c.DevView(); dev != nil {
			e.Port, e.Host, e.Upstream = dev.Port, dev.Host, dev.Upstream
		} else {
			e.Invalid = raw
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Port != entries[j].Port {
			return entries[i].Port < entries[j].Port
		}
		return entries[i].Label < entries[j].Label
	})
	markConflicts(entries)
	return entries
}

// markConflicts names both ways two contexts can contradict each other: the
// same port bound twice, and the same hostname routed twice. The second would
// make Caddy refuse the config outright; the first would let it load and send
// you to the wrong application, which is worse.
func markConflicts(entries []portEntry) {
	byPort := map[int][]string{}
	byHost := map[string][]string{}
	for _, e := range entries {
		if e.Port == 0 {
			continue
		}
		byPort[e.Port] = append(byPort[e.Port], e.Label)
		byHost[e.Host] = append(byHost[e.Host], e.Label)
	}
	for i, e := range entries {
		reasons := []string{}
		if peers := others(byPort[e.Port], e.Label); len(peers) > 0 {
			reasons = append(reasons, fmt.Sprintf("port %d also claimed by %s", e.Port, strings.Join(peers, ", ")))
		}
		if peers := others(byHost[e.Host], e.Label); len(peers) > 0 {
			reasons = append(reasons, fmt.Sprintf("host %s also claimed by %s", e.Host, strings.Join(peers, ", ")))
		}
		entries[i].Conflict = strings.Join(reasons, "; ")
	}
}

// others returns the labels in list that are not self, deduplicated. Two
// registered roots with the same label are their own kind of confusion, but
// not one this table has to solve.
func others(list []string, self string) []string {
	out := []string{}
	seen := map[string]bool{self: true}
	for _, l := range list {
		if seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}

// CmdPorts renders the allocation table. Exit code 1 on anything that would
// make the generated proxy configuration wrong — a doubly claimed port, a
// doubly routed host, a declaration that is not a port — so the command can
// stand in a pre-commit hook or before `caddy reload`.
func CmdPorts(w io.Writer, args []string) (int, error) {
	jsonOut, caddyOut := false, false
	for _, a := range args {
		switch a {
		case "--json":
			jsonOut = true
		case "--caddy":
			caddyOut = true
		default:
			return 0, fmt.Errorf("unknown flag %q for ports (--json, --caddy)", a)
		}
	}
	if jsonOut && caddyOut {
		return 0, fmt.Errorf("--json and --caddy are two renderings of one table; pick one")
	}

	ctxs, err := registeredContexts()
	if err != nil {
		return 0, err
	}
	entries := portTable(ctxs)

	code := 0
	for _, e := range entries {
		if e.Invalid != "" || e.Conflict != "" {
			code = 1
		}
	}

	switch {
	case jsonOut:
		b, err := json.Marshal(entries)
		if err != nil {
			return 0, err
		}
		fmt.Fprintln(w, string(b))
	case caddyOut:
		writeCaddyfile(w, entries)
	default:
		writePortTable(w, entries)
	}
	return code, nil
}

func writePortTable(w io.Writer, entries []portEntry) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No development port declared in any registered context.")
		fmt.Fprintln(w, "Declare one with  devPort: 8080  in .limen/limen.yaml (machine-local)")
		fmt.Fprintln(w, "or in .limen/meta.yaml (committed, the same for every clone).")
		return
	}
	width := 0
	for _, e := range entries {
		if len(e.Label) > width {
			width = len(e.Label)
		}
	}
	conflicts := 0
	for _, e := range entries {
		port := strconv.Itoa(e.Port)
		if e.Invalid != "" {
			port = "  ?  "
		}
		fmt.Fprintf(w, "%-*s  %5s  %-28s  %s\n", width, e.Label, port, e.Host, e.Root)
		if e.Invalid != "" {
			conflicts++
			fmt.Fprintf(w, "%-*s  !! devPort %q is not a port number (1–65535)\n", width, "", e.Invalid)
		}
		if e.Conflict != "" {
			conflicts++
			fmt.Fprintf(w, "%-*s  !! %s\n", width, "", e.Conflict)
		}
	}
	fmt.Fprintln(w)
	if conflicts == 0 {
		fmt.Fprintf(w, "%d declared, no conflicts.\n", len(entries))
		return
	}
	fmt.Fprintf(w, "%d declared, %d to resolve.\n", len(entries), conflicts)
}

// writeCaddyfile emits the sites, nothing around them: it is meant to be
// imported from the real Caddyfile, not to replace it. Generated output says
// so in its first line, because a file that looks hand-written will be edited
// by hand and the edit will be lost on the next run.
func writeCaddyfile(w io.Writer, entries []portEntry) {
	fmt.Fprintf(w, "# Generated by limen %s — do not edit; regenerate with `limen ports --caddy`.\n", version)
	fmt.Fprintln(w, "# Import it from your Caddyfile:    import limen.caddy")
	fmt.Fprintln(w)
	if len(entries) == 0 {
		fmt.Fprintln(w, "# No development port declared in any registered context.")
		return
	}
	for _, e := range entries {
		if e.Invalid != "" {
			fmt.Fprintf(w, "# %s: devPort %q is not a port number — no site generated.\n\n", e.Label, e.Invalid)
			continue
		}
		if e.Conflict != "" {
			fmt.Fprintf(w, "# !! %s\n", e.Conflict)
		}
		fmt.Fprintf(w, "# %s — %s\n", e.Label, e.Root)
		fmt.Fprintf(w, "%s {\n", e.Host)
		fmt.Fprintf(w, "\treverse_proxy %s\n", e.Upstream)
		fmt.Fprintln(w, "}")
		fmt.Fprintln(w)
	}
}
