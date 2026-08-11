# Limen

‹limen› — die Schwelle. Jedes `cd` ist ein Schwellengang; dahinter gilt eine
andere Identität. Limen sagt welche, und exportiert sie.

Ein einzelnes Go-Binary ohne Abhängigkeiten. Ersetzt `orca env` aus dem
stillgelegten Orca-Monolithen.

## Warum nicht das alte Werkzeug

Der Nutzen war nie das Problem, die Startzeit war es. `orca env json` brauchte
auf dieser Maschine **530 ms**, weil jeder Aufruf eine JVM hochfährt — deshalb
cachte die alte WezTerm-Integration ihr Ergebnis 15 Sekunden lang und lag bei
jedem Verzeichniswechsel eine Weile daneben. Ein Kontextwerkzeug, das bei jedem
`cd` läuft, darf das nicht kosten.

Gemessen mit `make bench` (200 Läufe je Zeile, dieselbe Maschine):

| | pro Aufruf |
|---|---|
| `orca env json` (JVM) | **530 ms**, deshalb 15 s Cache |
| `limen prompt` | **5,9 ms** |
| `limen shell`, Schlüssel schon in der Umgebung | **5,6 ms** |
| `limen shell`, Schlüsselbund-Zugriff | 25,0 ms |
| `/usr/bin/true` als Vergleich | 3,9 ms |

Die untere Schranke auf dieser Maschine ist der Prozessstart selbst: 3,9 ms.
Limens Eigenanteil liegt also bei **rund 1,5 ms**. Die einzige teure Zeile ist
der Schlüsselbund, und das ist ein `security(1)`-Fork — macOS-Kosten, keine
Limen-Kosten. Wer den Schlüssel einmal in die Umgebung exportiert, zahlt sie
nicht.

## Installation

```bash
make install                    # baut und legt nach ~/.local/bin
eval "$(limen hook zsh)"        # in ~/.zshrc aufnehmen
```

Braucht Go zum Bauen (`brew install go`), danach nichts mehr — das Binary ist
statisch und trägt keine Laufzeitabhängigkeit.

Der Hook ruft Limen genau **einmal** je Verzeichniswechsel. `LIMEN_SEGMENT`
kommt aus derselben Ausgabe mit, damit die Statuszeile keinen zweiten
Prozessstart kostet.

## Benutzung

```bash
limen show      # lesbare Übersicht
limen json      # maschinenlesbar (Atrium, Statuszeilen)
limen shell     # export-Zeilen für  eval "$(limen shell)"
limen prompt    # einzeiliges Segment, berührt den Schlüsselbund nicht
limen root      # Pfad der Projektwurzel
limen list      # alle registrierten Kontexte, --json für Agenten
limen register  # Kontext ins Register aufnehmen (der Hook tut das von selbst)
limen note      # datierte Notiz an .limen/notes.md anhängen, --at <label> von überall
limen backlog   # offene Notizen aller Kontexte — wo etwas zu tun ist
limen init      # .limen/limen.yaml anlegen
limen migrate   # aufs .limen/-Layout heben (flache .limen.yaml, .orca/), für viele Projekte
limen hook zsh  # Shell-Integration ausgeben
```

## Das Verzeichnis

Alles Limen-Eigene liegt in **`.limen/`** in der Projektwurzel, **aufwärts**
gesucht:

| Datei | Inhalt | Git |
|---|---|---|
| `.limen/limen.yaml` | der Deskriptor (unten) | ignoriert — maschinenlokal |
| `.limen/notes.md` | rollierende, datierte Notizen (`limen note`) | committen |
| `.limen/meta.yaml` | harte Kontextfakten (Bounded-Context-Schema) | committen |

Eine flache `.limen.yaml` aus früheren Versionen wird weiter gelesen;
`limen migrate` hebt sie samt `LIMEN.md`/`LIMEN-META.yaml` nach `.limen/`.

Der Deskriptor ist flaches YAML, ein `key: value` je Zeile, alle Felder
optional:

```yaml
label: tessera

# Für Agenten: was in diesem Baum passiert. Darüber ordnet ein Leser von
# `limen list` eine lose Notiz dem richtigen Verzeichnis zu.
purpose: Produktstrategie — Rollenerarbeitung und Präsentationen
topics: design-thinking, customer-journey, rollen

actor: Matthias Wegner
githubUser: leo81
claudeConfigDir: ~/.claude-work
gcloudAccount: leo@example.com
gcloudProject: my-project-123
provider: anthropic
model: claude-opus-5

# Zeigt ANTHROPIC_BASE_URL auf ein lokales Nuncio. Damit gehört die
# Modellroute zum Projekt und nicht zur Shell, aus der man es gestartet hat.
gateway: http://localhost:8787

keychainService: limen-anthropic
keychainAccount:                 # fällt auf actor zurück
```

Der Parser nimmt bewusst nur flaches YAML und bringt keine YAML-Bibliothek mit.
Ein Kontextdescriptor braucht keine Verschachtelung, und `key: value` je Zeile
ist in 40 Zeilen gelesen. `githubUser`, `github-user` und `github_user` sind
dasselbe Feld.

## Was exportiert wird

| Variable | Quelle |
|----------|--------|
| `LIMEN_ROOT` | Projektwurzel |
| `LIMEN_LABEL` | `label`, sonst der Verzeichnisname |
| `LIMEN_ACTOR`, `LIMEN_GH_USER` | `actor`, `githubUser` |
| `LIMEN_PROVIDER`, `LIMEN_MODEL` | `provider`, `model` |
| `LIMEN_SEGMENT` | fertiges Statuszeilen-Segment |
| `CLAUDE_CONFIG_DIR` | `claudeConfigDir`, Tilde aufgelöst |
| `CLOUDSDK_CORE_ACCOUNT`, `CLOUDSDK_CORE_PROJECT` | `gcloudAccount`, `gcloudProject` |
| `ANTHROPIC_BASE_URL` | `gateway` |
| `ANTHROPIC_API_KEY` | Umgebungsvariable, sonst Schlüsselbund |

Leere Felder werden ausgelassen. Ein Wechsel in ein Projekt ohne
`gcloudProject` exportiert also keine leere Variable, die `gcloud` dann
verwirrt. Werte mit Anführungszeichen werden shell-sicher quotiert; ein Label
wie `it's fine` übersteht `eval`.

## Register und Notizen — Kontexte als Ankerpunkte für Agenten

`Discover` sucht nur aufwärts; ein Agent, der eine Sprachnotiz „ergänze Design
Thinking in der Produktstrategie" zustellen soll, braucht die Gegenrichtung:
alle Kontexte dieser Maschine. Das ist das Register — eine Zeile je Wurzel in
`~/.local/state/limen/roots` (bzw. `$XDG_STATE_HOME/limen/roots`). Gefüttert
wird es vom Shell-Hook: jeder Schwellengang registriert die Wurzel einmalig,
ohne dass jemand einen Index pflegt. Bäume, die man nie per Shell betritt,
nimmt `limen register <pfad…>` auf. Verschwundene Wurzeln fallen beim nächsten
`limen list` still heraus.

```bash
limen list --json
# [{"root":"/Users/…/produktstrategie","label":"produktstrategie",
#   "purpose":"Produktstrategie — …","topics":["design-thinking","…"],
#   "source":"limen"}]
```

Die Ausgabe trägt Ort und Bedeutung, nie Identität oder Schlüsselzustand — sie
ist das Routing-Inventar, nicht die Konfiguration.

Die Arbeitsteilung ist bewusst: **Limen liefert Inventar und Mechanik, das
Zuordnen bleibt beim Agenten.** Der ruft `limen list --json` auf, matcht die
Notiz semantisch gegen `purpose`/`topics`, und stellt sie zu:

```bash
limen note "Kundenbedürfnisse pro Journey-Phase explizit aufschreiben"
limen note --at produktstrategie "…"    # von überall, über das Register
```

Das landet datiert in `.limen/notes.md` — dem rollierenden Freitext-Begleiter
des Deskriptors:

```markdown
## 2026-08-11
- Kundenbedürfnisse pro Journey-Phase explizit aufschreiben
```

Die Trennung ist der Punkt: der Deskriptor bleibt harte Wahrheit — Identität,
Schnittstellen, Routing — und wird von Werkzeugen **nie** beschrieben. Lose
Gedanken, Ideen und Backlog wandern nach `.limen/notes.md`, nur angehängt, nie
umgeschrieben. Und anders als der Deskriptor sind `notes.md` und `meta.yaml`
Projektinhalt, kein Maschinenzustand: sie **gehören** ins Repository.

Die Gegenrichtung — *wo* ist etwas offen? — beantwortet `limen backlog`: es
läuft übers Register, liest jede `notes.md` und listet alles Unabgehakte,
mit Pfad zum Reinwechseln:

```
produktstrategie — 1 offen
  /Users/…/Produktmanagement/Produktstrategie
  2026-08-11  Design Thinking: Kundenbedürfnisse pro Journey-Phase …
```

**Abhaken** ist das eine erlaubte In-Place-Edit im Log: aus `- …` wird
`- ✓ …`, sonst ändert sich nichts. `backlog` zählt abgehakte Zeilen mit
(`--json` trägt sie als `done`), zeigt sie aber nicht mehr als offen. Wer
etwas erledigt — Mensch oder Agent — hakt die Zeile ab und hängt bei Bedarf
eine datierte Folge-Notiz an, was wo eingearbeitet wurde.

## `.limen/limen.yaml` gehört nicht ins Repository

Der Deskriptor trägt maschinenlokale Identität — `githubUser`,
`claudeConfigDir`, `gcloudAccount` — und kann versehentlich einen `apiKey`
enthalten. Eingecheckt verteilt er den Zustand *einer* Maschine an alle.
Ignoriert wird deshalb **nur die Datei**, nie das ganze `.limen/`-Verzeichnis —
`notes.md` und `meta.yaml` sollen ja committet werden.

`limen init` und `limen migrate` erledigen das selbst, in dieser Reihenfolge:

1. Existiert eine `.gitignore`, wird `.limen/limen.yaml` dort eingetragen —
   idempotent, vorhandener Inhalt bleibt; `migrate` biegt einen alten
   `.limen.yaml`-Eintrag auf den neuen Pfad um.
2. Existiert keine, wird **keine angelegt.** Eine `.gitignore` ist eingecheckter
   Inhalt; eine anzulegen, nur um eine *nicht* eingecheckte Datei zu verstecken,
   fügt dem Repository etwas hinzu, das nicht dazugehört. Stattdessen bekommt
   `.git/info/exclude` den Eintrag — dieselbe Wirkung, pro Arbeitskopie, nichts
   zu committen.
3. Gibt es kein `.git`, passiert nichts: ohne Repository kann nichts
   versehentlich eingecheckt werden.

Nachprüfbar ist das mit git selbst, nicht mit einem Blick in die Datei:

```bash
git check-ignore -v .limen/limen.yaml
```

## Schlüssel

Limen liest den Schlüssel **nie** aus der Konfigurationsdatei. Auflösung:
`ANTHROPIC_API_KEY` aus der Umgebung, dann der macOS-Schlüsselbund
(`keychainService` / `keychainAccount`, letzteres fällt auf `actor` zurück).
Nur `limen shell` gibt ihn aus, weil das sein Zweck ist — `show`, `json` und
`prompt` zeigen ihn nie. Bei `provider != anthropic` wird gar nicht gesucht.

Steht doch ein `apiKey:` in der Datei, wird das als Warnzeichen behandelt:
`show` warnt, `json` setzt `api_key_in_config: true`, `prompt` hängt
`!key-in-config` an. Verschieben:

```bash
limen keychain-import   # legt ihn im Schlüsselbund ab
                        # danach die Zeile apiKey: löschen
```

## WezTerm

Die Statuszeile oben rechts, die vorher `orca env json` fütterte, wird jetzt von
`limen json` gefüttert. Gleiche Stelle, gleiche Information, ohne den 15-Sekunden-Cache.

```bash
make install-wezterm     # symlinkt integrations/wezterm-limen.lua nach ~/.config/wezterm
```

Dann in `~/.config/wezterm/wezterm.lua`:

```lua
local limen = require 'wezterm-limen'
-- WezTerm erbt deinen Shell-PATH nicht. Falls limen dort nicht liegt:
-- limen.bin = '/Users/du/.local/bin/limen'
limen.apply(config)
```

Was gezeichnet wird:

| Stelle | Inhalt |
|---|---|
| Tab-Titel | farbiges `label`, Farbe aus `M.palette` (Präfix genügt) |
| oben rechts | `actor · model · gh:… · gcp:… · gw · !key-in-config` |
| ohne Kontext | gedimmtes `no limen — limen init` |

Leere Felder entfallen, es bleiben keine Trennzeichen stehen. `gw` erscheint,
wenn das Projekt über ein lokales Nuncio läuft — dann gehört die Modellroute zum
Projekt und nicht zur Shell. `!key-in-config` erscheint **nur** bei einem
Klartextschlüssel in der Datei; ein auflösbarer Schlüssel aus Umgebung oder
Schlüsselbund ist keine Warnung.

Die Farbe wird zuerst exakt, dann über das **längste passende Präfix** gesucht.
Das ist nötig, nicht bequem: `label` ist bei Legacy-Projekten der
Verzeichnisname, also `circlead-platform` statt `circlead` — ohne Präfixsuche
bekämen alle Projekte dasselbe Standard-Violett, was schlechter wäre als der
Vorgänger, der nach Circle-Namen färbte. `leviathan` bleibt absichtlich beim
Standard: es teilt nur „lev" mit `levara` und ist ein anderes Projekt. Eigene
Einträge in `M.palette` ergänzen.

Migration vom Vorgänger: `circles[1]` → `label`, `actor_name` → `actor`,
`!key-in-repo` → `!key-in-config`. `circles` kam aus dem Organisationsmodell,
das mit Orca weggefallen ist; `label` ersetzt es und fällt auf den
Verzeichnisnamen zurück.

Getestet wird das Modul in **WezTerms eigener Lua-Runtime** — einen
eigenständigen Lua-Interpreter braucht es dafür nicht:

```bash
make test-wezterm
```

```
  ok    module loads
  ok    apply() is callable from a real config
  ok    LIMEN-SELFTEST-OK 25 checks passed
  ok    a broken module is detected (negative control)
  ok    limen json carries every field the module reads
  ok    limen json outside a project is {} as the module assumes
```

Die Negativkontrolle ist Absicht: sie baut ein absichtlich kaputtes Modul und
verlangt, dass WezTerm das meldet. Ohne sie könnten die Prüfungen darüber
bestehen, indem nie etwas ausgeführt wird.

## GitHub-Account mitschalten

Limen ruft `gh auth switch` nicht selbst — ein Werkzeug, das bei jedem `cd`
läuft, soll keine fremden Zustände umschalten. Wer es will, hängt es an:

```bash
# in .zshrc, nach dem Hook
_limen_gh() { [[ -n "$LIMEN_GH_USER" ]] && gh auth switch --user "$LIMEN_GH_USER" >/dev/null 2>&1; }
add-zsh-hook chpwd _limen_gh
```

## Rückwärtskompatibilität

Ohne `.limen.yaml` liest Limen ein vorhandenes `.orca/identity.yaml` plus
`.orca/config.yaml` und übernimmt `name` (als actor), `provider`, `model`,
`githubUser`, `claudeConfigDir`, `gcloudAccount`, `gcloudProject`. Bestehende
Projekte funktionieren also ohne Migrationsschritt weiter; `limen show` weist
die Herkunft als `.orca/ (legacy)` aus. Liegen beide Dateien vor, gewinnt
`.limen.yaml`.

### Umstellen mit `limen migrate`

```bash
limen migrate --dry-run ~/Documents/GitHub/*/   # zeigt nur
limen migrate ~/Documents/GitHub/*/             # schreibt
```

Ein vorhandenes `.limen.yaml` wird **nie** überschrieben. Was `migrate` tut,
hängt daran, was es findet:

| Vorgefunden | Ergebnis |
|---|---|
| `.orca/` | alle Felder wörtlich übernommen, die Anzeige bleibt identisch |
| `.orca/` mit `apiKey` | Schlüssel **nicht** übernommen, dafür ein Hinweis auf `limen keychain-import` |
| nichts | nur `label` (der Verzeichnisname), Rest leer |

Der Schlüssel wird bewusst nicht mitkopiert: er stünde dann im Klartext in
*zwei* Dateien statt in keiner.

Geraten wird nichts. Insbesondere wird der Besitzer des `origin`-Remotes **nicht**
als `githubUser` eingetragen, sondern nur als Kommentar angeboten. Gegen die fünf
Projekte geprüft, deren echter Wert aus `.orca/` bekannt war, lag er in vier
daneben: der Remote gehört oft einer Organisation (`LevaraOrg`), dieselbe Person
nutzt je Projekt verschiedene Konten (`levaraleo`, `tgmatthias`), und bei einem
Fork ist es jemand ganz anderes (`palamim`). Ein falscher Kontoname in der
Statuszeile ist schlimmer als keiner — Arbeits- von Privat-Checkout zu
unterscheiden ist die ganze Aufgabe dieses Feldes.

Dass die Umstellung die Anzeige nicht verändert, prüfst du selbst:

```bash
for d in ~/Documents/GitHub/*/; do (cd "$d" && printf '%-30s %s\n' "$(basename $d)" "$(limen prompt)"); done > /tmp/vorher.txt
limen migrate ~/Documents/GitHub/*/
# dieselbe Schleife nach /tmp/nachher.txt, dann:
diff /tmp/vorher.txt /tmp/nachher.txt
```

Erwartet: Projekte mit `.orca/` erscheinen unverändert, Projekte ohne Kontext
zeigen neu ihr Label. So gemessen am 10.08.2026 über 26 Repositories.

## Tests

```bash
make test          # Go-Tests plus das WezTerm-Modul
make test-go       # nur Go
make test-wezterm  # nur das Lua-Modul, in WezTerms Runtime
make test-v        # verbose, zeigt welches Verhalten geprüft wird
make cover         # Abdeckung
make bench         # die Messung aus der Tabelle oben, auf deiner Maschine
```

**39 Tests** — Unit-Tests für Parser, Auflösung und Ausgabe, plus
Integrationstests, die das gebaute Binary in echten Verzeichnissen ausführen.
Der Schlüsselbund wird nie berührt: die Lookup-Funktion ist injizierbar, und die
CLI-Tests belegen mit einem `PATH=/nonexistent`, dass `prompt` ohne
`security(1)` auskommt.

Was die Tests festnageln, weil es das ist, was in einer `.zshrc` weh tut:

- `limen shell` bleibt ohne Kontext **still** und endet mit **0** — sonst wäre
  der bedingungslose Aufruf aus einer Startdatei nicht möglich
- die Ausgabe von `shell` übersteht ein echtes `eval` in `/bin/sh`, inklusive
  eines Wertes mit Apostroph
- der Klartextschlüssel taucht in `show`, `json` und `prompt` **nicht** auf
- der Hook-Text ist syntaktisch gültiges bash bzw. zsh (`bash -n`, `zsh -n`)
- ein zweites `limen init` überschreibt die Identitätsdatei **nicht**

## Aufbau

```
main.go        CLI-Verteilung, Exit-Codes
context.go     Aufwärtssuche, flacher YAML-Parser, Context
keychain.go    Auflösungsreihenfolge, security(1), injizierbar für Tests
render.go      show / json / shell / prompt
commands.go    init, keychain-import, Shell-Hooks
integrations/
  wezterm-limen.lua  WezTerm-Statuszeile und Tab-Titel
  selftest.lua       Prüfungen, die WezTerms Lua-Runtime ausführt
  test.sh            Testtreiber inklusive Negativkontrolle
scripts/bench.sh     die Messung
```
