package main

// `limen status`: does the declaration still hold?
//
// Limen knows what a context declares. `ports` already checks that the
// declarations are consistent with each other; this checks them against the
// machine — is the declared port actually listening — and joins in the start
// routine, so one table answers "what runs here, how is it brought up, and is
// it up right now".
//
// The line it draws: limen answers questions about its own declarations,
// including whether they hold. Which terminals are open, which tmux session
// belongs to whom, starting and stopping — that is clavo's subject. Clavo
// consumes `limen status --json` and adds its session layer; limen never calls
// clavo. And limen never runs the start command it prints.
//
// Exit code is 0 by default, even with everything down: a status that fails
// because a dev server is off is a status nobody runs. --check is the opposite
// contract, for a pre-commit hook or a watching proxy.

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// prober answers whether an endpoint is up, and by which check. path is the
// declared healthcheck path, empty when there is none; the probe decides
// whether to use it.
type prober func(ep Endpoint, path string) (up bool, check string)

// dialTimeout is short on purpose: the dial is against loopback, where a
// listening port answers within a millisecond and a closed one is refused at
// once. The timeout only ever matters when something is wedged.
const dialTimeout = 300 * time.Millisecond

// dialProbe is the default: a TCP dial against the loopback upstream. It costs
// nothing and needs no network policy, which is why it is the default and the
// HTTP check is opt-in.
func dialProbe(ep Endpoint, _ string) (bool, string) {
	conn, err := net.DialTimeout("tcp", ep.Upstream, dialTimeout)
	if err != nil {
		return false, "dial"
	}
	conn.Close()
	return true, "dial"
}

// deepProbe upgrades the dial to an HTTP GET where a healthcheck path is
// declared. Anything below 400 counts as up: a redirect from a health path is
// still a service that answers.
func deepProbe(ep Endpoint, path string) (bool, string) {
	if path == "" {
		return dialProbe(ep, "")
	}
	client := &http.Client{Timeout: 5 * dialTimeout}
	resp, err := client.Get("http://" + ep.Upstream + path)
	if err != nil {
		return false, "http"
	}
	resp.Body.Close()
	return resp.StatusCode < 400, "http"
}

// statusEndpoint is one declared endpoint and what the probe said about it.
type statusEndpoint struct {
	Endpoint
	Health string `json:"health"`
	Check  string `json:"check"`
}

// statusEntry is one row per context — the shape clavo consumes. Endpoints
// is always non-nil; Start is null when there is nothing to start.
type statusEntry struct {
	Root      string           `json:"root"`
	Label     string           `json:"label"`
	Endpoints []statusEndpoint `json:"endpoints"`
	Start     *Start           `json:"start"`
	// Invalid names endpoint declarations that could not be read, as ports
	// reports them. Empty is the normal case.
	Invalid []string `json:"invalid,omitempty"`
}

// silent reports a context with nothing to show: no endpoint, no start
// routine, no unreadable declaration.
func (e statusEntry) silent() bool {
	return len(e.Endpoints) == 0 && e.Start == nil && len(e.Invalid) == 0
}

// statusTable probes every endpoint of every registered context, in parallel
// because thirty dials in sequence are felt and thirty at once are not. Every
// context yields an entry, silent ones included — the renderer decides what to
// show, and --verbose shows all of it.
func statusTable(ctxs []*Context, probe prober, deep bool) []statusEntry {
	entries := make([]statusEntry, len(ctxs))
	var wg sync.WaitGroup
	for i, c := range ctxs {
		entries[i] = statusEntry{
			Root:      c.Root,
			Label:     c.Label,
			Endpoints: []statusEndpoint{},
			Start:     c.Start(),
			Invalid:   c.DevInvalid(),
		}
		path := ""
		if deep {
			path = readServiceSpec(c.Root).HealthcheckPath
		}
		for j, ep := range c.DevEndpoints() {
			entries[i].Endpoints = append(entries[i].Endpoints, statusEndpoint{Endpoint: ep})
			// The healthcheck belongs to the service, and the primary endpoint
			// is the service; named endpoints stay a dial.
			epPath := ""
			if j == 0 {
				epPath = path
			}
			wg.Add(1)
			go func(i, j int, ep Endpoint, path string) {
				defer wg.Done()
				up, check := probe(ep, path)
				entries[i].Endpoints[j].Check = check
				entries[i].Endpoints[j].Health = "down"
				if up {
					entries[i].Endpoints[j].Health = "up"
				}
			}(i, j, ep, epPath)
		}
	}
	wg.Wait()
	// Case-insensitive, unlike list: this table is read top to bottom by a
	// human, and Tessera sorting before agile-stack reads as a mistake.
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Label) < strings.ToLower(entries[j].Label)
	})
	return entries
}

// checkFailures names every declaration the machine contradicts: a port that
// is not listening, a start routine whose script is gone, an endpoint that
// could not be read.
func checkFailures(entries []statusEntry) []string {
	out := []string{}
	for _, e := range entries {
		for _, ep := range e.Endpoints {
			if ep.Health == "down" {
				out = append(out, fmt.Sprintf("%s: %d is not listening", qualifiedLabel(e.Label, ep.Name), ep.Port))
			}
		}
		if e.Start != nil && e.Start.Missing {
			out = append(out, fmt.Sprintf("%s: start routine %q declared in %s is not on disk", e.Label, e.Start.Command, e.Start.Evidence))
		}
		for _, bad := range e.Invalid {
			out = append(out, fmt.Sprintf("%s: %s", e.Label, bad))
		}
	}
	return out
}

func qualifiedLabel(label, name string) string {
	if name == "" {
		return label
	}
	return label + "/" + name
}

// CmdStatus renders the table. Flags mirror ports where they overlap.
func CmdStatus(w io.Writer, args []string) (int, error) {
	jsonOut, deep, verbose, check := false, false, false, false
	for _, a := range args {
		switch a {
		case "--json":
			jsonOut = true
		case "--deep":
			deep = true
		case "--verbose", "-v":
			verbose = true
		case "--check":
			check = true
		default:
			return 0, fmt.Errorf("unknown flag %q for status (--json, --deep, --verbose, --check)", a)
		}
	}

	ctxs, err := registeredContexts()
	if err != nil {
		return 0, err
	}
	probe := dialProbe
	if deep {
		probe = deepProbe
	}
	entries := statusTable(ctxs, probe, deep)

	code := 0
	failures := checkFailures(entries)
	if check && len(failures) > 0 {
		code = 1
	}

	if jsonOut {
		shown := []statusEntry{}
		for _, e := range entries {
			if verbose || !e.silent() {
				shown = append(shown, e)
			}
		}
		b, err := json.Marshal(shown)
		if err != nil {
			return 0, err
		}
		fmt.Fprintln(w, string(b))
		return code, nil
	}
	writeStatusTable(w, entries, verbose)
	if check {
		for _, f := range failures {
			fmt.Fprintf(w, "!! %s\n", f)
		}
	}
	return code, nil
}

// statusRow is one line of the rendered table.
type statusRow struct {
	context, endpoint, start, health string
}

// statusRows flattens entries: the first endpoint of a context carries the
// label and the start routine, every further endpoint hangs below it as
// label/name with a dot in the start column, a context without an endpoint
// gets one row with dots where the endpoint would be.
func statusRows(entries []statusEntry, verbose bool) []statusRow {
	rows := []statusRow{}
	for _, e := range entries {
		if e.silent() && !verbose {
			continue
		}
		if len(e.Endpoints) == 0 {
			rows = append(rows, statusRow{e.Label, "    ·", e.Start.Cell(), "·"})
			continue
		}
		for i, ep := range e.Endpoints {
			row := statusRow{
				context:  qualifiedLabel(e.Label, ep.Name),
				endpoint: fmt.Sprintf("%5d  %s", ep.Port, strings.TrimSuffix(ep.Host, ".localhost")),
				start:    "·",
				health:   ep.Health,
			}
			if i == 0 {
				row.context = e.Label
				row.start = e.Start.Cell()
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func writeStatusTable(w io.Writer, entries []statusEntry, verbose bool) {
	rows := statusRows(entries, verbose)
	if len(rows) == 0 {
		if len(entries) == 0 {
			fmt.Fprintln(w, "No contexts registered. The shell hook registers on entry;")
			fmt.Fprintln(w, "right now:  limen register <path…>")
			return
		}
		fmt.Fprintf(w, "%d context(s) registered, none with an endpoint or a start routine.\n", len(entries))
		fmt.Fprintln(w, "An endpoint is declared with  devEndpoints:  — see limen ports.")
		fmt.Fprintln(w, "A start routine is discovered from scripts/start*.sh, package.json,")
		fmt.Fprintln(w, "Makefile, a compose file, go.mod, pom.xml or pubspec.yaml; declare")
		fmt.Fprintln(w, "start:  in .limen/meta.yaml only when discovery finds several.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, statusSummary(entries))
		return
	}
	widths := [3]int{len("CONTEXT"), len("ENDPOINT"), len("START")}
	for _, r := range rows {
		for i, cell := range []string{r.context, r.endpoint, r.start} {
			if n := len([]rune(cell)); n > widths[i] {
				widths[i] = n
			}
		}
	}
	line := func(a, b, c, d string) {
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n",
			widths[0], a, widths[1]+len(b)-len([]rune(b)), b, widths[2]+len(c)-len([]rune(c)), c, d)
	}
	line("CONTEXT", "ENDPOINT", "START", "HEALTH")
	for _, r := range rows {
		line(r.context, r.endpoint, r.start, r.health)
	}
	if verbose {
		fmt.Fprintln(w)
		for _, e := range entries {
			if e.Start == nil || len(e.Start.Candidates) == 0 {
				continue
			}
			fmt.Fprintf(w, "%s\n", e.Label)
			if e.Start.Source == "declared" {
				fmt.Fprintf(w, "  declared    %-28s  %s\n", e.Start.Command, e.Start.Evidence)
			}
			for _, cand := range e.Start.Candidates {
				fmt.Fprintf(w, "  discovered  %-28s  %s\n", cand.Command, cand.Evidence)
			}
		}
	}
	for _, e := range entries {
		for _, bad := range e.Invalid {
			fmt.Fprintf(w, "!! %s: %s\n", e.Label, bad)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, statusSummary(entries))
}

// statusSummary is the one line that survives when the table scrolls away.
func statusSummary(entries []statusEntry) string {
	endpoints, up, down, noStart := 0, 0, 0, 0
	for _, e := range entries {
		for _, ep := range e.Endpoints {
			endpoints++
			if ep.Health == "up" {
				up++
			} else {
				down++
			}
		}
		if e.Start == nil {
			noStart++
		}
	}
	parts := []string{
		plural(endpoints, "endpoint"),
		strconv.Itoa(up) + " up",
		strconv.Itoa(down) + " down",
		plural(noStart, "context") + " without a start routine",
	}
	return strings.Join(parts, " · ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
