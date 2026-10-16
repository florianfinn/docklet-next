# Practical acceptance in a sandbox

Practical acceptance demonstrates a specified product revision and package against
real dependencies in a maintainer-approved isolated sandbox. Unit tests remain
independent of real services, Docker sockets and running agents. M0 establishes
this procedure only and deploys nothing.

Before a run, create an acceptance issue stating goal, exact commit/package and
image digests, prerequisites, permitted systems, module owner, test steps, expected
results, cleanup and recovery. Get explicit approval for the particular deployment;
a subagent must not alter a running system. Use origin/main for deployment.
Reserve test resources according to the sandbox owner's rules. Use synthetic data
and separate credentials; never reuse operating data or production secrets.

Back up the database before schema changes and record the backup verification
privately. Verify startup migrations, /health, served bundle and the tested feature
against the real core. Check error and recovery paths, agent/protocol compatibility,
restart persistence and installation instructions as applicable. A browser demo or
local unit test cannot establish these results.

For UI acceptance exercise 320/390/1600 pixel widths, light/dark and English/German,
including loading, empty, error and success states. Review screenshots for
publishability; only allowlisted synthetic evidence belongs in the public repo.
Public evidence identifies the revision, measurements, steps, results and limits
without hostnames, addresses, secrets or private links. Keep operational logs private.

Record cleanup, remaining risks and follow-ups. Link acceptance evidence, independent
review, CI and merge SHA from the closing issue. A release is accepted only after
all required features and the exact release package pass; source tests alone do
not authorise publication. Failures block release until fixed and rechecked.

For repository acceptance, verify with a test PR that the first pull_request_target
run creates the required review commit status before enabling the ruleset. The
OWNER comment attests to review; it cannot prove provider independence. Record
manual publication review of allowlisted binaries, leading-zero address notation
and mixed-script homograph emails in the acceptance evidence.

Enable required code-owner review for all governance paths in .github/CODEOWNERS,
including guard code, tests, policies, licences, hooks and design documents.
Record the accepted root SHA and verify that obsolete local roots are unreachable
and pruned before first publication. Rewriting the private root needs renewed
review of the resulting SHA; earlier review does not attest to changed content.

## Praktische Abnahme: Backup der Hub-Datenbank

Diese Schritte führt der Maintainer in einer ausdrücklich freigegebenen, isolierten
Testumgebung mit erfundenen Daten aus. Der Betreiber bereitet die Umgebung vor;
dieses Dokument erteilt keine Freigabe für ein Deployment. Notiere die geprüfte
Produktversion, das Paket und die Bild-Digests im Abnahme-Issue.

1. Lege im Test-Hub gut wiedererkennbare Beispieldaten an: einen zusätzlichen
   Benutzer, eine Einstellung und einen geplanten Auftrag. Notiere die Werte,
   ohne Zugangsdaten oder andere Geheimnisse in öffentliche Nachweise zu kopieren.
2. Öffne als globaler Administrator **Hub-Datenbank**. Prüfe, dass genügend freier
   Platz angezeigt wird. Klicke **Backup erstellen** und warte, bis das Archiv
   **Abgeschlossen** zeigt. Notiere Archiv-ID, Erstellungszeit und Größe.
3. Lade dieses Archiv über **Herunterladen** auf einen geschützten Speicherort
   außerhalb des Hub-Hosts. Prüfe, dass die Datei vorhanden und nicht leer ist.
4. Lass den Betreiber den Hub stoppen und eine neue, leere PostgreSQL-18-Datenbank
   in der freigegebenen Testumgebung bereitstellen. Der Hub darf diese leere
   Datenbank vor der Wiederherstellung nicht starten. Der Betreiber legt die
   heruntergeladene Datei als `<Archiv-ID>.dump` in das Backup-Volume und führt
   den in `deploy/hub/README.md` beschriebenen Befehl
   `restore-hub --backup-id <Archiv-ID>` aus.
5. Starte den Hub. Prüfe, dass der Health-Endpunkt bereit ist und eine neue
   Anmeldung nötig ist. Melde dich an und vergleiche Benutzer, Einstellung,
   Auftrag, Aktivitätsverlauf und Migrationsstand mit den vor Schritt 2
   notierten Werten. Die Daten müssen übereinstimmen. Prüfe außerdem, dass
   frühere Sitzungen keinen Zugriff mehr haben.
6. Halte Ergebnis, Archivgröße, Zeitpunkt, Paketversion, etwaige Abweichungen
   und Aufräumen im Abnahme-Issue fest. Bei einem Unterschied gilt die Abnahme
   als fehlgeschlagen; bewahre Archiv und private Fehlerbelege für die Diagnose.
