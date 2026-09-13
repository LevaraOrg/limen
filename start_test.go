package main

// The start routine: discovered from the files in the directory, declared only
// to disambiguate. Unit tests per discovery rule, for the precedence of the
// declaration and for the "declared but gone" case; CLI tests for what
// show/json/list report, because that is the shape clavo reads.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkdirAll is the test's shorthand for a directory that must exist.
func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// commands flattens candidates to what a human would type.
func commands(cands []StartCandidate) []string {
	out := []string{}
	for _, c := range cands {
		out = append(out, c.Command)
	}
	return out
}

func TestDiscoverStartOneRuleAtATime(t *testing.T) {
	cases := []struct {
		name     string
		files    map[string]string
		command  string
		evidence string
	}{
		{"service.yaml spec.start", map[string]string{
			"service.yaml": "apiVersion: agnostic-stack/v1\nkind: Service\nspec:\n  runtime:\n    build: maven\n  start: mvn spring-boot:run -Dspring.profiles.active=dev\n",
		}, "mvn spring-boot:run -Dspring.profiles.active=dev", "service.yaml"},
		{"scripts/start.sh", map[string]string{
			"scripts/start.sh": "#!/bin/sh\n",
		}, "scripts/start.sh", "scripts/start.sh"},
		{"scripts/start-<label>.sh", map[string]string{
			"scripts/start-api.sh": "#!/bin/sh\n",
		}, "scripts/start-api.sh", "scripts/start-api.sh"},
		{"package.json scripts.dev", map[string]string{
			"package.json": `{"name":"x","scripts":{"start":"node .","dev":"vite"}}`,
		}, "npm run dev", "package.json"},
		{"package.json scripts.start when there is no dev", map[string]string{
			"package.json": `{"scripts":{"start":"node .","serve":"x"}}`,
		}, "npm run start", "package.json"},
		{"package.json scripts.serve as the last resort", map[string]string{
			"package.json": `{"scripts":{"serve":"x","test":"y"}}`,
		}, "npm run serve", "package.json"},
		{"Makefile run", map[string]string{
			"Makefile": "build:\n\tgo build .\n\nrun: build\n\t./x\n",
		}, "make run", "Makefile"},
		{"Makefile dev before start", map[string]string{
			"Makefile": "start:\n\t./x\ndev:\n\t./x --dev\n",
		}, "make dev", "Makefile"},
		{"docker-compose.yml", map[string]string{
			"docker-compose.yml": "services: {}\n",
		}, "docker compose up", "docker-compose.yml"},
		{"compose.yaml", map[string]string{
			"compose.yaml": "services: {}\n",
		}, "docker compose up", "compose.yaml"},
		{"go.mod", map[string]string{
			"go.mod": "module x\n",
		}, "go run .", "go.mod"},
		{"pom.xml", map[string]string{
			"pom.xml": "<project/>\n",
		}, "mvn spring-boot:run", "pom.xml"},
		{"pubspec.yaml", map[string]string{
			"pubspec.yaml": "name: x\n",
		}, "flutter run", "pubspec.yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := tempDir(t)
			for name, body := range tc.files {
				write(t, filepath.Join(root, name), body)
			}
			got := discoverStart(root)
			if len(got) != 1 {
				t.Fatalf("discoverStart = %v, want exactly one candidate", commands(got))
			}
			if got[0].Command != tc.command || got[0].Evidence != tc.evidence {
				t.Errorf("candidate = %+v, want %q from %q", got[0], tc.command, tc.evidence)
			}
		})
	}
}

// Negative controls: files that look like evidence and are not.
func TestDiscoverStartIgnoresWhatIsNotAStartRoutine(t *testing.T) {
	cases := map[string]map[string]string{
		"an empty directory": {},
		"a package.json without a runnable script": {
			"package.json": `{"scripts":{"test":"jest","build":"tsc"}}`,
		},
		"a package.json that is not JSON": {
			"package.json": `{"scripts":`,
		},
		"a Makefile with only a build target": {
			"Makefile": "build:\n\tgo build .\n",
		},
		"a Makefile whose targets merely contain the words": {
			"Makefile": "restart:\n\t./x\ndevices:\n\t./y\n",
		},
		"a scripts directory with other scripts": {
			"scripts/bench.sh":   "#!/bin/sh\n",
			"scripts/starter.sh": "#!/bin/sh\n",
		},
		"a service.yaml without spec.start": {
			"service.yaml": "apiVersion: agnostic-stack/v1\nkind: Service\nspec:\n  runtime:\n    build: maven\n",
		},
		"a start: key outside spec": {
			"service.yaml": "apiVersion: agnostic-stack/v1\nkind: Service\nstart: nope\nmetadata:\n  start: nope\n",
		},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			root := tempDir(t)
			for name, body := range files {
				write(t, filepath.Join(root, name), body)
			}
			if got := discoverStart(root); len(got) != 0 {
				t.Errorf("discoverStart = %v, want nothing", commands(got))
			}
		})
	}
}

// Every hit is reported, in rule order, because the count is the finding: one
// hit is an answer, four hits are an ambiguity worth naming.
func TestDiscoverStartReportsEveryHitInRuleOrder(t *testing.T) {
	root := tempDir(t)
	write(t, filepath.Join(root, "scripts", "start-nuncio.sh"), "")
	write(t, filepath.Join(root, "scripts", "start-circlead.sh"), "")
	write(t, filepath.Join(root, "scripts", "start.sh"), "")
	write(t, filepath.Join(root, "docker-compose.yml"), "")
	write(t, filepath.Join(root, "pom.xml"), "")
	got := commands(discoverStart(root))
	want := []string{
		"scripts/start.sh",
		"scripts/start-circlead.sh",
		"scripts/start-nuncio.sh",
		"docker compose up",
		"mvn spring-boot:run",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("discoverStart = %v, want %v", got, want)
	}
}

func TestDiscoverStartIsReadOnly(t *testing.T) {
	root := tempDir(t)
	discoverStart(root)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("discovery must not create anything, found %d entries", len(entries))
	}
}

// --- declaration ----------------------------------------------------------

func TestStartIsDiscoveredWhenNothingIsDeclared(t *testing.T) {
	root := tempDir(t)
	write(t, filepath.Join(root, "scripts", "start.sh"), "")
	c := &Context{Root: root, Label: "x", Source: SourceLimen}
	s := c.Start()
	if s == nil {
		t.Fatal("Start() = nil, want the discovered routine")
	}
	if s.Command != "scripts/start.sh" || s.Source != "discovered" || s.Evidence != "scripts/start.sh" {
		t.Errorf("Start() = %+v", s)
	}
	if s.Ambiguous || s.Missing {
		t.Errorf("one hit is neither ambiguous nor missing: %+v", s)
	}
}

func TestStartIsNilWhenThereIsNothing(t *testing.T) {
	c := &Context{Root: tempDir(t), Label: "docs", Source: SourceLimen}
	if s := c.Start(); s != nil {
		t.Errorf("Start() = %+v, want nil for a directory with nothing to start", s)
	}
}

func TestStartSeveralHitsAreAmbiguousUntilDeclared(t *testing.T) {
	root := tempDir(t)
	write(t, filepath.Join(root, "scripts", "start-circlead.sh"), "")
	write(t, filepath.Join(root, "scripts", "start-stack.sh"), "")
	write(t, filepath.Join(root, "docker-compose.yml"), "")
	c := &Context{Root: root, Label: "circlead", Source: SourceLimen}

	s := c.Start()
	if !s.Ambiguous {
		t.Fatalf("three hits and no declaration must be ambiguous: %+v", s)
	}
	if s.Command != "scripts/start-circlead.sh" || len(s.Candidates) != 3 {
		t.Errorf("the first hit still wins the cell: %+v", s)
	}

	// A declaration silences the ambiguity without switching discovery off.
	c.Meta = &Meta{Start: "scripts/start-stack.sh dev"}
	s = c.Start()
	if s.Ambiguous {
		t.Errorf("declared, therefore not ambiguous: %+v", s)
	}
	if s.Command != "scripts/start-stack.sh dev" || s.Source != "declared" || s.Evidence != ".limen/meta.yaml" {
		t.Errorf("Start() = %+v, want the declared command", s)
	}
	if len(s.Candidates) != 3 {
		t.Errorf("the candidates stay visible for --verbose: %+v", s.Candidates)
	}
}

// Same two-layer precedence as devEndpoints: the committed declaration is what
// every clone agrees on, the descriptor is what this machine needs.
func TestDescriptorStartOverridesMeta(t *testing.T) {
	root := tempDir(t)
	write(t, filepath.Join(root, "scripts", "start.sh"), "")
	c := &Context{Root: root, Label: "x", Source: SourceLimen,
		Meta: &Meta{Start: "scripts/start.sh"}}
	c.set("start", "scripts/start.sh --skip-tests")
	s := c.Start()
	if s.Command != "scripts/start.sh --skip-tests" || s.Evidence != ".limen/limen.yaml" {
		t.Errorf("Start() = %+v, want the machine-local declaration", s)
	}
}

// A rename must not fail silently: the declaration names a script, the script
// is gone, and status is where that shows.
func TestDeclaredStartThatIsGoneIsReportedMissing(t *testing.T) {
	root := tempDir(t)
	c := &Context{Root: root, Label: "x", Source: SourceLimen,
		Meta: &Meta{Start: "scripts/start-old.sh dev"}}
	s := c.Start()
	if s == nil || !s.Missing {
		t.Fatalf("Start() = %+v, want missing=true", s)
	}
	if s.Command != "scripts/start-old.sh dev" {
		t.Errorf("the declared command is still reported so the drift can be named: %+v", s)
	}

	write(t, filepath.Join(root, "scripts", "start-old.sh"), "")
	if s := c.Start(); s.Missing {
		t.Errorf("the script exists again, nothing is missing: %+v", s)
	}
}

// `npm run dev` or `make run` name no file; there is nothing on disk to check
// and nothing to report as missing.
func TestDeclaredStartWithoutAPathIsNeverMissing(t *testing.T) {
	c := &Context{Root: tempDir(t), Label: "x", Source: SourceLimen,
		Meta: &Meta{Start: "npm run dev"}}
	if s := c.Start(); s == nil || s.Missing {
		t.Errorf("Start() = %+v, want declared and not missing", s)
	}
}

// --- rendering ------------------------------------------------------------

func TestRenderShowNamesTheStartRoutine(t *testing.T) {
	root := tempDir(t)
	write(t, filepath.Join(root, "package.json"), `{"scripts":{"dev":"vite"}}`)
	c := &Context{Root: root, Label: "x", Source: SourceLimen}
	var buf strings.Builder
	RenderShow(&buf, c, fixedResolver{})
	if !strings.Contains(buf.String(), "start:        npm run dev") {
		t.Errorf("show must name the start routine:\n%s", buf.String())
	}

	write(t, filepath.Join(root, "scripts", "start.sh"), "")
	buf.Reset()
	RenderShow(&buf, c, fixedResolver{})
	if !strings.Contains(buf.String(), "(+1)") {
		t.Errorf("show must say when discovery is ambiguous:\n%s", buf.String())
	}
}

func TestRenderShowMarksAMissingStart(t *testing.T) {
	c := &Context{Root: tempDir(t), Label: "x", Source: SourceLimen,
		Meta: &Meta{Start: "scripts/gone.sh"}}
	var buf strings.Builder
	RenderShow(&buf, c, fixedResolver{})
	if !strings.Contains(buf.String(), "!!") || !strings.Contains(buf.String(), "scripts/gone.sh") {
		t.Errorf("a declared script that is gone must be flagged:\n%s", buf.String())
	}
}

func TestRenderJSONCarriesStart(t *testing.T) {
	root := tempDir(t)
	write(t, filepath.Join(root, "scripts", "start.sh"), "")
	write(t, filepath.Join(root, "go.mod"), "module x\n")
	c := &Context{Root: root, Label: "x", Source: SourceLimen}
	var buf strings.Builder
	if err := RenderJSON(&buf, c, fixedResolver{}); err != nil {
		t.Fatal(err)
	}
	var v struct {
		Start *Start `json:"start"`
	}
	if err := json.Unmarshal([]byte(buf.String()), &v); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, buf.String())
	}
	if v.Start == nil || v.Start.Command != "scripts/start.sh" || !v.Start.Ambiguous {
		t.Errorf("start = %+v", v.Start)
	}
	if len(v.Start.Candidates) != 2 || v.Start.Candidates[1].Command != "go run ." {
		t.Errorf("candidates = %+v", v.Start.Candidates)
	}
}

func TestRenderJSONStartIsNullWithoutARoutine(t *testing.T) {
	c := &Context{Root: tempDir(t), Label: "docs", Source: SourceLimen}
	var buf strings.Builder
	if err := RenderJSON(&buf, c, fixedResolver{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"start":null`) {
		t.Errorf("expected an explicit null start:\n%s", buf.String())
	}
}

// The hook path must not pay for discovery: finish() runs on every cd, the
// start routine is only wanted by show, json, list and status.
func TestStartIsNotResolvedOnTheHookPath(t *testing.T) {
	root, nested := project(t, "label: x\n")
	write(t, filepath.Join(root, "scripts", "start.sh"), "")
	c, ok := Discover(nested)
	if !ok {
		t.Fatal("no context")
	}
	var buf strings.Builder
	RenderShell(&buf, c, fixedResolver{})
	if strings.Contains(buf.String(), "start") {
		t.Errorf("shell exports nothing about the start routine:\n%s", buf.String())
	}
}

// --- CLI ------------------------------------------------------------------

func TestCLIShowAndListReportTheStartRoutine(t *testing.T) {
	env := sharedState(t)
	root := tempDir(t)
	write(t, filepath.Join(root, ".limen", "limen.yaml"), "label: circlead\n")
	write(t, filepath.Join(root, ".limen", "meta.yaml"), "start: scripts/start-circlead.sh dev\n")
	write(t, filepath.Join(root, "scripts", "start-circlead.sh"), "")
	write(t, filepath.Join(root, "scripts", "start-stack.sh"), "")
	runLimen(t, root, env, "register")

	r := runLimen(t, root, env, "show")
	if !strings.Contains(r.stdout, "start:        scripts/start-circlead.sh dev") {
		t.Errorf("show:\n%s", r.stdout)
	}
	r = runLimen(t, tempDir(t), env, "list", "--json")
	if !strings.Contains(r.stdout, `"start":{"command":"scripts/start-circlead.sh dev","source":"declared","evidence":".limen/meta.yaml"`) {
		t.Errorf("list --json must carry the start routine:\n%s", r.stdout)
	}
}
