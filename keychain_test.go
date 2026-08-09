package main

import "testing"

func TestResolveOnlyForAnthropicProvider(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-env")
	r := NewSystemKeyResolver()

	if _, _, ok := r.Resolve(&Context{Provider: "ollama"}); ok {
		t.Error("a non-anthropic provider must not resolve an anthropic key")
	}
	if _, _, ok := r.Resolve(&Context{Provider: ""}); ok {
		t.Error("an unset provider must not resolve")
	}
	if _, _, ok := r.Resolve(nil); ok {
		t.Error("nil context must not resolve")
	}
}

func TestResolvePrefersTheEnvironmentOverTheKeychain(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-env")
	called := false
	r := &SystemKeyResolver{Lookup: func(string, string) (string, bool) {
		called = true
		return "sk-from-keychain", true
	}}

	v, source, ok := r.Resolve(&Context{Provider: "anthropic", Actor: "Leo"})
	if !ok || v != "sk-from-env" {
		t.Fatalf("got (%q, %v), want the environment value", v, ok)
	}
	if source != "ANTHROPIC_API_KEY" {
		t.Errorf("source = %q", source)
	}
	if called {
		t.Error("the keychain must not be touched when the environment already has a key")
	}
}

func TestResolveFallsBackToTheKeychain(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	var gotService, gotAccount string
	r := &SystemKeyResolver{Lookup: func(service, account string) (string, bool) {
		gotService, gotAccount = service, account
		return "sk-from-keychain", true
	}}

	v, source, ok := r.Resolve(&Context{
		Provider: "anthropic", Actor: "Leo", KeychainService: "limen-anthropic",
	})
	if !ok || v != "sk-from-keychain" {
		t.Fatalf("got (%q, %v)", v, ok)
	}
	if source != "keychain limen-anthropic" {
		t.Errorf("source = %q", source)
	}
	if gotService != "limen-anthropic" || gotAccount != "Leo" {
		t.Errorf("looked up service=%q account=%q, want limen-anthropic/Leo", gotService, gotAccount)
	}
}

func TestKeychainServiceAndAccountDefaults(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	var gotService, gotAccount string
	r := &SystemKeyResolver{Lookup: func(service, account string) (string, bool) {
		gotService, gotAccount = service, account
		return "k", true
	}}
	// No keychainService: derived from the provider. No keychainAccount: the actor.
	r.Resolve(&Context{Provider: "anthropic", Actor: "Matthias"})
	if gotService != "limen-anthropic" {
		t.Errorf("default service = %q, want limen-anthropic", gotService)
	}
	if gotAccount != "Matthias" {
		t.Errorf("default account = %q, want the actor", gotAccount)
	}
}

func TestResolveNeedsAnAccount(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	called := false
	r := &SystemKeyResolver{Lookup: func(string, string) (string, bool) {
		called = true
		return "k", true
	}}
	if _, _, ok := r.Resolve(&Context{Provider: "anthropic"}); ok {
		t.Error("without an actor or keychainAccount there is nothing to look up")
	}
	if called {
		t.Error("must not call the keychain without an account")
	}
}

func TestKeySourceWording(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	miss := &SystemKeyResolver{Lookup: func(string, string) (string, bool) { return "", false }}

	if got := KeySource(&Context{Provider: "ollama"}, miss); got != "n/a (provider=ollama)" {
		t.Errorf("got %q", got)
	}
	if got := KeySource(&Context{Provider: ""}, miss); got != "n/a (provider=unset)" {
		t.Errorf("got %q", got)
	}
	if got := KeySource(&Context{Provider: "anthropic", Actor: "Leo"}, miss); got != "not resolvable" {
		t.Errorf("got %q", got)
	}
}
