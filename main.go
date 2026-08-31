// limen — ‹limen›, the threshold. It says which identity applies behind this
// directory threshold, and exports it.
package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

const version = "0.14.0"

const usage = `limen ` + version + ` — context and identity per directory

  limen show            readable overview
  limen json            machine-readable
  limen shell           export lines for  eval "$(limen shell)"
  limen prompt          one-line segment for RPROMPT / status line
  limen root            path of the project root directory
  limen list [--json]   every context registered on this machine
  limen register [path…] take a context into the registry (the shell hook does
                        this by itself on entry)
  limen note [--at label] "text"
                        append a dated note to .limen/notes.md
  limen backlog [--json] open notes across all contexts — where something is to
                        be done; a line reading "- ✓ …" counts as checked off
  limen ports [--json|--caddy] [--write <path>]
                        the development-endpoint allocation across all contexts;
                        --caddy renders the reverse-proxy sites, --write puts
                        them in a file only when they changed. Exit 1 when two
                        endpoints claim the same port or hostname — and then
                        --write refuses, so a watching proxy never sees it
  limen profile         inherited norms: what applies here, is it current,
                        and which skills are paused (pausedSkills: in meta.yaml)
    … install <source>  fetch an Agent Plugins package (path or git URL)
    … sync [--dry-run]  materialise skills and ADRs into the project
    … check             exit 1 on drift — for pre-commit or CI
    … list              what the store holds
  limen init            create .limen/limen.yaml in the current directory
  limen migrate [path…] lift onto the .limen/ layout — moves a flat .limen.yaml
                        along with LIMEN.md/LIMEN-META.yaml, otherwise creates
                        anew. --dry-run only shows.
  limen keychain-import move a plaintext key into the keychain
  limen hook zsh|bash   shell hook to wire in

The search runs upward for .limen/limen.yaml, then for a flat .limen.yaml (old
layout). Without a context: json prints {}, shell stays silent, both with
exit 0 — so the call can be made unconditionally from a shell startup file.

Everything limen owns lives in .limen/: limen.yaml is the descriptor (hard
truth, never written by tools, machine-local), notes.md collects loose thoughts,
meta.yaml the hard context facts — among them profiles:, the inherited norms,
and pausedSkills:, the ones deliberately switched off here.
profiles.lock records what was materialised. meta.yaml can never set identity;
it is repository content.

Development endpoints are declared with devEndpoints: — a list of
[name=]port [stream], e.g.  5173, api=8081 stream. The first is the primary and
answers under devHost: (default <label>.localhost); a named one hangs off it as
<host>-<name>.localhost. stream stops the proxy buffering, which server-sent
events need. devPort: 8080 remains the shorthand for a single unnamed endpoint.
In meta.yaml it is what every clone agrees on, in limen.yaml what this machine
is free to use; the machine-local declaration replaces the committed one.
limen shell exports PORT, LIMEN_DEV_PORT/HOST and LIMEN_DEV_PORT_<NAME> per
named endpoint, and limen ports --caddy generates the matching Caddy sites, so
the service and the proxy read the same line.

If a service.yaml (agnostic-stack) sits alongside, its kind is read and
reported in show/json/list — discovered, not duplicated.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	out := bufio.NewWriter(stdout)
	defer out.Flush()

	cmd := "show"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "-h", "--help", "help":
		fmt.Fprint(out, usage)
		return 0
	case "-V", "--version":
		fmt.Fprintf(out, "limen %s\n", version)
		return 0
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "limen: %v\n", err)
		return 1
	}

	ctx, found := Discover(cwd)
	resolver := NewSystemKeyResolver()

	switch cmd {
	case "show":
		if !found {
			fmt.Fprintf(stderr, "limen: no .limen/limen.yaml above %s\n", cwd)
			return 1
		}
		RenderShow(out, ctx, resolver)

	case "json":
		if err := RenderJSON(out, ctxOrNil(ctx, found), resolver); err != nil {
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}

	case "shell":
		// Silence and exit 0 without a context: this runs from .zshrc.
		RenderShell(out, ctxOrNil(ctx, found), resolver)
		if found {
			// Crossing the threshold is what makes a root known: the hook runs
			// here on every cd, so every context ever entered appears in
			// `limen list` without anyone maintaining an index. Best effort —
			// a failure to record must never break a shell startup file.
			RegisterRoot(ctx.Root)
		}

	case "prompt":
		if found {
			fmt.Fprintln(out, Prompt(ctx))
		}

	case "root":
		if found {
			fmt.Fprintln(out, ctx.Root)
		}

	case "list":
		jsonOut := len(args) > 1 && args[1] == "--json"
		if err := CmdList(out, jsonOut); err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}

	case "register":
		if err := CmdRegister(out, args[1:], cwd); err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}

	case "backlog":
		jsonOut := len(args) > 1 && args[1] == "--json"
		if err := CmdBacklog(out, jsonOut); err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}

	case "ports":
		code, err := CmdPorts(out, args[1:])
		if err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}
		return code

	case "profile":
		code, err := CmdProfile(out, ctxOrNil(ctx, found), args[1:])
		if err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}
		return code

	case "note":
		if err := CmdNote(out, ctxOrNil(ctx, found), found, args[1:], time.Now()); err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}

	case "migrate":
		dirs := []string{}
		dryRun := false
		for _, a := range args[1:] {
			if a == "--dry-run" || a == "-n" {
				dryRun = true
				continue
			}
			dirs = append(dirs, a)
		}
		if err := CmdMigrate(out, dirs, dryRun); err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}

	case "init":
		if err := CmdInit(out, cwd); err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}

	case "keychain-import":
		if err := CmdKeychainImport(out, ctxOrNil(ctx, found)); err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}

	case "hook":
		shell := ""
		if len(args) > 1 {
			shell = args[1]
		}
		if err := CmdHook(out, shell); err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "limen: %v\n", err)
			return 1
		}

	default:
		fmt.Fprintf(stderr, "limen: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	return 0
}

func ctxOrNil(c *Context, found bool) *Context {
	if !found {
		return nil
	}
	return c
}
