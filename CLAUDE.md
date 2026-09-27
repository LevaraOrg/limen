# limen

Context and identity per directory — a Go binary with a descriptor
(`.limen/limen.yaml`), a registry and rolling notes as an anchor point for
agents.

## Home and identity (LevaraOrg)

- GitHub home: https://github.com/LevaraOrg/limen — organisation **LevaraOrg**
  (moved from `matthiaw` on 2026-08-18; old URLs redirect).
- The working identity of this directory comes from `.limen/limen.yaml` (tool:
  limen): githubUser `levaraleo`, actor Matthias, Claude config
  `~/.claude-levara`.

## Inherited norms

This directory inherits `levara-baseline@2.0.0` (see `.limen/meta.yaml`). The
skills are materialised in `.claude/skills/`, the reasoning as ADRs in
`docs/adr/`. In short:

- **ADR-0001** — everything committed is English: code, comments, documentation,
  commit messages, test names, CLI output and error strings. Conversation with
  the user stays in whatever language they write. The same default is
  machine-readable through limen itself: `limen show`, `limen json` and
  `limen list --json` report a `language` field, `english` unless a project
  declares otherwise via `language:` in `.limen/meta.yaml` — ask limen rather
  than guessing the working language.
- **ADR-0002** — test-driven: the failing test first, then the least code that
  makes it pass. Bug fixes are never exempt.
- **ADR-0003** — token-frugal: the cheapest tool that answers the question,
  never at the cost of completeness.
- **ADR-0004** — a 75% line-coverage floor per module (a ratchet where not yet
  met) and a bug-pattern analyzer over the sources.
- **ADR-0005** — dependency, secret and security static-analysis scans gate
  every build; a red-team pass before every release that changes a trust
  boundary.
- **ADR-0006** — large projects (services, several deployable modules) build,
  start and smoke-test as containers in CI. limen is a single binary and is not
  large, so this one does not bind here.

`limen profile check` proves the copies are unchanged — exit 1 on drift. Do not
edit the files under `.claude/skills/` or `docs/adr/` by hand: `limen profile
sync` overwrites them. A norm is changed in the package
(`~/Documents/GitHub/levara-baseline`) and its version raised.

Note that limen's own user-facing output is English too, deliberately. ADR-0001
covers error strings, and a tool with an English README and German error
messages would be the mixed state the norm exists to prevent.
