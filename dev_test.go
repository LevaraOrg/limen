package main

// The development port and the proxy view over it. Unit tests for the
// resolution (descriptor over meta over default), CLI tests for `limen ports`,
// because the Caddyfile is a contract with a file on disk.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevPortComesFromTheDescriptor(t *testing.T) {
	c := &Context{Root: "/tmp/circlead", Label: "circlead", Source: SourceLimen}
	c.set("devport", "8080")
	if got := c.DevPort(); got != 8080 {
		t.Errorf("DevPort() = %d, want 8080", got)
	}
	if got, want := c.DevHost(), "circlead.localhost"; got != want {
		t.Errorf("DevHost() = %q, want %q", got, want)
	}
	if got, want := c.DevView().Upstream, "127.0.0.1:8080"; got != want {
		t.Errorf("Upstream = %q, want %q", got, want)
	}
}

func TestDevPortFallsBackToMeta(t *testing.T) {
	c := &Context{Root: "/tmp/tessera", Label: "tessera", Source: SourceLimen,
		Meta: &Meta{DevPort: "8081"}}
	if got := c.DevPort(); got != 8081 {
		t.Errorf("DevPort() = %d, want the declared 8081", got)
	}
}

// The descriptor is the machine's truth: the project may declare 8080, this
// laptop may already have 8080 taken. The local word wins.
func TestDescriptorDevPortOverridesMeta(t *testing.T) {
	c := &Context{Root: "/tmp/x", Label: "x", Source: SourceLimen,
		Meta: &Meta{DevPort: "8080", DevHost: "declared.localhost"}}
	c.set("devport", "18080")
	c.set("devhost", "local.localhost")
	if got := c.DevPort(); got != 18080 {
		t.Errorf("DevPort() = %d, want the machine-local 18080", got)
	}
	if got, want := c.DevHost(), "local.localhost"; got != want {
		t.Errorf("DevHost() = %q, want %q", got, want)
	}
}

func TestDevPortRejectsWhatIsNotAPort(t *testing.T) {
	for _, raw := range []string{"eighty", "0", "-1", "70000", "80 80"} {
		c := &Context{Root: "/tmp/x", Label: "x", Source: SourceLimen}
		c.set("devport", raw)
		if got := c.DevPort(); got != 0 {
			t.Errorf("DevPort() = %d for %q, want 0", got, raw)
		}
		// The raw value survives so the tool can say what it could not read
		// instead of silently dropping a typo.
		if got := c.DevRaw(); got != raw {
			t.Errorf("DevRaw() = %q, want %q", got, raw)
		}
		if c.DevView() != nil {
			t.Errorf("DevView() must be nil for %q", raw)
		}
	}
}

func TestDevHostIsEmptyWithoutAPort(t *testing.T) {
	c := &Context{Root: "/tmp/x", Label: "x", Source: SourceLimen}
	c.set("devhost", "x.localhost")
	if got := c.DevHost(); got != "" {
		t.Errorf("DevHost() = %q, want empty without a port", got)
	}
	if c.DevView() != nil {
		t.Error("DevView() must be nil without a port")
	}
}

// A label is free text; a hostname is not. "Product Strategy" must not become
// an unusable site address in the generated Caddyfile.
func TestDevHostDerivedFromALabelIsHostSafe(t *testing.T) {
	c := &Context{Root: "/tmp/x", Label: "Product Strategy!", Source: SourceLimen}
	c.set("devport", "8082")
	if got, want := c.DevHost(), "product-strategy.localhost"; got != want {
		t.Errorf("DevHost() = %q, want %q", got, want)
	}
}

func TestRenderShowNamesTheDevPort(t *testing.T) {
	c := sampleContext()
	c.set("devport", "8081")
	var buf strings.Builder
	RenderShow(&buf, c, fixedResolver{})
	// One line per endpoint: port and hostname together, because reading them
	// apart is what makes a two-endpoint context unreadable.
	for _, want := range []string{"dev:          8081  tessera.localhost"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("show missing %q:\n%s", want, buf.String())
		}
	}
}

func TestRenderShowMarksAnUnreadableDevPort(t *testing.T) {
	c := sampleContext()
	c.set("devport", "eighty")
	var buf strings.Builder
	RenderShow(&buf, c, fixedResolver{})
	if !strings.Contains(buf.String(), "eighty") || !strings.Contains(buf.String(), "not a port") {
		t.Errorf("show must name the unreadable value:\n%s", buf.String())
	}
}

func TestRenderJSONCarriesDev(t *testing.T) {
	c := sampleContext()
	c.set("devport", "8081")
	var buf strings.Builder
	if err := RenderJSON(&buf, c, fixedResolver{}); err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &v); err != nil {
		t.Fatal(err)
	}
	dev, ok := v["dev"].(map[string]any)
	if !ok {
		t.Fatalf("dev missing or not an object: %s", buf.String())
	}
	if dev["port"] != float64(8081) || dev["host"] != "tessera.localhost" ||
		dev["upstream"] != "127.0.0.1:8081" {
		t.Errorf("dev = %v", dev)
	}
}

func TestRenderJSONDevIsNullWithoutAPort(t *testing.T) {
	var buf strings.Builder
	if err := RenderJSON(&buf, sampleContext(), fixedResolver{}); err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &v); err != nil {
		t.Fatal(err)
	}
	if v["dev"] != nil {
		t.Errorf("dev = %v, want null", v["dev"])
	}
}

// PORT is what a dev server reads without being told. Exporting it is the
// point: cd into the project and `npm run dev` lands on the agreed port.
func TestRenderShellExportsTheDevPort(t *testing.T) {
	c := sampleContext()
	c.set("devport", "8081")
	var buf strings.Builder
	RenderShell(&buf, c, fixedResolver{})
	for _, want := range []string{
		"export LIMEN_DEV_PORT='8081'",
		"export LIMEN_DEV_HOST='tessera.localhost'",
		"export PORT='8081'",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
}

func TestRenderShellOmitsPortWhenUndeclared(t *testing.T) {
	var buf strings.Builder
	RenderShell(&buf, sampleContext(), fixedResolver{})
	if strings.Contains(buf.String(), "PORT=") {
		t.Errorf("no port declared, nothing may be exported:\n%s", buf.String())
	}
}

// --- CLI ------------------------------------------------------------------

// devProject registers a context with a dev port under a shared registry.
func devProject(t *testing.T, env []string, label, port string) string {
	t.Helper()
	root := tempDir(t)
	write(t, filepath.Join(root, ".limen", "limen.yaml"),
		"label: "+label+"\ndevPort: "+port+"\n")
	if r := runLimen(t, root, env, "register"); r.code != 0 {
		t.Fatalf("register exit %d: %s", r.code, r.stderr)
	}
	return root
}

func TestCLIPortsListsTheAllocation(t *testing.T) {
	env := sharedState(t)
	devProject(t, env, "circlead", "8080")
	devProject(t, env, "tessera", "8081")

	r := runLimen(t, tempDir(t), env, "ports")
	if r.code != 0 {
		t.Fatalf("ports exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{"circlead", "8080", "circlead.localhost", "tessera", "8081"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("ports missing %q:\n%s", want, r.stdout)
		}
	}
}

func TestCLIPortsJSON(t *testing.T) {
	env := sharedState(t)
	devProject(t, env, "circlead", "8080")

	r := runLimen(t, tempDir(t), env, "ports", "--json")
	if r.code != 0 {
		t.Fatalf("ports --json exit %d: %s", r.code, r.stderr)
	}
	var views []map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &views); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, r.stdout)
	}
	if len(views) != 1 {
		t.Fatalf("got %d entries, want 1: %s", len(views), r.stdout)
	}
	if views[0]["label"] != "circlead" || views[0]["port"] != float64(8080) ||
		views[0]["upstream"] != "127.0.0.1:8080" {
		t.Errorf("entry = %v", views[0])
	}
}

func TestCLIPortsCaddyfile(t *testing.T) {
	env := sharedState(t)
	devProject(t, env, "circlead", "8080")
	devProject(t, env, "tessera", "8081")

	r := runLimen(t, tempDir(t), env, "ports", "--caddy")
	if r.code != 0 {
		t.Fatalf("ports --caddy exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{
		"circlead.localhost {",
		"\treverse_proxy 127.0.0.1:8080",
		"tessera.localhost {",
		"\treverse_proxy 127.0.0.1:8081",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("Caddyfile missing %q:\n%s", want, r.stdout)
		}
	}
	// Generated files get overwritten. Say so in the file itself.
	if !strings.Contains(r.stdout, "limen ports --caddy") {
		t.Errorf("the snippet must say how it is regenerated:\n%s", r.stdout)
	}
}

func TestCLIPortsFlagsACollision(t *testing.T) {
	env := sharedState(t)
	devProject(t, env, "circlead", "8080")
	devProject(t, env, "tessera", "8080")

	r := runLimen(t, tempDir(t), env, "ports")
	if r.code != 1 {
		t.Fatalf("a double-claimed port must exit 1, got %d:\n%s", r.code, r.stdout)
	}
	if !strings.Contains(r.stdout, "8080") || !strings.Contains(r.stdout, "circlead") {
		t.Errorf("the collision must name port and contexts:\n%s", r.stdout)
	}
	// Still exits 1 when generating, so a pre-flight check catches it.
	if r := runLimen(t, tempDir(t), env, "ports", "--caddy"); r.code != 1 {
		t.Errorf("--caddy must exit 1 on a collision, got %d", r.code)
	}
}

func TestCLIPortsFlagsAnUnreadablePort(t *testing.T) {
	env := sharedState(t)
	devProject(t, env, "circlead", "eighty")

	r := runLimen(t, tempDir(t), env, "ports")
	if r.code != 1 {
		t.Fatalf("an unreadable port must exit 1, got %d:\n%s", r.code, r.stdout)
	}
	if !strings.Contains(r.stdout, "eighty") {
		t.Errorf("the unreadable value must be named:\n%s", r.stdout)
	}
}

func TestCLIPortsWithoutAnyDeclaration(t *testing.T) {
	env := sharedState(t)
	root := tempDir(t)
	write(t, filepath.Join(root, ".limen", "limen.yaml"), "label: quiet\n")
	runLimen(t, root, env, "register")

	r := runLimen(t, tempDir(t), env, "ports")
	if r.code != 0 {
		t.Fatalf("nothing declared is not an error, got %d: %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "devPort") {
		t.Errorf("the empty case must say how a port is declared:\n%s", r.stdout)
	}

	// An empty Caddyfile is still a valid Caddyfile — no sites, no error.
	if r := runLimen(t, tempDir(t), env, "ports", "--caddy"); r.code != 0 {
		t.Errorf("--caddy with nothing declared exit %d", r.code)
	}
}

// meta.yaml is the committed half: a project may declare its dev port for
// everyone who clones it, and `ports` must see that just as well.
func TestCLIPortsReadsTheCommittedDeclaration(t *testing.T) {
	env := sharedState(t)
	root := tempDir(t)
	write(t, filepath.Join(root, ".limen", "limen.yaml"), "label: circlead\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "devPort: 8080\n")
	runLimen(t, root, env, "register")

	r := runLimen(t, tempDir(t), env, "ports", "--json")
	if !strings.Contains(r.stdout, `"port":8080`) {
		t.Errorf("meta.yaml declaration not seen:\n%s", r.stdout)
	}
}

func TestCLIListJSONCarriesTheDevPort(t *testing.T) {
	env := sharedState(t)
	devProject(t, env, "circlead", "8080")

	r := runLimen(t, tempDir(t), env, "list", "--json")
	if !strings.Contains(r.stdout, `"dev":{"port":8080`) {
		t.Errorf("list --json must carry the dev port:\n%s", r.stdout)
	}
}

func TestCLIPortsRejectsAnUnknownFlag(t *testing.T) {
	r := runLimen(t, tempDir(t), sharedState(t), "ports", "--nginx")
	if r.code == 0 {
		t.Errorf("an unknown flag must not pass silently:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "--nginx") {
		t.Errorf("stderr must name the flag: %s", r.stderr)
	}
}

// --- named endpoints -------------------------------------------------------

// One context, several endpoints: a UI dev server and the backend it talks to.
// The first endpoint is the primary — it is what PORT exports and what the bare
// hostname routes to.
func TestDevEndpointsCarryNameAndHost(t *testing.T) {
	c := &Context{Root: "/tmp/tessera", Label: "Tessera", Source: SourceLimen}
	c.set("devendpoints", "5173, api=8081")

	dev := c.DevView()
	if dev == nil {
		t.Fatal("DevView() is nil")
	}
	if len(dev.Endpoints) != 2 {
		t.Fatalf("got %d endpoints, want 2: %+v", len(dev.Endpoints), dev.Endpoints)
	}
	if dev.Port != 5173 || dev.Host != "tessera.localhost" {
		t.Errorf("primary = %d/%s, want 5173/tessera.localhost", dev.Port, dev.Host)
	}
	// A named endpoint hangs off the same first label rather than below it:
	// tessera-api.localhost, not api.tessera.localhost — one label under
	// .localhost is what the existing setups already use.
	api := dev.Endpoints[1]
	if api.Name != "api" || api.Port != 8081 || api.Host != "tessera-api.localhost" {
		t.Errorf("api endpoint = %+v", api)
	}
	if api.Upstream != "127.0.0.1:8081" {
		t.Errorf("upstream = %q", api.Upstream)
	}
}

// devHost renames the base, and the named endpoints follow it: the label is
// circlead-platform, the endpoints are circlead*.localhost.
func TestNamedEndpointsFollowTheDeclaredHost(t *testing.T) {
	c := &Context{Root: "/tmp/c", Label: "circlead-platform", Source: SourceLimen}
	c.set("devhost", "circlead.localhost")
	c.set("devendpoints", "8080 stream, mcp=8084")

	dev := c.DevView()
	if dev.Endpoints[0].Host != "circlead.localhost" {
		t.Errorf("primary host = %q", dev.Endpoints[0].Host)
	}
	if dev.Endpoints[1].Host != "circlead-mcp.localhost" {
		t.Errorf("named host = %q", dev.Endpoints[1].Host)
	}
	// stream is what makes server-sent events work through the proxy.
	if !dev.Endpoints[0].Stream {
		t.Error("the stream flag was dropped")
	}
	if dev.Endpoints[1].Stream {
		t.Error("stream must not leak to the next endpoint")
	}
}

// devPort is the shorthand for a single unnamed endpoint, and stays valid.
func TestDevPortIsShorthandForOneEndpoint(t *testing.T) {
	c := &Context{Root: "/tmp/x", Label: "x", Source: SourceLimen}
	c.set("devport", "8080")
	dev := c.DevView()
	if len(dev.Endpoints) != 1 || dev.Endpoints[0].Name != "" || dev.Endpoints[0].Port != 8080 {
		t.Errorf("endpoints = %+v", dev.Endpoints)
	}
}

// The two declarations are one field in two spellings; the richer one wins so
// a project can outgrow the shorthand without first deleting it.
func TestDevEndpointsOverrideDevPort(t *testing.T) {
	c := &Context{Root: "/tmp/x", Label: "x", Source: SourceLimen}
	c.set("devport", "8080")
	c.set("devendpoints", "9090, api=9091")
	if got := c.DevPort(); got != 9090 {
		t.Errorf("DevPort() = %d, want 9090", got)
	}
}

// The precedence from the single-port case holds for the whole set: the
// machine-local declaration replaces the committed one rather than merging
// with it, so what a descriptor says is what you get.
func TestDescriptorEndpointsReplaceMetaEndpoints(t *testing.T) {
	c := &Context{Root: "/tmp/x", Label: "x", Source: SourceLimen,
		Meta: &Meta{DevEndpoints: "8080, api=8081"}}
	if len(c.DevView().Endpoints) != 2 {
		t.Fatal("meta endpoints must be read")
	}
	c.set("devendpoints", "18080")
	dev := c.DevView()
	if len(dev.Endpoints) != 1 || dev.Endpoints[0].Port != 18080 {
		t.Errorf("endpoints = %+v, want only the machine-local one", dev.Endpoints)
	}
}

func TestDevEndpointsRejectNonsense(t *testing.T) {
	for _, raw := range []string{"api=", "=8080", "api=eighty", "8080 flushed", "a b=8080"} {
		c := &Context{Root: "/tmp/x", Label: "x", Source: SourceLimen}
		c.set("devendpoints", raw)
		if c.DevView() != nil {
			t.Errorf("DevView() must be nil for %q, got %+v", raw, c.DevView())
		}
		if len(c.DevInvalid()) == 0 {
			t.Errorf("no complaint recorded for %q", raw)
		}
	}
}

// A good endpoint next to a broken one must still be reported as broken:
// generating half a proxy configuration silently is the failure this guards.
func TestOneBadEndpointIsReportedBesideTheGoodOnes(t *testing.T) {
	c := &Context{Root: "/tmp/x", Label: "x", Source: SourceLimen}
	c.set("devendpoints", "8080, api=eighty")
	if got := len(c.DevView().Endpoints); got != 1 {
		t.Errorf("got %d usable endpoints, want 1", got)
	}
	if len(c.DevInvalid()) != 1 {
		t.Errorf("invalid = %v, want one complaint", c.DevInvalid())
	}
}

// Two endpoints of one context may not claim the same port either.
func TestRenderShellExportsEveryEndpoint(t *testing.T) {
	c := sampleContext()
	c.set("devendpoints", "5173, api=8081")
	var buf strings.Builder
	RenderShell(&buf, c, fixedResolver{})
	for _, want := range []string{
		"export PORT='5173'",
		"export LIMEN_DEV_PORT='5173'",
		"export LIMEN_DEV_HOST='tessera.localhost'",
		"export LIMEN_DEV_PORT_API='8081'",
		"export LIMEN_DEV_HOST_API='tessera-api.localhost'",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
}

func TestRenderShowListsEveryEndpoint(t *testing.T) {
	c := sampleContext()
	c.set("devendpoints", "5173, api=8081 stream")
	var buf strings.Builder
	RenderShow(&buf, c, fixedResolver{})
	for _, want := range []string{"5173", "tessera.localhost", "api", "8081", "tessera-api.localhost", "stream"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("show missing %q:\n%s", want, buf.String())
		}
	}
}

func TestCLIPortsCaddyfileRendersNamedEndpointsAndStream(t *testing.T) {
	env := sharedState(t)
	root := tempDir(t)
	write(t, filepath.Join(root, ".limen", "limen.yaml"),
		"label: circlead-platform\ndevHost: circlead.localhost\ndevEndpoints: 8080 stream, mcp=8084\n")
	runLimen(t, root, env, "register")

	r := runLimen(t, tempDir(t), env, "ports", "--caddy")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{
		"circlead.localhost {",
		"	reverse_proxy 127.0.0.1:8080 {",
		"		flush_interval -1",
		"circlead-mcp.localhost {",
		"\treverse_proxy 127.0.0.1:8084\n",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("Caddyfile missing %q:\n%s", want, r.stdout)
		}
	}
}

// The generated snippet is meant to be imported next to hand-written blocks.
// Emitting it must therefore be all-or-nothing per context, and a collision
// with a sibling context still has to fail loudly.
func TestCLIPortsFlagsACollisionAcrossEndpoints(t *testing.T) {
	env := sharedState(t)
	root := tempDir(t)
	write(t, filepath.Join(root, ".limen", "limen.yaml"),
		"label: alpha\ndevEndpoints: 8080, api=8081\n")
	runLimen(t, root, env, "register")
	other := tempDir(t)
	write(t, filepath.Join(other, ".limen", "limen.yaml"),
		"label: beta\ndevPort: 8081\n")
	runLimen(t, other, env, "register")

	r := runLimen(t, tempDir(t), env, "ports")
	if r.code != 1 {
		t.Fatalf("want exit 1, got %d:\n%s", r.code, r.stdout)
	}
	if !strings.Contains(r.stdout, "8081") {
		t.Errorf("the collision must name the port:\n%s", r.stdout)
	}
}

var _ = os.Getenv
