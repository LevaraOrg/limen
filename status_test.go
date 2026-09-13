package main

// `limen status`: does the declaration still hold? Unit tests for the table
// with an injected probe, CLI tests against real listeners in the test
// process, because a dial is the whole extent of the runtime limen looks at.

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// listen opens a TCP listener on a free loopback port and returns the port.
func listen(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l.Addr().(*net.TCPAddr).Port
}

// closedPort returns a loopback port nothing listens on.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// statusProject registers a context with a descriptor body and optional files.
func statusProject(t *testing.T, env []string, body string, files map[string]string) string {
	t.Helper()
	root := tempDir(t)
	write(t, filepath.Join(root, ".limen", "limen.yaml"), body)
	for name, content := range files {
		write(t, filepath.Join(root, name), content)
	}
	if r := runLimen(t, root, env, "register"); r.code != 0 {
		t.Fatalf("register exit %d: %s", r.code, r.stderr)
	}
	return root
}

// --- table ----------------------------------------------------------------

// upOnly answers up for one port and down for every other.
func upOnly(port int) prober {
	return func(ep Endpoint, path string) (bool, string) {
		return ep.Port == port, "dial"
	}
}

func TestStatusTableJoinsEndpointHealthAndStart(t *testing.T) {
	a := tempDir(t)
	write(t, filepath.Join(a, "scripts", "start.sh"), "")
	up := &Context{Root: a, Label: "up", Source: SourceLimen}
	up.set("devendpoints", "8080, api=8081")
	down := &Context{Root: tempDir(t), Label: "down", Source: SourceLimen}
	down.set("devport", "9090")

	entries := statusTable([]*Context{down, up}, upOnly(8080), false)
	if len(entries) != 2 || entries[0].Label != "down" || entries[1].Label != "up" {
		t.Fatalf("entries sorted by label, got %+v", entries)
	}
	if h := entries[0].Endpoints[0].Health; h != "down" {
		t.Errorf("9090 health = %q, want down", h)
	}
	if entries[0].Start != nil {
		t.Errorf("no start routine, want nil: %+v", entries[0].Start)
	}
	if got := entries[1].Endpoints; len(got) != 2 || got[0].Health != "up" || got[1].Health != "down" {
		t.Errorf("up endpoints = %+v", got)
	}
	if entries[1].Start == nil || entries[1].Start.Command != "scripts/start.sh" {
		t.Errorf("start = %+v", entries[1].Start)
	}
}

// A directory with neither an endpoint nor a start routine has no row: most
// registered contexts are document repositories, and a table of ✗ says less
// than a count. The entry still exists, so the count and --verbose have it.
func TestStatusTableSkipsWhatDeclaresNothing(t *testing.T) {
	quiet := &Context{Root: tempDir(t), Label: "docs", Source: SourceLimen}
	entries := statusTable([]*Context{quiet}, upOnly(0), false)
	if len(entries) != 1 || !entries[0].silent() {
		t.Fatalf("the entry exists and is silent: %+v", entries)
	}
	if rows := statusRows(entries, false); len(rows) != 0 {
		t.Errorf("a silent context has no row: %+v", rows)
	}
	if rows := statusRows(entries, true); len(rows) != 1 || rows[0].start != "✗ none" {
		t.Errorf("--verbose shows it as ✗ none: %+v", rows)
	}
}

func TestStatusTableKeepsAStartWithoutAnEndpoint(t *testing.T) {
	root := tempDir(t)
	write(t, filepath.Join(root, "go.mod"), "module x\n")
	c := &Context{Root: root, Label: "tool", Source: SourceLimen}
	entries := statusTable([]*Context{c}, upOnly(0), false)
	if len(entries) != 1 || len(entries[0].Endpoints) != 0 || entries[0].Start == nil {
		t.Errorf("a start routine alone earns a row: %+v", entries)
	}
}

// --- CLI ------------------------------------------------------------------

func TestCLIStatusDialsTheDeclaredPorts(t *testing.T) {
	env := sharedState(t)
	up, down := listen(t), closedPort(t)
	statusProject(t, env, "label: alive\ndevPort: "+strconv.Itoa(up)+"\n",
		map[string]string{"scripts/start.sh": ""})
	statusProject(t, env, "label: dead\ndevPort: "+strconv.Itoa(down)+"\n", nil)

	r := runLimen(t, tempDir(t), env, "status")
	if r.code != 0 {
		t.Fatalf("status exits 0 even with something down, got %d: %s", r.code, r.stderr)
	}
	for _, line := range strings.Split(r.stdout, "\n") {
		switch {
		case strings.HasPrefix(line, "alive"):
			if !strings.Contains(line, strconv.Itoa(up)) || !strings.Contains(line, "scripts/start.sh") ||
				!strings.HasSuffix(strings.TrimSpace(line), "up") {
				t.Errorf("alive row: %q", line)
			}
		case strings.HasPrefix(line, "dead"):
			if !strings.Contains(line, "✗ none") || !strings.HasSuffix(strings.TrimSpace(line), "down") {
				t.Errorf("dead row: %q", line)
			}
		}
	}
	if !strings.Contains(r.stdout, "2 endpoints · 1 up · 1 down") {
		t.Errorf("summary line:\n%s", r.stdout)
	}
}

func TestCLIStatusCheckFailsOnWhatIsDown(t *testing.T) {
	env := sharedState(t)
	up := listen(t)
	statusProject(t, env, "label: alive\ndevPort: "+strconv.Itoa(up)+"\n", nil)

	if r := runLimen(t, tempDir(t), env, "status", "--check"); r.code != 0 {
		t.Fatalf("everything up, --check must exit 0, got %d:\n%s", r.code, r.stdout)
	}

	down := closedPort(t)
	statusProject(t, env, "label: dead\ndevPort: "+strconv.Itoa(down)+"\n", nil)
	r := runLimen(t, tempDir(t), env, "status", "--check")
	if r.code != 1 {
		t.Fatalf("a declared port not listening must fail --check, got %d:\n%s", r.code, r.stdout)
	}
	if !strings.Contains(r.stdout, "dead") {
		t.Errorf("--check must name what is down:\n%s", r.stdout)
	}
}

// A declared start routine whose script is gone is a declaration limen knows
// to be wrong — the same reason ports refuses a collision.
func TestCLIStatusCheckFailsOnAMissingDeclaredStart(t *testing.T) {
	env := sharedState(t)
	up := listen(t)
	root := statusProject(t, env, "label: renamed\ndevPort: "+strconv.Itoa(up)+"\n", nil)
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "start: scripts/start-old.sh\n")

	r := runLimen(t, tempDir(t), env, "status")
	if r.code != 0 || !strings.Contains(r.stdout, "scripts/start-old.sh") || !strings.Contains(r.stdout, "missing") {
		t.Errorf("status shows the drift and still exits 0 (%d):\n%s", r.code, r.stdout)
	}
	if r := runLimen(t, tempDir(t), env, "status", "--check"); r.code != 1 {
		t.Errorf("--check must fail on a missing declared start, got %d:\n%s", r.code, r.stdout)
	}
}

func TestCLIStatusJSONIsTheShapeClavoReads(t *testing.T) {
	env := sharedState(t)
	up := listen(t)
	statusProject(t, env, "label: alive\ndevEndpoints: "+strconv.Itoa(up)+", api=1\n",
		map[string]string{"package.json": `{"scripts":{"dev":"vite"}}`})

	r := runLimen(t, tempDir(t), env, "status", "--json")
	if r.code != 0 {
		t.Fatalf("status --json exit %d: %s", r.code, r.stderr)
	}
	var entries []struct {
		Root      string `json:"root"`
		Label     string `json:"label"`
		Endpoints []struct {
			Name   string `json:"name"`
			Port   int    `json:"port"`
			Host   string `json:"host"`
			Health string `json:"health"`
			Check  string `json:"check"`
		} `json:"endpoints"`
		Start *Start `json:"start"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &entries); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, r.stdout)
	}
	if len(entries) != 1 || entries[0].Label != "alive" || len(entries[0].Endpoints) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	e := entries[0]
	if e.Endpoints[0].Port != up || e.Endpoints[0].Health != "up" || e.Endpoints[0].Check != "dial" {
		t.Errorf("primary = %+v", e.Endpoints[0])
	}
	if e.Endpoints[1].Name != "api" || e.Endpoints[1].Health != "down" {
		t.Errorf("api = %+v", e.Endpoints[1])
	}
	if e.Start == nil || e.Start.Command != "npm run dev" || e.Start.Source != "discovered" {
		t.Errorf("start = %+v", e.Start)
	}
}

func TestCLIStatusVerboseListsEveryCandidate(t *testing.T) {
	env := sharedState(t)
	statusProject(t, env, "label: circlead\n", map[string]string{
		"scripts/start-circlead.sh": "",
		"scripts/start-stack.sh":    "",
		"docker-compose.yml":        "",
	})

	r := runLimen(t, tempDir(t), env, "status")
	if !strings.Contains(r.stdout, "scripts/start-circlead.sh (+2)") {
		t.Errorf("ambiguity is shown, not hidden:\n%s", r.stdout)
	}
	if strings.Contains(r.stdout, "docker compose up") {
		t.Errorf("the other candidates wait for --verbose:\n%s", r.stdout)
	}
	r = runLimen(t, tempDir(t), env, "status", "--verbose")
	for _, want := range []string{"scripts/start-stack.sh", "docker compose up", "docker-compose.yml"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("--verbose missing %q:\n%s", want, r.stdout)
		}
	}
}

// --deep upgrades the dial to an HTTP GET where service.yaml names a path;
// without one the dial stays, because a dial needs no network policy.
func TestCLIStatusDeepUsesTheDeclaredHealthcheck(t *testing.T) {
	env := sharedState(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/actuator/health" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(503)
	}))
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	service := "apiVersion: agnostic-stack/v1\nkind: Service\nspec:\n  healthcheck:\n    path: %s\n    port: 8080\n"
	statusProject(t, env, "label: healthy\ndevPort: "+strconv.Itoa(port)+"\n",
		map[string]string{"service.yaml": strings.Replace(service, "%s", "/actuator/health", 1)})
	statusProject(t, env, "label: sick\ndevPort: "+strconv.Itoa(port)+"\n",
		map[string]string{"service.yaml": strings.Replace(service, "%s", "/nope", 1)})
	statusProject(t, env, "label: plain\ndevPort: "+strconv.Itoa(port)+"\n", nil)

	r := runLimen(t, tempDir(t), env, "status", "--deep", "--json")
	if r.code != 0 {
		t.Fatalf("status --deep exit %d: %s", r.code, r.stderr)
	}
	var entries []struct {
		Label     string `json:"label"`
		Endpoints []struct {
			Health string `json:"health"`
			Check  string `json:"check"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &entries); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, r.stdout)
	}
	want := map[string][2]string{"healthy": {"up", "http"}, "sick": {"down", "http"}, "plain": {"up", "dial"}}
	for _, e := range entries {
		got := [2]string{e.Endpoints[0].Health, e.Endpoints[0].Check}
		if got != want[e.Label] {
			t.Errorf("%s = %v, want %v", e.Label, got, want[e.Label])
		}
	}
	// Without --deep the sick one is a plain dial, and up.
	r = runLimen(t, tempDir(t), env, "status", "--json")
	if strings.Contains(r.stdout, `"health":"down"`) || strings.Contains(r.stdout, `"check":"http"`) {
		t.Errorf("the default stays a dial:\n%s", r.stdout)
	}
}

func TestCLIStatusWithNothingDeclaredSaysSo(t *testing.T) {
	env := sharedState(t)
	statusProject(t, env, "label: docs\n", nil)
	r := runLimen(t, tempDir(t), env, "status")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "1 context without a start routine") {
		t.Errorf("the silent context is counted, not listed:\n%s", r.stdout)
	}
	if strings.Contains(r.stdout, "docs ") {
		t.Errorf("a silent context has no row:\n%s", r.stdout)
	}
	if r := runLimen(t, tempDir(t), env, "status", "--verbose"); !strings.Contains(r.stdout, "docs") {
		t.Errorf("--verbose lists it anyway:\n%s", r.stdout)
	}
}

func TestCLIStatusRejectsAnUnknownFlag(t *testing.T) {
	r := runLimen(t, tempDir(t), sharedState(t), "status", "--restart")
	if r.code == 0 {
		t.Fatal("an unknown flag must not exit 0")
	}
	if !strings.Contains(r.stderr, "--restart") {
		t.Errorf("the flag must be named: %s", r.stderr)
	}
}
