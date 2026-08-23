# Limen

‹limen› — the threshold. Every `cd` is a crossing; behind it a different
identity applies. Limen says which, and exports it.

A single Go binary with no dependencies. Replaces `orca env` from the retired
Orca monolith.

## Installation

```bash
make install                    # builds and drops it in ~/.local/bin
eval "$(limen hook zsh)"        # add to ~/.zshrc
```

Needs Go to build (`brew install go`), nothing after that — the binary is static
and carries no runtime dependency.

The hook calls Limen exactly **once** per directory change. `LIMEN_SEGMENT`
rides along in the same output, so the status line costs no second process
start.

## Usage

```bash
limen show      # readable overview
limen json      # machine-readable (Atrium, status lines)
limen shell     # export lines for  eval "$(limen shell)"
limen prompt    # one-line segment, never touches the keychain
limen root      # path of the project root
limen list      # every registered context, --json for agents
limen register  # take a context into the registry (the hook does it by itself)
limen note      # append a dated note to .limen/notes.md, --at <label> from anywhere
limen backlog   # open notes across all contexts — where something is to be done
limen profile   # inherited norms: what applies here, is it current (sync, check, install)
limen init      # create .limen/limen.yaml
limen migrate   # lift onto the .limen/ layout (flat .limen.yaml, .orca/), for many projects
limen hook zsh  # print the shell integration
```

## The directory

Everything Limen owns lives in **`.limen/`** at the project root, searched
**upward**:

| File | Content | Git |
|---|---|---|
| `.limen/limen.yaml` | the descriptor (below) | ignored — machine-local |
| `.limen/notes.md` | rolling, dated notes (`limen note`) | commit it |
| `.limen/meta.yaml` | hard context facts, among them `profiles:` | commit it |
| `.limen/profiles.lock` | what was materialised, with a hash per file | commit it |

A flat `.limen.yaml` from earlier versions is still read; `limen migrate` lifts
it into `.limen/` along with `LIMEN.md`/`LIMEN-META.yaml`.

The descriptor is flat YAML, one `key: value` per line, every field optional:

```yaml
label: tessera

# For agents: what happens in this tree. It is how a reader of `limen list`
# routes a loose note to the right directory.
purpose: Product strategy — role design and presentations
topics: design-thinking, customer-journey, roles

actor: Matthias Wegner
githubUser: leo81
claudeConfigDir: ~/.claude-work
gcloudAccount: leo@example.com
gcloudProject: my-project-123
provider: anthropic
model: claude-opus-5

# Points ANTHROPIC_BASE_URL at a local Nuncio. That makes the model route a
# property of the project rather than of the shell it was started from.
gateway: http://localhost:8787

keychainService: limen-anthropic
keychainAccount:                 # falls back to actor
```

The parser deliberately takes only flat YAML and brings no YAML library with it.
A context descriptor needs no nesting, and `key: value` per line is read in 40
lines. `githubUser`, `github-user` and `github_user` are the same field.

## What gets exported

| Variable | Source |
|----------|--------|
| `LIMEN_ROOT` | project root |
| `LIMEN_LABEL` | `label`, otherwise the directory name |
| `LIMEN_ACTOR`, `LIMEN_GH_USER` | `actor`, `githubUser` |
| `LIMEN_PROVIDER`, `LIMEN_MODEL` | `provider`, `model` |
| `LIMEN_SEGMENT` | ready-made status-line segment |
| `CLAUDE_CONFIG_DIR` | `claudeConfigDir`, tilde expanded |
| `CLOUDSDK_CORE_ACCOUNT`, `CLOUDSDK_CORE_PROJECT` | `gcloudAccount`, `gcloudProject` |
| `ANTHROPIC_BASE_URL` | `gateway` |
| `ANTHROPIC_API_KEY` | environment variable, otherwise the keychain |

Empty fields are left out. Moving into a project without `gcloudProject`
therefore exports no empty variable for `gcloud` to be confused by. Values with
quotes are shell-quoted safely; a label like `it's fine` survives `eval`.

## service.yaml is discovered, not duplicated

Some projects carry a `service.yaml` (agnostic-stack) — the descriptor of what
they actually offer. When one sits next to the context, Limen reads **only**
`apiVersion` and `kind` out of it and reports them in `show`, `json` and
`list --json`:

```
service:      Service (agnostic-stack/v1, service.yaml)
```

Deliberately **no** `kind:` field of its own in the descriptor: `service.yaml`
carries `kind:` itself, and a hand-maintained second value could drift from it —
and would. `agnostic-stack-tests` says `TestOrchestrator` there; a copied field
would almost certainly have said "service". Discovering costs no maintenance and
cannot disagree with the file it describes.

It is read by the same flat parser but through its own branch: `service.yaml`
has a nested `metadata:` block whose `name:` would otherwise land in `actor`.
Reading stops after the two fields — they are in the first lines.

## Inherited norms — `limen profile`

Some rules apply not to one project but to all: documentation in English, work
test-first, spend tokens sparingly. Deciding them once and then restating them
in every repository is exactly the second truth Limen otherwise avoids — so the
same rule applies here as for `service.yaml`: **Limen binds and verifies, it
does not store.**

A norm's text lives in an **Agent Plugins package**
([agent-plugins.org](https://agent-plugins.org), spec 1.0.0) — a directory with
`plugin.json`, `skills/` and, here, `adr/` as well. The format was adopted
unchanged rather than adapted: its whole value is that Codex, Cursor, Copilot,
Kiro and VS Code read it too, and a private dialect would trade that away for
nothing. What Limen adds is the two things the spec deliberately leaves to the
client: **which project inherits a package**, and **whether the copy in the
project is still the one that was approved**.

```bash
limen profile install https://github.com/LevaraOrg/levara-baseline
```

The declaration goes in `.limen/meta.yaml`, not in the descriptor. That is not
cosmetic: `limen.yaml` is machine-local and gitignored, `meta.yaml` is
repository content. Which norms a project inherits is a property of the project,
not of the laptop it was checked out on.

```yaml
# .limen/meta.yaml
profiles: levara-baseline@1.0.0, matt-pocock@1.0.0
pausedSkills: write-a-skill      # inherited, deliberately off here
skillTarget: .claude/skills      # default
adrTarget: docs/adr              # default
language: english                # default — working language for code and docs
```

`language:` is the standard language for code and documentation in this tree.
It defaults to `english` even when the file is absent, so `show` and `json`
always answer with one — an agent never has to guess. A project that works in
another language declares it here, where every clone sees the same value.

```bash
limen profile          # what applies here, is it current
limen profile sync     # write skills and ADRs into the project, --dry-run only shows
limen profile check    # exit 1 on drift — for pre-commit or CI
limen profile list     # what the store holds
```

The pairing of ADR and skill is the point: **the ADR is the why, the skill is
the how.** An ADR is read when someone questions a rule; a skill acts without
anyone reading it. Shipping only skills leaves norms nobody can argue with;
shipping only ADRs leaves norms nobody follows.

### Pausing a single skill

A package is inherited whole, but a project does not always want every skill in
it. `pausedSkills:` is the fine-grained half of the same decision: the package
stays declared, one skill stops being materialised.

**What deactivates a skill is absence, not documentation.** A skill missing from
the skill directory cannot be loaded, so no agent has to be told and no text can
contradict the file system. The declaration exists so a human can see the gap is
a decision, and so `sync` can undo it — delete the name and the skill comes back.

ADRs are never paused. A decision record explains why a norm exists and stays
readable where its enforcement is switched off.

Two ways this can lie to you, both of which `check` refuses to stay quiet about:

```
pausedSkills: "write_a_skill" matches no skill in any declared profile — typo? the skill is still active
write-a-skill: paused, but still present in .claude/skills and not written by limen — delete it by hand
```

The first is a typo: you believe a skill is off and it is quietly still there.
The second is a copy that predates the binding — limen removes only what it
wrote, because deleting a file it never owned would be worse than the
inconsistency, so it names the file and leaves the choice to you.

`limen json` carries both lists, so an agent can ask what is available but off
without reading any directory:

```json
"skills": { "active": ["caveman", "tdd"], "paused": ["write-a-skill"] }
```

### Why a lock file

`sync` writes `.limen/profiles.lock` — version, origin and **a SHA-256 per
file**:

```json
{ "version": 1, "profiles": { "levara-baseline": {
  "version": "1.0.0",
  "source": "https://github.com/LevaraOrg/levara-baseline",
  "files": { ".claude/skills/levara-english/SKILL.md": "sha256:6ee2a868…" } } } }
```

Only that makes `check` a statement rather than a guess: a norm someone bent
inside the project shows up without anyone having to read the file again. The
origin comes from the package's own `repository` field, so Limen keeps no second
registry of sources that could contradict the `plugin.json`.

`sync` also **removes**: whatever a profile no longer carries, or has vanished
from `meta.yaml`, is deleted along with the directories that went empty. A
withdrawn norm left lying around is worse than one that was never there — the
agent goes on obeying a rule nobody holds any more.

### None of it runs in the hook

`limen shell` costs 5.6 ms because it touches no network and copies no files. A
profile is materialised by an explicit verb, never by crossing a threshold. That
is the same constraint this tool exists for in the first place.

### `meta.yaml` cannot set identity

It is read by the same flat parser as the descriptor, but through its **own
branch** — like `service.yaml`, and for a sharper reason: `meta.yaml` is
committed. If it could set `actor:` or `githubUser:`, a cloned repository would
decide who the person opening it is. Only `profiles:`, `pausedSkills:`,
`skillTarget:`, `adrTarget:` and `language:` are taken.

## Registry and notes — contexts as anchor points for agents

`Discover` only searches upward; an agent that has to deliver a voice note
"add design thinking to the product strategy" needs the opposite direction:
every context on this machine. That is the registry — one line per root in
`~/.local/state/limen/roots` (or `$XDG_STATE_HOME/limen/roots`). It is fed by
the shell hook: every crossing registers the root once, without anyone
maintaining an index. Trees you never enter by shell are taken in by
`limen register <path…>`. Roots that have vanished quietly drop out at the next
`limen list`.

```bash
limen list --json
# [{"root":"/Users/…/product-strategy","label":"product-strategy",
#   "purpose":"Product strategy — …","topics":["design-thinking","…"],
#   "language":"english",
#   "profiles":[{"name":"levara-baseline","version":"1.0.0"}],
#   "source":"limen"}]
```

The output carries location and meaning, never identity or key state — it is the
routing inventory, not the configuration.

The division of labour is deliberate: **Limen supplies inventory and mechanics,
the matching stays with the agent.** It calls `limen list --json`, matches the
note semantically against `purpose`/`topics`, and delivers it:

```bash
limen note "write down customer needs per journey phase explicitly"
limen note --at product-strategy "…"    # from anywhere, via the registry
```

That lands dated in `.limen/notes.md` — the rolling free-text companion to the
descriptor:

```markdown
## 2026-08-11
- write down customer needs per journey phase explicitly
```

The separation is the point: the descriptor stays hard truth — identity,
interfaces, routing — and is **never** written by tools. Loose thoughts, ideas
and backlog move to `.limen/notes.md`, appended only, never rewritten. And
unlike the descriptor, `notes.md` and `meta.yaml` are project content, not
machine state: they **belong** in the repository.

The opposite direction — *where* is something open? — is answered by
`limen backlog`: it runs over the registry, reads every `notes.md` and lists
everything unchecked, with the path to `cd` into:

```
product-strategy — 1 open
  /Users/…/ProductManagement/ProductStrategy
  2026-08-11  Design thinking: customer needs per journey phase …
```

**Checking off** is the one in-place edit the log allows: `- …` becomes
`- ✓ …`, nothing else changes. `backlog` counts checked lines (`--json` carries
them as `done`) but stops showing them as open. Whoever finishes something —
human or agent — checks the line off and, where useful, appends a dated
follow-up note saying what was worked in where.

## `.limen/limen.yaml` does not belong in the repository

The descriptor carries machine-local identity — `githubUser`,
`claudeConfigDir`, `gcloudAccount` — and can accidentally hold an `apiKey`.
Committed, it distributes the state of *one* machine to everyone. What is
ignored is therefore **only the file**, never the whole `.limen/` directory —
`notes.md` and `meta.yaml` are meant to be committed.

`limen init` and `limen migrate` handle that themselves, in this order:

1. If a `.gitignore` exists, `.limen/limen.yaml` is entered there —
   idempotently, existing content stays; `migrate` bends an old `.limen.yaml`
   entry onto the new path.
2. If none exists, **none is created.** A `.gitignore` is committed content;
   creating one just to hide a file that is *not* committed adds something to
   the repository that does not belong there. Instead `.git/info/exclude` gets
   the entry — same effect, per working copy, nothing to commit.
3. If there is no `.git`, nothing happens: without a repository nothing can be
   committed by accident.

You verify that with git itself, not by looking at the file:

```bash
git check-ignore -v .limen/limen.yaml
```

## Keys

Limen **never** reads the key from the configuration file. Resolution order:
`ANTHROPIC_API_KEY` from the environment, then the macOS keychain
(`keychainService` / `keychainAccount`, the latter falling back to `actor`).
Only `limen shell` prints it, because that is its purpose — `show`, `json` and
`prompt` never show it. With `provider != anthropic` no lookup happens at all.

If an `apiKey:` does sit in the file, that is treated as a warning sign: `show`
warns, `json` sets `api_key_in_config: true`, `prompt` appends
`!key-in-config`. To move it:

```bash
limen keychain-import   # stores it in the keychain
                        # afterwards delete the apiKey: line
```

## WezTerm

The status line in the top right and the tab title are fed by `limen json`.
The module lives in this repository, at `integrations/wezterm-limen.lua`.

Setup is two steps, and the second is the one people forget:

```bash
make install-wezterm     # step 1: symlinks integrations/wezterm-limen.lua into ~/.config/wezterm
```

**Step 2 — wire it into your config yourself.** `make install-wezterm` installs
the module but deliberately does not touch your configuration. Find out which
file WezTerm actually reads, because it supports two and only one of them is
yours:

```bash
ls -l ~/.wezterm.lua ~/.config/wezterm/wezterm.lua 2>/dev/null
```

`~/.wezterm.lua` wins if both exist. Editing the one WezTerm does not read is
the single most likely reason nothing changes.

```lua
local limen = require 'wezterm-limen'

-- Not optional in practice. The module shells out with `bash -c`, which reads
-- no profile, and a GUI-launched WezTerm inherits launchd's PATH — which does
-- not contain ~/.local/bin. Without this line the module finds no binary and
-- draws `no limen` in a directory that has a perfectly good context.
limen.bin = '/Users/you/.local/bin/limen'

limen.apply(config)
```

Existing windows keep the old configuration until `Cmd+Shift+R`
(Reload Configuration); new windows pick it up immediately.

### Coming from the orca module

Replace the `require` and the `apply` call — installing the limen module does
not remove the old one, and an untouched config goes on loading it:

```lua
local orca = require "wezterm-orca"   -- becomes: local limen = require "wezterm-limen"
orca.apply(config)                    -- becomes: limen.apply(config)
```

Then delete `~/.config/wezterm/wezterm-orca.lua`. The symptom of skipping this
is a dimmed `no orca` in the status line — which is the *old* module reporting
correctly, not the new one failing.

What gets drawn:

| Place | Content |
|---|---|
| tab title | coloured `label`, colour from `M.palette` (a prefix suffices) |
| top right | `actor · model · gh:… · gcp:… · gw · !key-in-config` |
| without a context | dimmed `no limen — limen init` |

Empty fields fall away, leaving no stray separators. `gw` appears when the
project runs through a local Nuncio — then the model route belongs to the
project and not to the shell. `!key-in-config` appears **only** for a plaintext
key in the file; a resolvable key from the environment or the keychain is not a
warning.

The colour is looked up exactly first, then by the **longest matching prefix**.
That is necessary, not convenient: for legacy projects `label` is the directory
name, so `circlead-platform` rather than `circlead` — without prefix matching
every project would get the same default violet, which would be worse than the
predecessor that coloured by circle name. `leviathan` deliberately keeps the
default: it shares only "lev" with `levara` and is a different project. Add your
own entries to `M.palette`.

Migration from the predecessor: `circles[1]` → `label`, `actor_name` → `actor`,
`!key-in-repo` → `!key-in-config`. `circles` came from the organisational model,
which went away with Orca; `label` replaces it and falls back to the directory
name.

The module is tested in **WezTerm's own Lua runtime** — no standalone Lua
interpreter is needed for it:

```bash
make test-wezterm
```

```
  ok    module loads
  ok    apply() is callable from a real config
  ok    LIMEN-SELFTEST-OK 31 checks passed
  ok    a broken module is detected (negative control)
  ok    limen json carries every field the module reads
  ok    limen json outside a project is {} as the module assumes
```

The negative control is deliberate: it builds a knowingly broken module and
demands that WezTerm report it. Without it, the checks above could pass by never
executing anything.

## Switching the GitHub account along

Limen does not call `gh auth switch` itself — a tool that runs on every `cd`
should not toggle other people's state. Whoever wants it appends it:

```bash
# in .zshrc, after the hook
_limen_gh() { [[ -n "$LIMEN_GH_USER" ]] && gh auth switch --user "$LIMEN_GH_USER" >/dev/null 2>&1; }
add-zsh-hook chpwd _limen_gh
```

## Backwards compatibility

Without a `.limen.yaml`, Limen reads an existing `.orca/identity.yaml` plus
`.orca/config.yaml` and takes over `name` (as actor), `provider`, `model`,
`githubUser`, `claudeConfigDir`, `gcloudAccount`, `gcloudProject`. Existing
projects therefore keep working without a migration step; `limen show` reports
the origin as `.orca/ (legacy)`. If both files are present, `.limen.yaml` wins.

### Switching over with `limen migrate`

```bash
limen migrate --dry-run ~/Documents/GitHub/*/   # only shows
limen migrate ~/Documents/GitHub/*/             # writes
```

An existing `.limen.yaml` is **never** overwritten. What `migrate` does depends
on what it finds:

| Found | Result |
|---|---|
| `.orca/` | every field taken over verbatim, the display stays identical |
| `.orca/` with `apiKey` | key **not** taken over, a pointer to `limen keychain-import` instead |
| nothing | only `label` (the directory name), the rest empty |

The key is deliberately not copied along: it would then sit in plaintext in
*two* files instead of none.

Nothing is guessed. In particular the owner of the `origin` remote is **not**
entered as `githubUser`, only offered as a comment. Checked against the five
projects whose real value was known from `.orca/`, it was wrong in four: the
remote often belongs to an organisation (`LevaraOrg`), the same person uses
different accounts per project (`levaraleo`, `tgmatthias`), and for a fork it is
someone else entirely (`palamim`). A wrong account name in the status line is
worse than none — telling a work checkout from a private one is this field's
entire job.

That the switch does not change the display is something you verify yourself:

```bash
for d in ~/Documents/GitHub/*/; do (cd "$d" && printf '%-30s %s\n' "$(basename $d)" "$(limen prompt)"); done > /tmp/before.txt
limen migrate ~/Documents/GitHub/*/
# the same loop into /tmp/after.txt, then:
diff /tmp/before.txt /tmp/after.txt
```

Expected: projects with `.orca/` appear unchanged, projects without a context
newly show their label. Measured that way on 2026-08-10 across 26 repositories.

## Tests

```bash
make test          # Go tests plus the WezTerm module
make test-go       # Go only
make test-wezterm  # the Lua module only, in WezTerm's runtime
make test-v        # verbose, shows which behaviour is being checked
make cover         # coverage
make bench         # startup time per call, measured on your machine
```

**124 test cases** — unit tests for the parser, resolution and output, plus
integration tests that run the built binary in real directories. The keychain is
never touched: the lookup function is injectable, and the CLI tests prove with a
`PATH=/nonexistent` that `prompt` gets by without `security(1)`.

What the tests pin down, because it is what hurts inside a `.zshrc`:

- `limen shell` stays **silent** without a context and exits **0** — otherwise
  the unconditional call from a startup file would not be possible
- the output of `shell` survives a real `eval` in `/bin/sh`, including a value
  with an apostrophe
- the plaintext key does **not** appear in `show`, `json` or `prompt`
- the hook text is syntactically valid bash resp. zsh (`bash -n`, `zsh -n`)
- a second `limen init` does **not** overwrite the identity file
- `meta.yaml` cannot set identity, however a repository writes it

## Layout

```
main.go        CLI dispatch, exit codes
context.go     upward search, flat YAML parser, Context
meta.go        .limen/meta.yaml — profiles and targets, its own key switch
profile.go     store, Agent Plugins package, sync/check, the lock
keychain.go    resolution order, security(1), injectable for tests
render.go      show / json / shell / prompt
commands.go    init, keychain-import, shell hooks
integrations/
  wezterm-limen.lua  WezTerm status line and tab title
  selftest.lua       checks that WezTerm's Lua runtime executes
  test.sh            test driver including the negative control
scripts/bench.sh     startup-time benchmark
```
