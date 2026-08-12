package main

// Some projects carry a service.yaml — the agnostic-stack descriptor that says
// what the thing actually offers. Limen neither owns nor writes it; it only
// reports that it is there.
//
// That is deliberately not a `kind:` field in the limen descriptor. The
// service file already carries `kind:` itself, so a hand-maintained copy would
// be a second truth that can drift from the first: agnostic-stack-tests says
// `TestOrchestrator`, and a copied field would most likely have said "service".
// Discovering beats duplicating — it costs nothing to maintain and cannot
// disagree with the file it describes.

import (
	"bufio"
	"os"
	"path/filepath"
)

// Service is the little that Limen reads out of a service.yaml: enough for an
// agent to tell a digital service from a folder of documents, and to know
// which kind it is. Everything else stays in the file, where it belongs.
type Service struct {
	APIVersion string `json:"api_version"`
	Kind       string `json:"kind"`
	File       string `json:"file"`
}

// readService looks for a service descriptor next to the context root.
//
// It parses with the same flat reader as the descriptor but through its own
// switch, never through Context.set: service.yaml has a nested `metadata:`
// block whose `name:` would otherwise land in the actor field. Only the two
// top-level keys are taken, and reading stops as soon as both are in hand —
// they are the first lines of the file.
func readService(root string) *Service {
	for _, name := range []string{"service.yaml", "service.yml"} {
		path := filepath.Join(root, name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		svc := &Service{File: name}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			key, val, ok := parseLine(sc.Text())
			if !ok {
				continue
			}
			switch key {
			case "apiversion":
				svc.APIVersion = val
			case "kind":
				svc.Kind = val
			}
			if svc.APIVersion != "" && svc.Kind != "" {
				break
			}
		}
		f.Close()
		// A file without `kind:` is not a service descriptor — reporting it
		// would tell an agent something that is not there.
		if svc.Kind == "" {
			continue
		}
		return svc
	}
	return nil
}

// HasService reports an adjacent service descriptor.
func (c *Context) HasService() bool { return c.Service != nil }
