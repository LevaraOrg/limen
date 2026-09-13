# Sketch: `limen status` and the start routine

Status: **implemented** in v0.15.0 (`start.go`, `status.go`), 2026-09-13. The
sketch below is kept as the reasoning; the README section "Does the declaration
still hold?" is the user-facing description. Decisions taken on the way:

- Rule 1 is real: `spec.start` is read from `service.yaml` when present. Whether
  the key belongs there (open question 1) stays an agnostic-stack decision;
  limen only reads, it never asks for it.
- "Ambiguous" means more than one discovery hit and nothing declared. Measured
  again with that definition, 12 of the 19 contexts with a routine show `(+n)`,
  most of them `scripts/start.sh` beside a `package.json` — the rule order
  resolves those, and `--verbose` shows what it resolved. The two cases the
  sketch names (three `start-*.sh` in `circlead-platform`, three rules in
  `Tessera`) remain the ones where a declaration actually earns its place.
- `--check` also fails on a declared start routine that is gone from disk and on
  an endpoint that cannot be read: both are declarations limen knows to be
  wrong, the same reason `ports` exits 1 on a collision.
- `--deep` applies the HTTP check to the primary endpoint only; named endpoints
  stay a dial. `healthcheck.port` is not read (open question 2, left open).
- Contexts with neither endpoint nor routine have no row and are counted in the
  summary line; `--verbose` lists them as `✗ none`. The sketch's example table
  showed such a row; the sentence above it wins.
- Open questions 3 (the reverse direction) and 4 (`status` versus `ports`) are
  untouched: `ports` keeps `--caddy` and `--write`, `status` shares the
  endpoint parsing with it and nothing else.

## The gap

Limen knows what a context *declares*. It does not know whether that declaration
still holds, and it has nothing to say about how the thing is started.

Measured on this machine, **38 registered contexts**:

- `limen ports` lists **8 endpoints**. **35 TCP ports** are actually listening.
  Two of the undeclared ones are project servers, not system noise: nuncio on
  8787 and a second agile-stack process on 5200. The registry silently disagrees
  with reality and nothing says so.
- Of the 8 declared endpoints, **5 were listening** at the time of writing
  (4317, 5199, 8000, 8080, 8081) and 3 were not (4318, 5173, 8083). From the
  outside, "declared but down" and "declared and up" look identical.
- The start routine differs per project with no pattern: `scripts/start*.sh`
  (11), `docker-compose.yml` (4), `pom.xml` (4), `npm dev` (3), `npm start` (3),
  `go.mod` (2), `pubspec.yaml` (1). **20 of the 38 contexts have no discoverable
  way to start at all** — most of them rightly so, being document repositories
  or skill libraries rather than services.
- `service.yaml` (6 contexts) declares `spec.runtime.build` and a
  `spec.healthcheck`, but **no run command**. The one file that could answer the
  question does not.

## Scope — the line this sketch draws

Limen answers questions about **its own declarations, including whether they
hold**. That is the same move `limen ports` already makes when it exits 1 on a
port collision: the declaration is limen's, so its consistency is limen's too.

Everything about *processes and sessions* stays out. Which terminals are open,
which tmux session belongs to whom, starting and stopping — that is clavo's
subject, and clavo already owns tmux, a PTY and the phone app.

|                                   | limen | clavo |
|-----------------------------------|:-----:|:-----:|
| context, endpoints, start routine | ●     |       |
| is the declared port listening    | ●     |       |
| open terminals / tmux sessions    |       | ●     |
| running a start routine           |       | ●     |
| the board that joins both         |       | ●     |

So clavo consumes `limen status --json` and adds its session layer. Limen never
calls clavo — the direction is one-way, as it already is between circlead and
`start-stack.sh`.

**Limen prints the start command. Limen never runs it.** A tool whose job is to
describe a directory must not acquire the power to execute what it finds there.

## The start routine: discover, declare only to disambiguate

`service.go` already states the principle: *discovering beats duplicating.* A
hand-maintained `start:` in every descriptor would be a second truth that drifts
from `package.json` the first time a script is renamed.

So the routine is **derived**, in this order, first hit wins:

| # | Evidence                                        | Yields                      |
|---|-------------------------------------------------|-----------------------------|
| 1 | `service.yaml` → `spec.start` *(does not exist yet — see open questions)* | as written |
| 2 | `scripts/start.sh`                              | `scripts/start.sh`          |
| 3 | `scripts/start-<label>.sh`                      | that script                 |
| 4 | `package.json` → `scripts.dev`, else `start`, else `serve` | `npm run <name>` |
| 5 | `Makefile` target `run`, else `dev`, else `start` | `make <target>`            |
| 6 | `docker-compose.yml` / `compose.yaml`           | `docker compose up`         |
| 7 | `go.mod` / `pom.xml` / `pubspec.yaml`           | `go run .` / `mvn spring-boot:run` / `flutter run` |

Discovery reports **every** hit, not only the winner, because the count is the
interesting part: one hit is an answer, four hits are an ambiguity worth naming.
Exactly two contexts are ambiguous today — `circlead-platform` and `Tessera`,
both matching rules 2, 6 and 7 at once. For `circlead-platform` rule 2 then
finds three `start*.sh` scripts, and discovery cannot and should not guess
between `start-circlead.sh`, `start-stack.sh` and `start-nuncio.sh`. Two cases
out of 38 is the measure of how small the declaration surface needs to be.

That is the only case where a declaration earns its place:

```yaml
# .limen/meta.yaml   — committed, the same for every clone
start: scripts/start-circlead.sh dev

# .limen/limen.yaml  — machine-local, wins over the committed one
start: scripts/start-circlead.sh dev --skip-tests --with-sidecars
```

Same two-layer precedence as `devEndpoints`, for the same reason: what every
clone agrees on belongs in `meta.yaml`, what this machine happens to need is
local. A declared `start:` silences the ambiguity; it does not switch discovery
off, and `status` still reports when the declared command no longer exists on
disk — a rename must not fail silently.

## `limen status`

One row per registered context, skipping contexts that declare nothing and
expose nothing.

```
CONTEXT              ENDPOINT              START                        HEALTH
circlead-platform    8080  circlead        scripts/start-circlead.sh    up
Tessera              5173  tessera         scripts/start-tessera.sh (+2) down
Tessera/api          8081  tessera-api     ·                            up
cxo-dashboard        4317  dashboard       npm run dev                  up
orca                 8083  orca            scripts/start.sh             down
agile-stack          5199  agile-stack     scripts/start.sh             up
circlead-widgets     4318  circlead-widg.  npm run start                down
text-anonymizer      8000  anonymizer      docker compose up            up
tessera-pmsc         ·     ·               ✗ none                       ·

8 endpoints · 5 up · 3 down · 20 contexts without a start routine
```

- `up` / `down` is a TCP dial against `127.0.0.1:<port>` with a short timeout.
  When `service.yaml` carries a `healthcheck.path`, `--deep` upgrades that to an
  HTTP GET — but the default stays a dial, because a dial costs nothing and
  needs no network policy.
- `✗ none` is the finding, not an error. 20 contexts are in that state today and
  most of them should be: `tessera-pmsc` is a document repository, and so are
  `ppwr-tool`, `produktstrategie` and the `turbogruen-skill*` libraries.
- **Ambiguity is shown, not hidden.** Where discovery finds several candidates
  and nothing is declared, the cell reads `scripts/start-circlead.sh  (+3)` and
  `--verbose` lists them.

Flags, mirroring `ports`:

| Flag        | Effect                                                            |
|-------------|-------------------------------------------------------------------|
| `--json`    | machine-readable, the shape clavo consumes                        |
| `--deep`    | HTTP healthcheck where `service.yaml` declares a path             |
| `--verbose` | every discovery hit per context, not only the winner              |
| `--check`   | assertive variant: exit 1 when something declared is not listening |

Exit code is **0 by default, even with everything down** — a status that fails
because a dev server is off is a status nobody runs. `--check` is the opposite
contract, for a pre-commit hook or a watching proxy, and follows the precedent
of `ports --write` refusing to emit a file limen already knows to be wrong.

## Non-goals

- No starting, stopping or restarting. See the scope table.
- No process, tmux or `lsof` inspection. A TCP dial against a port limen itself
  declared is the whole extent of the runtime it looks at.
- No new truth. Every discovered routine is re-derivable from the files in the
  directory; the only thing limen stores is a disambiguation, and only when the
  directory is genuinely ambiguous.

## Open questions

1. **Does `spec.start` belong in `service.yaml` instead?** That file is the
   service descriptor and already carries `runtime.build` and `healthcheck` —
   the run command is the obvious missing sibling, and signum could then verify
   it against reality like it verifies the rest. It would make rule 1 real and
   `meta.yaml: start:` unnecessary for the 6 contexts that have a service file.
   It is an agnostic-stack decision, not a limen one.
2. **Is `healthcheck.port` a drift signal?** `circlead-platform` declares 8080 in
   both `service.yaml` and its limen endpoint. When those two disagree, one of
   them is wrong, and `status` is where it would show. Cheap to add, but it
   makes limen read further into a file it deliberately only skims today.
3. **Should `--check` cover the reverse direction** — a port listening that no
   context declares? It would have caught nuncio on 8787 and agile-stack on
   5200. It also means limen looking at ports that are none of its business.
4. **Does `status` supersede `ports`,** or stay a second command? `ports` has a
   job `status` will not take over (`--caddy`, `--write`), so probably both —
   but then the shared table wants one implementation, not two.

## Implementation order

Test-first per ADR-0002, each step shippable on its own:

1. `discoverStart(root) []Candidate` — pure function over a directory, no I/O
   beyond reading. Table-driven test with a temp dir per rule.
2. `start:` in `meta.yaml` and `limen.yaml`, precedence identical to
   `devEndpoints`; reuse that resolution rather than writing a second one.
3. `Context.Start()` — declaration over discovery, plus the "declared but
   missing on disk" case.
4. Expose in `limen show` / `json` / `list --json`. At this point clavo can
   already build the board; everything after is rendering.
5. `CmdStatus` with the dial, then `--json`, `--verbose`, `--check`, `--deep`.
