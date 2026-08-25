package main

// The development endpoints: which ports this tree's services listen on while
// you work on them, and under which hostnames a local reverse proxy reaches
// them.
//
// They are here rather than in a docker-compose or a .env because the question
// they answer is not "how do I start this project" but "which of the projects
// on this machine owns 8080". That is a question across trees, and the register
// is the only place that already knows every tree. `limen ports` is therefore
// the allocation table, and `limen ports --caddy` is the same table in the
// shape Caddy reads — one truth, two renderings, so the proxy cannot drift away
// from what the project actually binds.
//
// Declared in either half of the context, and for the usual reason:
//
//	.limen/meta.yaml   devEndpoints: 5173, api=8081   — committed
//	.limen/limen.yaml  devEndpoints: 15173, api=18081 — machine-local
//
// The descriptor wins, and it replaces the set rather than merging into it: a
// half-overridden list would be a third thing that neither file states. A
// collision is a property of one machine, and only the machine-local half can
// resolve it without asking everyone else to re-clone.
//
// A project is rarely one port. A UI dev server and the backend it proxies to
// are two, and a stream endpoint needs the proxy to stop buffering. The single
// `devPort:` is kept as the shorthand it always was — one unnamed endpoint —
// so nothing that already declares it has to be rewritten.

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Endpoint is one reachable port of a context. Name is empty for the primary
// endpoint, which answers under the bare hostname.
type Endpoint struct {
	Name string `json:"name"`
	Port int    `json:"port"`
	Host string `json:"host"`
	// Upstream is what a proxy connects to: the loopback address and the port.
	// Loopback rather than 0.0.0.0 on purpose — a development service that is
	// reachable from the coffee-shop network is a defect, not a convenience.
	Upstream string `json:"upstream"`
	// Stream asks the proxy not to buffer. Server-sent events and MCP streams
	// arrive in one lump at the end of the request without it, which looks
	// exactly like a hung server.
	Stream bool `json:"stream,omitempty"`
}

// Dev is the resolved development endpoint set. Nil when nothing usable is
// declared — most directories are not services and must not pretend to be.
// Port, Host and Upstream mirror the primary endpoint, because that is the one
// answer most callers want and PORT can only carry one value anyway.
type Dev struct {
	Port      int        `json:"port"`
	Host      string     `json:"host"`
	Upstream  string     `json:"upstream"`
	Endpoints []Endpoint `json:"endpoints"`
}

// DevRaw is the declaration exactly as written, descriptor over meta and the
// endpoint list over the single-port shorthand. Kept as written so a typo can
// be reported instead of silently dropped.
func (c *Context) DevRaw() string {
	if c == nil {
		return ""
	}
	if c.devEndpoints != "" {
		return c.devEndpoints
	}
	if c.devPort != "" {
		return c.devPort
	}
	if c.Meta != nil {
		if c.Meta.DevEndpoints != "" {
			return c.Meta.DevEndpoints
		}
		return c.Meta.DevPort
	}
	return ""
}

// DevBaseHost is the hostname the primary endpoint answers to, and the stem the
// named ones hang off. Declared explicitly, or derived from the label:
// `circlead` becomes `circlead.localhost`, which needs no /etc/hosts entry and
// which Caddy will serve over its internal TLS.
func (c *Context) DevBaseHost() string {
	if c == nil {
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

// endpointHost places a named endpoint beside the base rather than below it:
// `tessera-api.localhost`, not `api.tessera.localhost`. One label under
// .localhost is what browsers, Caddy and existing bookmarks already handle
// without anyone thinking about wildcard certificates.
func endpointHost(base, name string) string {
	if name == "" {
		return base
	}
	first, rest, found := strings.Cut(base, ".")
	if !found {
		return base + "-" + name
	}
	return first + "-" + name + "." + rest
}

// parseDev reads the declaration into endpoints and complaints. Both are
// returned: a list where one item is a typo must still yield the others, and
// must still say that something was unreadable — see CmdPorts, which refuses
// to call such a configuration good.
func parseDev(raw, base string) ([]Endpoint, []string) {
	endpoints := []Endpoint{}
	invalid := []string{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		ep, err := parseEndpoint(item, base)
		if err != nil {
			invalid = append(invalid, fmt.Sprintf("%q: %v", item, err))
			continue
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, invalid
}

// parseEndpoint reads one item: `[name=]port [flag…]`.
func parseEndpoint(item, base string) (Endpoint, error) {
	fields := strings.Fields(item)
	name, portText, found := strings.Cut(fields[0], "=")
	if !found {
		name, portText = "", fields[0]
	}
	name = strings.ToLower(name)
	if found && (name == "" || hostSafe(name) != name) {
		return Endpoint{}, fmt.Errorf("%q is not an endpoint name", name)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return Endpoint{}, fmt.Errorf("%q is not a port number (1–65535)", portText)
	}
	ep := Endpoint{
		Name:     name,
		Port:     port,
		Host:     endpointHost(base, name),
		Upstream: fmt.Sprintf("127.0.0.1:%d", port),
	}
	for _, flag := range fields[1:] {
		switch strings.ToLower(flag) {
		case "stream":
			ep.Stream = true
		default:
			return Endpoint{}, fmt.Errorf("unknown flag %q (stream)", flag)
		}
	}
	return ep, nil
}

// DevEndpoints is the usable endpoint set, in declaration order.
func (c *Context) DevEndpoints() []Endpoint {
	eps, _ := parseDev(c.DevRaw(), c.DevBaseHost())
	return eps
}

// DevInvalid names what could not be read. Empty is the normal case.
func (c *Context) DevInvalid() []string {
	_, bad := parseDev(c.DevRaw(), c.DevBaseHost())
	return bad
}

// DevView is the resolved endpoint set, or nil when there is none.
func (c *Context) DevView() *Dev {
	eps := c.DevEndpoints()
	if len(eps) == 0 {
		return nil
	}
	return &Dev{
		Port:      eps[0].Port,
		Host:      eps[0].Host,
		Upstream:  eps[0].Upstream,
		Endpoints: eps,
	}
}

// DevPort is the primary port, or 0 when none is declared.
func (c *Context) DevPort() int {
	if dev := c.DevView(); dev != nil {
		return dev.Port
	}
	return 0
}

// DevHost is the primary hostname, empty without a port: a hostname with
// nothing behind it is not an endpoint.
func (c *Context) DevHost() string {
	if dev := c.DevView(); dev != nil {
		return dev.Host
	}
	return ""
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

// envSuffix is the upper-case form a named endpoint takes in an exported
// variable: api → LIMEN_DEV_PORT_API.
func envSuffix(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}

// portEntry is one line of the allocation table: one endpoint of one context.
type portEntry struct {
	Root     string `json:"root"`
	Label    string `json:"label"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Host     string `json:"host"`
	Upstream string `json:"upstream"`
	Stream   bool   `json:"stream,omitempty"`
	Conflict string `json:"conflict,omitempty"`
}

// portTable collects every endpoint of every registered context, sorted by port
// so the table reads as an allocation, and marks what cannot all be true.
func portTable(ctxs []*Context) (entries []portEntry, complaints []string) {
	entries = []portEntry{}
	complaints = []string{}
	for _, c := range ctxs {
		for _, bad := range c.DevInvalid() {
			complaints = append(complaints, fmt.Sprintf("%s: %s", c.Label, bad))
		}
		for _, ep := range c.DevEndpoints() {
			entries = append(entries, portEntry{
				Root: c.Root, Label: c.Label, Name: ep.Name,
				Port: ep.Port, Host: ep.Host, Upstream: ep.Upstream, Stream: ep.Stream,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Port != entries[j].Port {
			return entries[i].Port < entries[j].Port
		}
		return entries[i].Label < entries[j].Label
	})
	markConflicts(entries)
	return entries, complaints
}

// qualified names an entry the way a human finds it again: the context, and the
// endpoint within it when there is more than one.
func (e portEntry) qualified() string {
	if e.Name == "" {
		return e.Label
	}
	return e.Label + "/" + e.Name
}

// markConflicts names both ways two endpoints can contradict each other: the
// same port bound twice, and the same hostname routed twice. The second makes
// Caddy refuse the configuration outright; the first lets it load and sends you
// to the wrong application, which is worse.
func markConflicts(entries []portEntry) {
	byPort := map[int][]string{}
	byHost := map[string][]string{}
	for _, e := range entries {
		byPort[e.Port] = append(byPort[e.Port], e.qualified())
		byHost[e.Host] = append(byHost[e.Host], e.qualified())
	}
	for i, e := range entries {
		reasons := []string{}
		if peers := others(byPort[e.Port], e.qualified()); len(peers) > 0 {
			reasons = append(reasons, fmt.Sprintf("port %d also claimed by %s", e.Port, strings.Join(peers, ", ")))
		}
		if peers := others(byHost[e.Host], e.qualified()); len(peers) > 0 {
			reasons = append(reasons, fmt.Sprintf("host %s also claimed by %s", e.Host, strings.Join(peers, ", ")))
		}
		entries[i].Conflict = strings.Join(reasons, "; ")
	}
}

// others returns the names in list that are not self, deduplicated.
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
// doubly routed host, a declaration that cannot be read — so the command can
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
	entries, complaints := portTable(ctxs)

	code := 0
	if len(complaints) > 0 {
		code = 1
	}
	for _, e := range entries {
		if e.Conflict != "" {
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
		writeCaddyfile(w, entries, complaints)
	default:
		writePortTable(w, entries, complaints)
	}
	return code, nil
}

func writePortTable(w io.Writer, entries []portEntry, complaints []string) {
	if len(entries) == 0 && len(complaints) == 0 {
		fmt.Fprintln(w, "No development port declared in any registered context.")
		fmt.Fprintln(w, "Declare one with  devPort: 8080  in .limen/limen.yaml (machine-local)")
		fmt.Fprintln(w, "or in .limen/meta.yaml (committed, the same for every clone).")
		fmt.Fprintln(w, "Several:          devEndpoints: 5173, api=8081 stream")
		return
	}
	width := 0
	for _, e := range entries {
		if len(e.qualified()) > width {
			width = len(e.qualified())
		}
	}
	conflicts := len(complaints)
	for _, e := range entries {
		flags := "      "
		if e.Stream {
			flags = "stream"
		}
		fmt.Fprintf(w, "%-*s  %5d  %-28s  %s  %s\n", width, e.qualified(), e.Port, e.Host, flags, e.Root)
		if e.Conflict != "" {
			conflicts++
			fmt.Fprintf(w, "%-*s  !! %s\n", width, "", e.Conflict)
		}
	}
	for _, c := range complaints {
		fmt.Fprintf(w, "!! %s\n", c)
	}
	fmt.Fprintln(w)
	if conflicts == 0 {
		fmt.Fprintf(w, "%d endpoint(s), no conflicts.\n", len(entries))
		return
	}
	fmt.Fprintf(w, "%d endpoint(s), %d to resolve.\n", len(entries), conflicts)
}

// writeCaddyfile emits the sites, nothing around them: it is meant to be
// imported from the real Caddyfile, not to replace it. Generated output says so
// in its first line, because a file that looks hand-written will be edited by
// hand and the edit will be lost on the next run.
func writeCaddyfile(w io.Writer, entries []portEntry, complaints []string) {
	fmt.Fprintf(w, "# Generated by limen %s — do not edit; regenerate with `limen ports --caddy`.\n", version)
	fmt.Fprintln(w, "# Import it from your Caddyfile:    import limen.caddy")
	fmt.Fprintln(w)
	for _, c := range complaints {
		fmt.Fprintf(w, "# !! %s — no site generated.\n", c)
	}
	if len(complaints) > 0 {
		fmt.Fprintln(w)
	}
	if len(entries) == 0 {
		fmt.Fprintln(w, "# No development port declared in any registered context.")
		return
	}
	for _, e := range entries {
		if e.Conflict != "" {
			fmt.Fprintf(w, "# !! %s\n", e.Conflict)
		}
		fmt.Fprintf(w, "# %s — %s\n", e.qualified(), e.Root)
		fmt.Fprintf(w, "%s {\n", e.Host)
		if e.Stream {
			// Without this the proxy buffers, and server-sent events arrive in
			// one lump at the end of the request — indistinguishable from a
			// hung server.
			fmt.Fprintf(w, "\treverse_proxy %s {\n", e.Upstream)
			fmt.Fprintln(w, "\t\tflush_interval -1")
			fmt.Fprintln(w, "\t}")
		} else {
			fmt.Fprintf(w, "\treverse_proxy %s\n", e.Upstream)
		}
		fmt.Fprintln(w, "}")
		fmt.Fprintln(w)
	}
}
