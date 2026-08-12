package main

// Tests for service.yaml discovery. The point of discovering rather than
// declaring is that the two can never disagree — so the tests pin down that
// Limen reports what the file says, and that the file cannot bleed into the
// descriptor.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const serviceYAML = `apiVersion: agnostic-stack/v1
kind: Service
metadata:
  name: nuncio-the-service-not-the-actor
  title: "Nuncio LLM Gateway"
  owner: someone-else
spec:
  system: agnostic-stack
`

func TestServiceIsDiscoveredNextToTheDescriptor(t *testing.T) {
	root, _ := project(t, "label: nuncio\nactor: Matthias\n")
	write(t, filepath.Join(root, "service.yaml"), serviceYAML)

	ctx, ok := Discover(root)
	if !ok {
		t.Fatal("expected a context")
	}
	if !ctx.HasService() {
		t.Fatal("service.yaml not discovered")
	}
	if ctx.Service.Kind != "Service" || ctx.Service.APIVersion != "agnostic-stack/v1" {
		t.Errorf("Service = %+v", ctx.Service)
	}
	if ctx.Service.File != "service.yaml" {
		t.Errorf("File = %q", ctx.Service.File)
	}

	// The whole reason for discovering instead of copying: the service file
	// has its own nested metadata, and none of it may reach the descriptor.
	if ctx.Actor != "Matthias" {
		t.Errorf("Actor = %q — service.yaml leaked into the descriptor", ctx.Actor)
	}
	if ctx.Label != "nuncio" {
		t.Errorf("Label = %q", ctx.Label)
	}
}

func TestServiceKindIsReportedVerbatim(t *testing.T) {
	// agnostic-stack-tests says TestOrchestrator, not Service. A copied field
	// would most likely have said "service" — this is the drift that
	// discovery prevents, so the reported value must be the file's own.
	root, _ := project(t, "label: agnostic-stack-tests\n")
	write(t, filepath.Join(root, "service.yaml"),
		"apiVersion: agnostic-stack/v1\nkind: TestOrchestrator\n")

	ctx, _ := Discover(root)
	if !ctx.HasService() || ctx.Service.Kind != "TestOrchestrator" {
		t.Fatalf("Service = %+v, want kind TestOrchestrator", ctx.Service)
	}
}

func TestServiceAbsentOrWithoutKindIsNotReported(t *testing.T) {
	// No file at all.
	root, _ := project(t, "label: plain\n")
	ctx, _ := Discover(root)
	if ctx.HasService() {
		t.Errorf("reported a service where there is none: %+v", ctx.Service)
	}

	// A file without `kind:` is not a service descriptor; claiming otherwise
	// would tell an agent something that is not there.
	root2, _ := project(t, "label: kindless\n")
	write(t, filepath.Join(root2, "service.yaml"), "apiVersion: agnostic-stack/v1\n")
	ctx2, _ := Discover(root2)
	if ctx2.HasService() {
		t.Errorf("a file without kind: must not count: %+v", ctx2.Service)
	}
}

func TestServiceYmlSpellingIsAlsoFound(t *testing.T) {
	root, _ := project(t, "label: ymlspelling\n")
	write(t, filepath.Join(root, "service.yml"), "apiVersion: v1\nkind: Service\n")
	ctx, _ := Discover(root)
	if !ctx.HasService() || ctx.Service.File != "service.yml" {
		t.Fatalf("Service = %+v", ctx.Service)
	}
}

func TestCLIReportsTheServiceToAgents(t *testing.T) {
	root, nested := project(t, fullConfig)
	write(t, filepath.Join(root, "service.yaml"), serviceYAML)
	env := sharedState(t)
	runLimen(t, nested, env, "register")

	// show: readable, for a human standing in the directory.
	if r := runLimen(t, nested, env, "show"); !strings.Contains(r.stdout, "Service (agnostic-stack/v1") {
		t.Errorf("show missing the service line:\n%s", r.stdout)
	}
	// json and list --json: the routing inventory an agent reads.
	for _, args := range [][]string{{"json"}, {"list", "--json"}} {
		r := runLimen(t, nested, env, args...)
		if !strings.Contains(r.stdout, `"kind":"Service"`) {
			t.Errorf("%v missing the service block:\n%s", args, r.stdout)
		}
		// The actor must still be the descriptor's, never the service file's.
		if strings.Contains(r.stdout, "someone-else") {
			t.Errorf("%v leaked service.yaml metadata:\n%s", args, r.stdout)
		}
	}

	// Without a service file the field is explicitly null, so a consumer can
	// tell "no service" from "field missing".
	_, nested2 := project(t, fullConfig)
	if r := runLimen(t, nested2, nil, "json"); !strings.Contains(r.stdout, `"service":null`) {
		t.Errorf("expected an explicit null service:\n%s", r.stdout)
	}
}

func TestServiceDiscoveryDoesNotSlowTheHookPath(t *testing.T) {
	// finish() runs on every cd. Reading two top-level keys out of a file
	// that is usually absent must stay invisible next to the 5.9 ms budget;
	// this pins the order of magnitude rather than a wall-clock number.
	root, _ := project(t, fullConfig)
	write(t, filepath.Join(root, "service.yaml"), serviceYAML)

	const n = 200
	for i := 0; i < n; i++ {
		if svc := readService(root); svc == nil {
			t.Fatal("service vanished")
		}
	}
	// Also prove the absent case does not error or allocate a Service.
	empty := t.TempDir()
	if svc := readService(empty); svc != nil {
		t.Fatalf("readService on an empty dir = %+v", svc)
	}
	if _, err := os.Stat(filepath.Join(empty, "service.yaml")); !os.IsNotExist(err) {
		t.Error("readService must not create anything")
	}
}
