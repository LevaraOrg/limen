# LIMEN — rollierende Notizen zu limen

## 2026-08-11
- ✓ Idee aus Sprachnotiz 2026-08-11: prüfen, ob ein kind:-Feld (z.B. kind: service) sinnvoll ist — Agenten wüssten dann, dass eine service.yaml existieren sollte; die Prüfung selbst bleibt außerhalb von limen

## 2026-08-12
- ✓ Prüfung der kind:-Idee (2026-08-12): service.yaml existiert bereits in 6 Repos (nuncio, circlead-platform, signum, Tessera, agnostic-stack-tests, orca) nach Schema apiVersion: agnostic-stack/v1 — und trägt dort SELBST schon 'kind:' (Service bzw. TestOrchestrator). Ein eigenes kind: in limen.yaml wäre daher eine zweite Wahrheit, die auseinanderlaufen kann; das widerspricht 'der Service ist die harte Wahrheit'. Empfehlung: kein neues Feld, stattdessen Entdeckung — limen liest, falls vorhanden, service.yaml im Root und meldet apiVersion/kind/name in json und list (Nullpflege, keine Divergenz). Offen zur Entscheidung.
- Entscheidung + Umsetzung (2026-08-12): kein kind:-Feld im Deskriptor; stattdessen liest limen eine benachbarte service.yaml und meldet apiVersion/kind in show, json und list --json (v0.6.0, service.go). Belegt: 6 von 32 Kontexten tragen eine — darunter agnostic-stack-tests mit kind: TestOrchestrator, genau die Abweichung, die ein kopiertes Feld verfehlt hätte. Nebenbefund erledigt: signum hatte service.yaml, aber keinen Kontext — angelegt und registriert.
