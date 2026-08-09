# Limen

‹limen› — die Schwelle. Jedes `cd` ist ein Schwellengang; dahinter gilt eine
andere Identität. Limen sagt welche, und exportiert sie.

Ein Shell-Skript, keine Installation, keine Laufzeitumgebung. Ersetzt
`orca env` aus dem stillgelegten Orca-Monolithen.

## Warum nicht das alte Werkzeug

Der Nutzen war nie das Problem, die Startzeit war es. `orca env json` brauchte
auf dieser Maschine **530 ms**, weil jeder Aufruf eine JVM hochfährt — deshalb
cachte die alte WezTerm-Integration ihr Ergebnis 15 Sekunden lang und lag bei
jedem Verzeichniswechsel eine Weile daneben. Ein Kontextwerkzeug, das bei
jedem `cd` läuft, darf das nicht kosten.

| | pro Verzeichniswechsel |
|---|---|
| `orca env json` (JVM) | 530 ms, deshalb 15 s Cache |
| `limen shell` | **39 ms**, kein Cache nötig |

Die 39 ms sind fast vollständig ein `security(1)`-Aufruf für den
Schlüsselbund (~22 ms). Ohne Schlüssel im Spiel — etwa `limen prompt` für die
Statuszeile — sind es **15 ms**.

## Installation

```bash
./install.sh                    # symlinkt bin/limen nach ~/.local/bin
eval "$(limen hook zsh)"        # in ~/.zshrc aufnehmen
```

Der Hook ruft Limen genau einmal je Verzeichniswechsel. `LIMEN_SEGMENT` kommt
aus derselben Ausgabe mit, damit die Statuszeile keinen zweiten Prozessstart
kostet.

## Benutzung

```bash
limen show      # lesbare Übersicht
limen json      # maschinenlesbar (Atrium, Statuszeilen)
limen shell     # export-Zeilen für  eval "$(limen shell)"
limen prompt    # einzeiliges Segment, berührt den Schlüsselbund nicht
limen root      # Pfad der Projektwurzel
limen init      # .limen.yaml anlegen
```

## Die Datei

`.limen.yaml` in der Projektwurzel, aufwärts gesucht. Flaches YAML, ein
`key: value` je Zeile, alle Felder optional:

```yaml
label: tessera
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

Der Parser nimmt bewusst nur flaches YAML. Ein Kontextdescriptor braucht keine
Verschachtelung, und ein Parser für mehr wäre der Grund, eine Abhängigkeit zu
ziehen — womit die Startzeit wieder da wäre, die wir gerade losgeworden sind.

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

Leere Felder werden ausgelassen. Ein Wechsel in ein Projekt ohne `gcloudProject`
exportiert also keine leere Variable, die `gcloud` dann verwirrt.

## Schlüssel

Limen liest den Schlüssel **nie** aus der Konfigurationsdatei. Auflösung:
`ANTHROPIC_API_KEY` aus der Umgebung, dann der macOS-Schlüsselbund
(`keychainService` / `keychainAccount`). Nur `limen shell` gibt ihn aus, weil
das sein Zweck ist — `show`, `json` und `prompt` zeigen ihn nie.

Steht doch ein `apiKey:` in der Datei, wird das als Warnzeichen behandelt:
`show` warnt, `json` setzt `api_key_in_config: true`, `prompt` hängt
`!key-in-config` an. Verschieben:

```bash
limen keychain-import   # legt ihn im Schlüsselbund ab
                        # danach die Zeile apiKey: löschen
```

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
`.orca/config.yaml` und übernimmt `name`, `provider`, `model`, `githubUser`,
`claudeConfigDir`, `gcloudAccount`, `gcloudProject`. Bestehende Projekte
funktionieren also ohne Migrationsschritt weiter; `limen show` weist die
Herkunft als `.orca/ (legacy)` aus.

## Tests

```bash
test/run.sh    # 39 Prüfungen, keine Netz- und keine Schlüsselbundschreibzugriffe
```

Geprüft wird unter anderem, dass der Schlüssel nie in `show`, `json` oder
`prompt` auftaucht, dass leere Felder nicht exportiert werden und dass
`limen shell` ohne Kontext still bleibt und mit 0 endet — sonst wäre es nicht
bedingungslos aus einer `.zshrc` aufrufbar.

## Warum Bash und nicht Go

Der erste Entwurf war Go, wegen ~2 ms Startzeit und robusterem Parsing. Auf
dieser Maschine ist keine Go-Toolchain installiert, und ein Artefakt, das
niemand kompilieren und testen kann, ist kein Artefakt. Die Bash-Fassung
läuft heute, ist geprüft und hat kein Buildsystem — was zum Anlass des Ganzen
passt. Sollte sie je in einem Profil auffallen, ist der Go-Port die
dokumentierte nächste Stufe; die Schnittstelle (`show`/`json`/`shell`/`prompt`)
ist dafür so gehalten, dass ein Austausch nichts weiter berührt.
