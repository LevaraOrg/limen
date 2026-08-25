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
		if got := c.DevPortRaw(); got != raw {
			t.Errorf("DevPortRaw() = %q, want %q", got, raw)
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
	for _, want := range []string{"dev port:     8081", "dev host:     tessera.localhost"} {
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

var _ = os.Getenv
