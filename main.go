// limen — ‹limen› Schwelle. Sagt, welche Identität hinter dieser
// Verzeichnisschwelle gilt, und exportiert sie.
//
// Ersetzt `orca env`. Derselbe Zweck, ohne die JVM: `orca env json` kostete auf
// dieser Maschine 530 ms, weshalb die alte WezTerm-Integration ihr Ergebnis 15
// Sekunden cachte und nach jedem Verzeichniswechsel daneben lag.
package main

import (
	"bufio"
	"fmt"
	"os"
)

const version = "0.2.0"

const usage = `limen ` + version + ` — Kontext und Identität pro Verzeichnis

  limen show            lesbare Übersicht
  limen json            maschinenlesbar
  limen shell           export-Zeilen für  eval "$(limen shell)"
  limen prompt          einzeiliges Segment für RPROMPT / Statuszeile
  limen root            Pfad des Projektwurzelverzeichnisses
  limen init            .limen.yaml im aktuellen Verzeichnis anlegen
  limen migrate [pfad…] .limen.yaml erzeugen — aus .orca/ übernommen, sonst
                        aus dem ableitbaren Rest. --dry-run zeigt nur.
  limen keychain-import Klartextschlüssel in den Schlüsselbund verschieben
  limen hook zsh|bash   Shell-Hook zum Einbinden

Gesucht wird aufwärts nach .limen.yaml, sonst nach einem .orca/-Verzeichnis.
Ohne Kontext: json gibt {} aus, shell bleibt still, beide mit Exit 0 — damit
der Aufruf bedingungslos aus einer Shell-Startdatei möglich ist.
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
			fmt.Fprintf(stderr, "limen: kein .limen.yaml und kein .orca/ oberhalb von %s\n", cwd)
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

	case "prompt":
		if found {
			fmt.Fprintln(out, Prompt(ctx))
		}

	case "root":
		if found {
			fmt.Fprintln(out, ctx.Root)
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
		fmt.Fprintf(stderr, "limen: unbekannter Befehl %q\n\n%s", cmd, usage)
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
