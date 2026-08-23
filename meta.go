package main

// .limen/meta.yaml is the committed half of the context. The descriptor next
// to it is machine-local and gitignored — it says who you are here. This file
// says what the project is bound to, and it is the same for everyone who
// clones the repository.
//
// The split matters for profiles in particular. Which norms a project inherits
// is a property of the project, not of the laptop it is checked out on, so the
// declaration cannot live in limen.yaml. It also must not be able to reach
// back: a repository that could set `actor:` or `githubUser:` would let a
// clone dictate the identity of whoever opened it. Hence the same arrangement
// service.go uses — the flat parser is shared, the key switch is not.

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Meta is the little that limen reads out of meta.yaml. Everything else in
// that file is for humans and for whatever tool owns the bounded-context
// schema; limen neither validates nor rewrites it.
type Meta struct {
	// File is the path it was read from, for messages.
	File string

	// Profiles is the raw comma-separated declaration, e.g.
	// "levara-baseline@1.0.0, house-style".
	Profiles string

	// SkillTarget and ADRTarget say where `limen profile sync` materialises
	// what a profile carries. Empty means the conventional default.
	SkillTarget string
	ADRTarget   string

	// PausedSkills names skills that a declared profile carries but this
	// project does not want. Comma-separated, e.g. "zoom-out, grill-me".
	PausedSkills string

	// Language is the declared working language for code and documentation.
	// Empty means the english default; see Context.Language.
	Language string
}

// metaNames are searched in order. The second is the pre-0.4 layout, where the
// file sat in the root as LIMEN-META.yaml; `limen migrate` lifts it, but a
// tree that has not been migrated must still be readable.
var metaNames = []string{
	filepath.Join(".limen", "meta.yaml"),
	filepath.Join(".limen", "meta.yml"),
	"LIMEN-META.yaml",
}

// readMeta looks for the committed context facts next to the descriptor.
// A missing file is not an error: most projects have nothing to declare.
func readMeta(root string) *Meta {
	for _, name := range metaNames {
		path := filepath.Join(root, name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		m := &Meta{File: name}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			key, val, ok := parseLine(sc.Text())
			if !ok {
				continue
			}
			// Deliberately its own switch and not Context.set: meta.yaml is
			// repository content and must not be able to set identity.
			switch key {
			case "profiles":
				m.Profiles = val
			case "skilltarget":
				m.SkillTarget = val
			case "adrtarget":
				m.ADRTarget = val
			case "pausedskills":
				m.PausedSkills = val
			case "language":
				m.Language = val
			}
		}
		f.Close()
		return m
	}
	return nil
}

// Profile is one inherited norm package: a name, and how tightly the project
// pins it. An empty Version means the project takes whatever is installed —
// pinning has to be a decision, not a side effect of writing the name down.
type Profile struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// String renders the declaration the way it is written in meta.yaml.
func (p Profile) String() string {
	if p.Version == "" {
		return p.Name
	}
	return p.Name + "@" + p.Version
}

// ProfileList splits the profiles field. Always non-nil, so JSON renders []
// rather than null and consumers can range over it blindly — the same
// arrangement TopicList uses.
func (c *Context) ProfileList() []Profile {
	list := []Profile{}
	if c == nil || c.Meta == nil {
		return list
	}
	for _, item := range strings.Split(c.Meta.Profiles, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, version, found := strings.Cut(item, "@")
		p := Profile{Name: strings.TrimSpace(name)}
		if found {
			p.Version = strings.TrimSpace(version)
		}
		if p.Name == "" {
			continue
		}
		list = append(list, p)
	}
	return list
}

// SkillTarget is where a profile's skills land in the project. The default is
// the directory Claude Code reads; a project that serves another client points
// it elsewhere with `skillTarget:` in meta.yaml.
func (c *Context) SkillTarget() string {
	if c != nil && c.Meta != nil && c.Meta.SkillTarget != "" {
		return c.Meta.SkillTarget
	}
	return ".claude/skills"
}

// ADRTarget is where the decision records land. Default is the conventional
// docs/adr; Tessera, for one, keeps them under .planning/adr instead.
func (c *Context) ADRTarget() string {
	if c != nil && c.Meta != nil && c.Meta.ADRTarget != "" {
		return c.Meta.ADRTarget
	}
	return "docs/adr"
}

// Language is the working language for code and documentation in this tree.
// English is the default rather than a mere convention (the tool-level twin of
// ADR-0001), so every context answers with a language even when none is
// declared. A project that works in another language says so in meta.yaml,
// where every clone sees the same declaration — like profiles, this is a
// property of the project, not of the machine it is checked out on.
func (c *Context) Language() string {
	if c != nil && c.Meta != nil && c.Meta.Language != "" {
		return c.Meta.Language
	}
	return "english"
}

// PausedSkillList names the skills this project deliberately does without.
//
// Pausing is the fine-grained half of inheriting a package: the package stays
// declared, one skill stops being materialised. What actually deactivates it is
// absence — a skill missing from the skill directory cannot be loaded, so no
// agent has to be told. The declaration exists so a human can see that the gap
// is a decision rather than an accident, and so `sync` can undo it.
//
// ADRs are untouched by this. A decision record explains why a norm exists and
// stays readable even where its enforcement is switched off.
func (c *Context) PausedSkillList() []string {
	list := []string{}
	if c == nil || c.Meta == nil {
		return list
	}
	for _, item := range strings.Split(c.Meta.PausedSkills, ",") {
		if item = strings.TrimSpace(item); item != "" {
			list = append(list, item)
		}
	}
	return list
}
