# limen

Kontext und Identität pro Verzeichnis — ein Go-Binary mit Deskriptor (.limen.yaml), Register und rollierenden Notizen als Ankerpunkt für Agenten

## Heimat & Identität (LevaraOrg)

- GitHub-Heimat: https://github.com/LevaraOrg/limen — Organisation **LevaraOrg** (umgezogen von `matthiaw` am 18.08.2026; alte URLs leiten weiter).
- Die Arbeitsidentität dieses Verzeichnisses kommt aus `.limen/limen.yaml` (Werkzeug: limen): githubUser `levaraleo`, actor Matthias, Claude-Config `~/.claude-levara`.
- Zweck laut Limen: Kontext und Identität pro Verzeichnis — ein Go-Binary mit Deskriptor (.limen.yaml), Register und rollierenden Notizen als Ankerpunkt für Agenten

## Geerbte Normen

Dieses Verzeichnis erbt `levara-baseline@1.0.0` (siehe `.limen/meta.yaml`).
Die Skills liegen materialisiert in `.claude/skills/`, die Begründungen als
ADRs in `docs/adr/`. Kurz:

- **ADR-0001** — alles Eingecheckte ist Englisch (Code, Kommentare, Doku,
  Commit-Messages, Testnamen). Das Gespräch bleibt Deutsch.
- **ADR-0002** — testgetrieben: erst der fallende Test, dann der kleinste Code,
  der ihn grün macht. Bugfixes sind nie ausgenommen.
- **ADR-0003** — tokensparsam: das billigste Werkzeug, das die Frage beantwortet;
  nie auf Kosten der Vollständigkeit.

`limen profile check` belegt, dass die Kopien unverändert sind — Exit 1 bei
Abweichung. Nicht die Dateien in `.claude/skills/` oder `docs/adr/` von Hand
bearbeiten: sie werden von `limen profile sync` überschrieben. Eine Norm ändert
man im Paket (`~/Documents/GitHub/levara-baseline`) und zieht die Version hoch.
