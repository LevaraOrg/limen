package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Source records which file shape the context was read from. A legacy .orca/
// tree still loads unchanged, so step 1 of the migration breaks nothing.
type Source string

const (
	SourceLimen Source = "limen"
	SourceOrca  Source = "orca"
)

// Context is everything Limen knows about the directory it was called in.
// The API key is deliberately absent: it is resolved on demand in keychain.go
// so that `prompt` and `json` cannot leak it and do not pay for a lookup.
type Context struct {
	Root          string
	Source        Source
	Label         string
	Actor         string
	GithubUser    string
	ClaudeDir     string
	GcloudAccount string
	GcloudProject string
	Provider      string
	Model         string
	Gateway       string

	// Purpose and Topics are for agents, not for the shell: one line about what
	// this tree is for, and a comma-separated topic list. They are what lets a
	// reader of `limen list` match a loose note to the right directory — the
	// matching itself stays outside the binary.
	Purpose string
	Topics  string

	KeychainService string
	KeychainAccount string

	// PlaintextKey holds a key found in the config file. It is never rendered;
	// its presence is reported as a warning, because a key in a committed file
	// is a defect rather than a feature.
	PlaintextKey string
}

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// Discover walks upward from dir looking for a context. Calling Limen deep in a
// project tree must yield that project's context — that is the whole point of
// the command, so the search direction is up, not down.
func Discover(dir string) (*Context, bool) {
	for {
		if fileExists(filepath.Join(dir, ".limen.yaml")) {
			ctx := &Context{Root: dir, Source: SourceLimen}
			ctx.applyFile(filepath.Join(dir, ".limen.yaml"))
			ctx.finish()
			return ctx, true
		}
		orcaCfg := filepath.Join(dir, ".orca", "config.yaml")
		orcaID := filepath.Join(dir, ".orca", "identity.yaml")
		if fileExists(orcaCfg) || fileExists(orcaID) {
			ctx := &Context{Root: dir, Source: SourceOrca}
			ctx.applyFile(orcaID)
			ctx.applyFile(orcaCfg)
			ctx.finish()
			return ctx, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, false
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// applyFile reads a flat key: value file. Deliberately not a YAML library: a
// context descriptor needs no nesting, and pulling in a parser for more would
// bring back the startup cost this tool exists to avoid.
func (c *Context) applyFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		key, val, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		c.set(key, val)
	}
}

// parseLine returns the normalised key and value of one line, or ok=false when
// the line carries no assignment.
func parseLine(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
		return "", "", false
	}
	// Indented lines belong to a nested block, which has no place here.
	if line != strings.TrimLeft(line, " \t") {
		return "", "", false
	}
	i := strings.Index(line, ":")
	if i < 0 {
		return "", "", false
	}

	key := strings.TrimSpace(line[:i])
	val := strings.TrimSpace(line[i+1:])
	if !keyPattern.MatchString(key) {
		return "", "", false
	}

	quoted := strings.HasPrefix(val, `"`) || strings.HasPrefix(val, `'`)
	if !quoted {
		// A trailing comment, but only outside quotes.
		if h := strings.Index(val, "#"); h >= 0 {
			val = strings.TrimRight(val[:h], " \t")
		}
	} else if len(val) >= 2 {
		if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) ||
			(strings.HasPrefix(val, `'`) && strings.HasSuffix(val, `'`)) {
			val = val[1 : len(val)-1]
		}
	}
	if val == "" {
		return "", "", false
	}

	norm := strings.ToLower(strings.ReplaceAll(key, "-", ""))
	norm = strings.ReplaceAll(norm, "_", "")
	return norm, val, true
}

func (c *Context) set(key, val string) {
	switch key {
	case "label":
		c.Label = val
	case "actor":
		if c.Actor == "" {
			c.Actor = val
		}
	case "name": // .orca/identity.yaml calls the actor `name`
		c.Actor = val
	case "githubuser":
		c.GithubUser = val
	case "claudeconfigdir":
		c.ClaudeDir = val
	case "gcloudaccount":
		c.GcloudAccount = val
	case "gcloudproject":
		c.GcloudProject = val
	case "provider":
		c.Provider = val
	case "model":
		c.Model = val
	case "gateway":
		c.Gateway = val
	case "purpose":
		c.Purpose = val
	case "topics":
		c.Topics = val
	case "keychainservice":
		c.KeychainService = val
	case "keychainaccount":
		c.KeychainAccount = val
	case "apikey":
		c.PlaintextKey = val
	}
}

func (c *Context) finish() {
	if c.Label == "" {
		c.Label = filepath.Base(c.Root)
	}
	c.ClaudeDir = expandTilde(c.ClaudeDir)
}

func expandTilde(p string) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// HasPlaintextKey reports a key sitting in the config file.
func (c *Context) HasPlaintextKey() bool { return c.PlaintextKey != "" }

// TopicList splits the comma-separated topics field. Always non-nil, so JSON
// renders [] rather than null and consumers can range over it blindly.
func (c *Context) TopicList() []string {
	list := []string{}
	for _, t := range strings.Split(c.Topics, ",") {
		if t = strings.TrimSpace(t); t != "" {
			list = append(list, t)
		}
	}
	return list
}
