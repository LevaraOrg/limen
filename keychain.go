package main

import (
	"os"
	"os/exec"
	"strings"
)

// KeyResolver looks up the API key. It is an interface so the tests can prove
// the resolution order without touching the real keychain.
type KeyResolver interface {
	Resolve(c *Context) (value string, source string, ok bool)
}

// SystemKeyResolver resolves from the environment first, then the macOS
// keychain. It never reads the key from the config file: a key that lives in a
// committed file is the problem, not the source.
type SystemKeyResolver struct {
	// Lookup is the keychain call, injectable for tests.
	Lookup func(service, account string) (string, bool)
}

func NewSystemKeyResolver() *SystemKeyResolver {
	return &SystemKeyResolver{Lookup: securityLookup}
}

func (r *SystemKeyResolver) Resolve(c *Context) (string, string, bool) {
	if c == nil || c.Provider != "anthropic" {
		return "", "", false
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); strings.TrimSpace(v) != "" {
		return v, "ANTHROPIC_API_KEY", true
	}

	service := c.KeychainService
	if service == "" {
		service = "limen-" + c.Provider
	}
	account := c.KeychainAccount
	if account == "" {
		account = c.Actor
	}
	if account == "" || r.Lookup == nil {
		return "", "", false
	}
	if v, ok := r.Lookup(service, account); ok && v != "" {
		return v, "keychain " + service, true
	}
	return "", "", false
}

// securityLookup shells out to security(1). macOS only; elsewhere the
// environment variable is the way in, which is what the README says.
func securityLookup(service, account string) (string, bool) {
	path, err := exec.LookPath("security")
	if err != nil {
		return "", false
	}
	out, err := exec.Command(path, "find-generic-password",
		"-s", service, "-a", account, "-w").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimRight(string(out), "\r\n"), true
}

// KeySource describes where the key would come from, for `show`.
func KeySource(c *Context, r KeyResolver) string {
	if c.Provider != "anthropic" {
		p := c.Provider
		if p == "" {
			p = "unset"
		}
		return "n/a (provider=" + p + ")"
	}
	if _, source, ok := r.Resolve(c); ok {
		return source
	}
	return "not resolvable"
}
