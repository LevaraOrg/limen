package main

// The start routine: how the thing in this directory is brought up.
//
// It is derived, not declared. service.go states the principle — discovering
// beats duplicating — and a hand-maintained `start:` in every descriptor would
// be a second truth that drifts from package.json the first time a script is
// renamed. So the routine is read off the files that are there, in a fixed
// order, first hit wins. Every hit is reported, not only the winner, because
// the count is the interesting part: one hit is an answer, four hits are an
// ambiguity worth naming.
//
// That ambiguity is the only case where a declaration earns its place:
//
//	.limen/meta.yaml   start: scripts/start-circlead.sh dev              — committed
//	.limen/limen.yaml  start: scripts/start-circlead.sh dev --with-sidecars — machine-local
//
// Same two-layer precedence as devEndpoints, for the same reason. A declared
// start silences the ambiguity; it does not switch discovery off, and a
// declaration naming a script that is no longer on disk is reported as such —
// a rename must not fail silently.
//
// Limen prints the start command. Limen never runs it. A tool whose job is to
// describe a directory must not acquire the power to execute what it finds
// there; running things is clavo's subject.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// StartCandidate is one way discovery found to start the directory: the
// command a human would type, and the file that is the evidence for it.
type StartCandidate struct {
	Command  string `json:"command"`
	Evidence string `json:"evidence"`
}

// Start is the resolved start routine of a context. Nil when there is nothing
// to start — a document repository must not pretend to be a service.
type Start struct {
	// Command is what to type: the declaration when there is one, otherwise
	// the first discovered candidate.
	Command string `json:"command"`
	// Source is "declared" or "discovered".
	Source string `json:"source"`
	// Evidence is the file the command comes from: the descriptor or meta file
	// for a declaration, the discovered file otherwise.
	Evidence string `json:"evidence"`
	// Ambiguous is set when discovery found several candidates and nothing is
	// declared to pick one.
	Ambiguous bool `json:"ambiguous,omitempty"`
	// Missing is set when the declaration names a path that is not on disk.
	Missing bool `json:"missing,omitempty"`
	// Candidates is every discovery hit in rule order, always non-nil.
	Candidates []StartCandidate `json:"candidates"`
}

// discoverStart reads the start routine off the files in root. Pure over the
// directory: nothing is created, nothing is run. Candidates come back in rule
// order, so the first one is the winner.
func discoverStart(root string) []StartCandidate {
	cands := []StartCandidate{}
	add := func(command, evidence string) {
		cands = append(cands, StartCandidate{Command: command, Evidence: evidence})
	}

	// 1. service.yaml → spec.start. The service descriptor already carries the
	// build and the healthcheck; whether it should carry the run command too is
	// an agnostic-stack decision, so limen reads the key if present and never
	// asks for it.
	if spec := readServiceSpec(root); spec.Start != "" {
		add(spec.Start, spec.File)
	}

	// 2. scripts/start.sh, 3. scripts/start-<label>.sh — one candidate each,
	// because between three start scripts discovery cannot and should not guess.
	if fileExists(filepath.Join(root, "scripts", "start.sh")) {
		add("scripts/start.sh", "scripts/start.sh")
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "scripts", "start-*.sh")); len(matches) > 0 {
		sort.Strings(matches)
		for _, m := range matches {
			if fileExists(m) {
				rel := filepath.ToSlash(filepath.Join("scripts", filepath.Base(m)))
				add(rel, rel)
			}
		}
	}

	// 4. package.json → scripts.dev, else start, else serve.
	if name := packageScript(filepath.Join(root, "package.json")); name != "" {
		add("npm run "+name, "package.json")
	}

	// 5. Makefile → run, else dev, else start.
	if target := makeTarget(filepath.Join(root, "Makefile")); target != "" {
		add("make "+target, "Makefile")
	}

	// 6. A compose file, under any of its four spellings.
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yaml", "compose.yml"} {
		if fileExists(filepath.Join(root, name)) {
			add("docker compose up", name)
			break
		}
	}

	// 7. The build system's own run verb.
	for _, rule := range []struct{ file, command string }{
		{"go.mod", "go run ."},
		{"pom.xml", "mvn spring-boot:run"},
		{"pubspec.yaml", "flutter run"},
	} {
		if fileExists(filepath.Join(root, rule.file)) {
			add(rule.command, rule.file)
		}
	}
	return cands
}

// packageScript names the first runnable script a package.json declares, in
// the order a developer would try them. Empty when the file is absent,
// unreadable or has none of them — a package that only builds and tests has
// nothing to start.
func packageScript(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return ""
	}
	for _, name := range []string{"dev", "start", "serve"} {
		if pkg.Scripts[name] != "" {
			return name
		}
	}
	return ""
}

// makeTarget names the first of run, dev, start that the Makefile defines as
// a target — a line starting with the bare name and a colon. Targets that
// merely contain the word (restart, devices) do not count.
func makeTarget(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	defined := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		name, _, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		defined[strings.TrimSpace(name)] = true
	}
	for _, name := range []string{"run", "dev", "start"} {
		if defined[name] {
			return name
		}
	}
	return ""
}

// serviceSpec is the part of service.yaml's spec: block that the start routine
// and the status need. Read on demand, never on the hook path: service.go
// skims the two top-level keys on every cd, this walks the file once when
// somebody asks how the service starts or whether it is healthy.
type serviceSpec struct {
	File            string
	Start           string
	HealthcheckPath string
}

// readServiceSpec walks the file with an indentation stack, so `start:` is
// only taken under `spec:` and `path:` only under `spec.healthcheck:`. A
// `start:` at the top level or under metadata: is somebody else's key.
func readServiceSpec(root string) serviceSpec {
	for _, name := range []string{"service.yaml", "service.yml"} {
		f, err := os.Open(filepath.Join(root, name))
		if err != nil {
			continue
		}
		spec := serviceSpec{File: name}
		type frame struct {
			indent int
			key    string
		}
		stack := []frame{}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			raw := sc.Text()
			line := strings.TrimLeft(raw, " \t")
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			key, val, found := strings.Cut(line, ":")
			if !found || !keyPattern.MatchString(strings.TrimSpace(key)) {
				continue
			}
			indent := len(raw) - len(line)
			for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, frame{indent, strings.TrimSpace(key)})
			path := make([]string, len(stack))
			for i, fr := range stack {
				path[i] = fr.key
			}
			val = strings.TrimSpace(val)
			if h := strings.Index(val, " #"); h >= 0 {
				val = strings.TrimSpace(val[:h])
			}
			val = strings.Trim(val, `"'`)
			switch strings.Join(path, ".") {
			case "spec.start":
				spec.Start = val
			case "spec.healthcheck.path":
				spec.HealthcheckPath = val
			}
		}
		f.Close()
		return spec
	}
	return serviceSpec{}
}

// StartRaw is the declaration exactly as written, descriptor over meta.
func (c *Context) StartRaw() (command, file string) {
	if c == nil {
		return "", ""
	}
	if c.start != "" {
		return c.start, filepath.ToSlash(filepath.Join(".limen", "limen.yaml"))
	}
	if c.Meta != nil && c.Meta.Start != "" {
		return c.Meta.Start, filepath.ToSlash(filepath.Join(".limen", "meta.yaml"))
	}
	return "", ""
}

// Start resolves the routine: declaration over discovery. Nil when neither
// yields anything.
func (c *Context) Start() *Start {
	if c == nil {
		return nil
	}
	cands := discoverStart(c.Root)
	declared, file := c.StartRaw()
	if declared == "" {
		if len(cands) == 0 {
			return nil
		}
		return &Start{
			Command:    cands[0].Command,
			Source:     "discovered",
			Evidence:   cands[0].Evidence,
			Ambiguous:  len(cands) > 1,
			Candidates: cands,
		}
	}
	return &Start{
		Command:    declared,
		Source:     "declared",
		Evidence:   file,
		Missing:    startMissing(c.Root, declared),
		Candidates: cands,
	}
}

// startMissing reports a declared command whose program is a path — carries
// a slash — that does not exist under root. `npm run dev` names no file and
// can never be missing; `scripts/start-old.sh dev` after a rename can.
func startMissing(root, command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 || !strings.Contains(fields[0], "/") {
		return false
	}
	program := fields[0]
	if !filepath.IsAbs(program) {
		program = filepath.Join(root, program)
	}
	return !fileExists(program)
}

// Cell is the start routine as one table cell: the command, and how many
// other candidates discovery found when nothing is declared to pick one.
func (s *Start) Cell() string {
	if s == nil {
		return "✗ none"
	}
	cell := s.Command
	if s.Missing {
		cell = "!! " + cell + " (missing)"
	}
	if s.Ambiguous {
		cell += " (+" + strconv.Itoa(len(s.Candidates)-1) + ")"
	}
	return cell
}
