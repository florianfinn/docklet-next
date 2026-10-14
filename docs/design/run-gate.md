# Run-Gate und Einzelinstanzbetrieb

## Geltung und Garantien

Pro PostgreSQL-Datenbank läuft genau ein Hub-Prozess in einem Hub-Container. Ein
zweiter Hub mit derselben Datenbank ist nicht unterstützt und darf keine Arbeit
annehmen. Agents sind getrennte Prozesse; diese Entscheidung begrenzt ihre Zahl
nicht. Der Record beschreibt den lebenden Hub sowie dessen Neustart und Absturz.
Ein abgestürzter Hub nimmt begonnene Operationen nicht automatisch wieder auf:
Ein Agent kann eine bereits angenommene Aktion trotzdem zu Ende führen. Darum
muss die tatsächliche Ressourcenlage vor einem erneuten Eingriff gelesen werden.

Ein Run hat genau einen aktiven Datensatz je `(environment_id, gate_scope)`.
`gate_key` bezeichnet die Operation. `queued` und `running` belegen den Scope;
`success`, `failed` und `cancelled` geben ihn frei. Ein gesetztes
`cancel_requested_at` ist ein dauerhafter Abbruchwunsch und **kein** zusätzlicher
Wert der Spalte `status`. Es gibt weder automatisches Replay einer Operation
noch eine Zusage über die atomare Ausführung externer Effekte. Die Garantie
betrifft die eindeutige Aufnahme und den einmaligen dauerhaften Abschluss des
Run-Datensatzes.

## Zustände und Übergänge

Die Tabelle ist der Vertrag für Gate-Runs. `message_code` bezeichnet den Code im
Run-Snapshot; ein zusätzlich erwähnter `activity_messages.code` gehört zum
dauerhaften Verlauf. Jeder terminale Schreibvorgang setzt `ended_at`,
`duration_ms` und `updated_at` in derselben Transaktion wie `status` und
`message_code`. Wiederholte oder verspätete Abschlüsse verändern einen
terminalen Run nicht und erzeugen keine zweite Abschlussnachricht.

| Von → nach | Auslöser und Bedingung | Geschriebene Spalten | Code |
| --- | --- | --- | --- |
| Kein Run → `queued` | Hub nimmt eine Operation mit Warteschlange auf. | `status`, `gate_scope`, `gate_key`, `started_at`, `message_code` | `activity.queued` |
| Kein Run → `running` | Hub nimmt eine sofort ausführbare Operation auf. | Wie oben, dazu `status=running` | `run.started` bei Gate-Aufnahme; sonst `activity.running` |
| `queued` → `running` | Der lokale Worker erhält seinen Ausführungsslot. | `status`, `updated_at`, `message_code` | `activity.running` |
| `queued`/`running` → Abbruch angefordert | Der Hub nimmt einen erstmaligen Abbruchwunsch an und signalisiert den lokalen Worker. `status` bleibt erhalten. | `cancel_requested_at`, `latest_message`, `message_code`, `updated_at`; zusätzlich eine Audit-Nachricht | `activity.cancel_requested` im Snapshot und in `activity_messages.code` |
| `queued`/`running` → gleicher aktiver Zustand | Der Worker meldet Fortschritt. | `progress`, `step`, `updated_at` und gegebenenfalls Nachrichten; ohne Abbruchwunsch auch `latest_message`, `message_code` | Ohne Abbruchwunsch der validierte Fortschrittscode, standardmäßig `activity.message`; bei Abbruchwunsch bleibt im Snapshot `activity.cancel_requested` stehen. |
| `queued`/`running` → `success` | Der Worker meldet Erfolg; kein Abbruchwunsch liegt vor. | Terminale Spalten, `progress=100`, gegebenenfalls Abschlussnachricht | `activity.success` |
| `queued`/`running` → `failed` | Der Worker meldet einen Fehler; kein Abbruchwunsch liegt vor. | Terminale Spalten, `error`, gegebenenfalls Abschlussnachricht | `activity.failed` |
| `queued`/`running` mit Abbruchwunsch → `cancelled` | Der Worker kehrt zurück, auch wenn er Erfolg oder Fehler meldet. | Terminale Spalten, Abschlussnachricht; kein Fehlertext aus dem überstimmten Ergebnis | `activity.cancelled` |
| `queued`/`running` → `failed` | Neustart-Reparatur erkennt einen Run des vorigen Hub-Prozesses ohne Abbruchwunsch. | Terminale Spalten, Unterbrechungsgrund und Abschlussnachricht | `activity.failed` |
| `queued`/`running` mit Abbruchwunsch → `cancelled` | Neustart-Reparatur erkennt einen Run des vorigen Hub-Prozesses mit dauerhaftem Abbruchwunsch. | Terminale Spalten und Abschlussnachricht | `activity.cancelled` |
| Terminal → derselbe Terminalzustand | Abschluss wird wiederholt, ein verspäteter Worker schreibt oder eine Reparatur wird erneut ausgeführt. | Keine | Vorhandener Code bleibt bestehen. |

Der Job-Run-Snapshot ist eine Projektion des dauerhaften Job-Koordinators. Ein
ausdrücklicher Retry darf ihn nach einem Terminalzustand wieder öffnen; das ist
ein neuer Koordinatorzustand und keine Wiederöffnung eines Gate-Runs. Seine
`job.*`-Codes und `activity_sync_receipts` folgen dem Koordinator und bleiben
von den oben genannten Gate-Übergängen getrennt.
Ein `queued`-Run mit gesetztem Abbruchwunsch darf keinen Slot mehr übernehmen;
sein Wartender geht direkt in den terminalen Abbruch. Ein gleichzeitiger
Slot-Erhalt und Abbruch wird anhand der dauerhaft gespeicherten Reihenfolge
entschieden, ohne den Abbruchwunsch zu verlieren.

## Aufnahme und Gate

Die Aufnahme sperrt den Scope innerhalb einer PostgreSQL-Transaktion, liest
aktive Runs und legt höchstens einen neuen Run an. Die bestehende
transaktionsgebundene Advisory-Sperre pro `(environment_id, gate_scope)` bleibt
für gleichzeitig eintreffende Anfragen im einen Hub erhalten. Der
Einzelinstanz-Schutz weiter unten hat eine andere Aufgabe und ersetzt die
atomare Scope-Aufnahme nicht.

| Befund bei der Aufnahme | Ergebnis | Datenbankänderung |
| --- | --- | --- |
| Aktiver Run mit demselben `gate_key`, ohne Abbruchwunsch | `run.joined` mit dessen ID; nur der ursprüngliche Aufruf besitzt einen Worker. | Keine |
| Aktiver Run mit anderem `gate_key` | Konflikt `run.busy` mit der aktiven Run-ID. | Keine |
| Aktiver Run mit demselben `gate_key` und Abbruchwunsch | Konflikt `run.cancelling` mit der aktiven Run-ID, auch bei `Check`. | Keine |
| Kein aktiver Run; `Check=true`, `Recheck=false`; letzter erfolgreicher Run derselben Operation und desselben Scopes endete vor weniger als 30 Sekunden | `run.fresh` mit dessen ID. | Keine |
| Sonst | `run.started` und eine neue Run-ID; genau dieser Aufruf beginnt die Arbeit. | Neuer aktiver Run |

Die Aktivprüfung kommt immer vor der Frischeprüfung. `Recheck=true` umgeht nur
das 30-Sekunden-Ergebnis, niemals einen aktiven Run. Nur ein erfolgreicher
Abschluss zählt als frisch; die Frist vergleicht `ended_at` mit PostgreSQL-Zeit.
Nach Fristablauf oder `Recheck` wird die Ressourcenlage vor der Ausführung neu
gelesen. Ein Join, eine Ablehnung und ein Frischetreffer starten keinen Worker
und schreiben keinen neuen Run-Code in den bestehenden Datensatz.

## Abbruch

Der erste Abbruchwunsch schreibt `cancel_requested_at` mit PostgreSQL-Zeit,
`activity.cancel_requested` und die anfordernde Person als Audit-Nachricht in
einer Transaktion. Danach veröffentlicht der Hub Nachricht und Snapshot und
signalisiert den lokal registrierten Worker. Ein erneuter Wunsch ist idempotent:
kein zweiter Audit-Eintrag, kein neuer Zeitpunkt. Das Signal allein ist nicht
dauerhaft; die Spalte ist es. Die lokale Worker-Registrierung muss bei der
Aufnahme vorhanden sein, bevor ein Abbruch angenommen werden kann.

Während des Abwickelns bleiben `status` und Scope aktiv. Fortschritts- und
Metadaten-Schreibwege dürfen weiterhin arbeiten; sie erhalten aber
`cancel_requested_at`, `message_code=activity.cancel_requested` und die
Abbruchmeldung in `latest_message` bis zum terminalen Abschluss. Spätere
Fortschrittsnachrichten behalten ihren eigenen Code im Nachrichtenverlauf.
Diese Regel macht den Abbruch im Snapshot sichtbar, auch wenn der Worker noch
meldet. Service-Schreibwege für den besitzenden Worker lösen ihre Datenbankarbeit
selbst mit einer neuen begrenzten Schreibfrist vom abgebrochenen Arbeitskontext;
einzelne Aufrufer müssen das nicht nachholen.

Erst die Rückkehr des Workers und sein terminaler Schreibvorgang geben den Scope
frei. Ein Worker, der nie zurückkehrt, hält ihn bis zum Ende des Hub-Prozesses.
Die Reparatur des nächsten Hub-Prozesses schließt den Run danach ab. Ein
abgebrochener Agent-Aufruf kann außerhalb des Hubs bereits Wirkung gezeigt
haben; `cancelled` ist keine Rollback-Zusage.

## Neustart, Absturz und einmaliger Abschluss

Nach Erwerb der Einzelinstanz-Sperre und vor der Aufnahme neuer Arbeit repariert
der Hub alle vom vorigen Prozess verbliebenen Gate-Runs in `queued` oder
`running` sofort. Er wartet nicht auf ein Lease-Alter: Im neuen Prozess gibt es
keinen alten lokalen Worker. Mit `cancel_requested_at` werden sie `cancelled`,
sonst `failed`; beide Fälle erhalten einen verständlichen Unterbrechungstext,
einen passenden `activity.<status>`-Code und einen terminalen Zeitpunkt. Die
Reparatur ist wiederholbar und erfasst auch Runs, die zwischen Erzeugung und
Worker-Start oder während eines Abbruchs liegen geblieben sind. Ungated
verwaiste Queue- und Laufzustände werden ebenfalls repariert; spezielle
Koordinatoren, etwa Job-Runs oder ein belegter Self-Update-Abschluss, werden
über ihren eigenen dauerhaften Zustand abgeglichen.

Der terminale Schreibvorgang prüft atomar `status IN ('queued', 'running')` und
schreibt Status, Code und letzte Nachricht in einer Transaktion. Ein zweiter
Abschluss trifft keine aktive Zeile und erzeugt keine weitere Abschlussnachricht.
Stürzt der Hub nach Commit, aber vor dem Live-Event ab, liest der neue Hub den
terminalen Snapshot; Clients laden ihn nach Reconnect neu. Stürzt er vor dem
Commit ab, repariert der neue Hub den noch aktiven Run. Der Abschluss-Effekt
des Job-Koordinators nutzt zusätzlich `activity_sync_receipts`: Run-ID und
zuletzt angewandte Koordinatorversion werden zusammen mit Projektion und
Nachricht transaktional geschrieben. Replay derselben Version erzeugt keinen
zweiten Effekt, auch nach Pruning der Activity-Historie. Der Receipt bleibt bis
zum belegten Ausscheiden des Koordinator-Runs erhalten. Diese Regeln garantieren
keine genau einmalige externe Agent-Aktion; nach einem Absturz ist deren
tatsächlicher Ausgang zu prüfen.

## Einzelinstanz-Schutz und Datenbankverlust

Der Hub erwirbt beim Start vor Migration, Reparatur, Hintergrundjobs und
HTTP-Bereitschaft mit `pg_try_advisory_lock` eine feste, namensraumgebundene
PostgreSQL-**Session**-Sperre auf einer eigens reservierten physischen
Verbindung. Die Verbindung bleibt bis nach dem Stoppen aller Worker offen;
eine Transaktionssperre oder eine gewöhnliche Pool-Verbindung reicht dafür
nicht. Ist die Sperre belegt, endet der Start mit dem stabilen operativen
Fehlercode `hub.already_running`, ohne Jobs oder HTTP zu starten. Der Code ist
ein Startfehler und gehört nicht zum Run-Nachrichtenkatalog.

Verliert der Hub die Sperrverbindung oder kann er ihren Besitz nicht mehr
prüfen, setzt er seine Bereitschaft bei Erkennung auf falsch, nimmt keine neue Arbeit
an, signalisiert laufenden Workern das Ende und beendet den Prozess. Er darf
auf einer automatisch neu verbundenen Pool-Verbindung nicht weitermachen:
Andernfalls könnte ein zweiter Hub die freigewordene Sperre bereits halten.
Ein neuer Prozess erwirbt die Session-Sperre erneut und repariert die
verbliebenen Runs vor dem Öffnen der API. Bei einer Netztrennung, in der
PostgreSQL die alte Session noch für lebendig hält, wartet der Start auf deren
Freigabe beziehungsweise scheitert sichtbar mit `hub.already_running`; er
umgeht die Sperre nicht. Der Verlust im laufenden Prozess wird als
`hub.db_lock_lost` gemeldet. Kontrolliertes Herunterfahren gibt die Sperre erst
frei, wenn keine Worker mehr schreiben können; bei einem nicht endenden Worker
endet der Prozess statt die Sperre vorzeitig freizugeben.

## Abgleich mit der Mehrprozess-Mechanik aus PR #42

Die Entscheidung in [#43](https://github.com/florianfinn/docklet-next/issues/43)
ändert die Betriebsannahme von [PR #42](https://github.com/florianfinn/docklet-next/pull/42).
Bereits ausgelieferte Migrationen bleiben unverändert; ein Schemaabbau erfolgt
gegebenenfalls nur durch eine neue Vorwärtsmigration.

| Baustein | Entscheidung | Grund und Zielzustand |
| --- | --- | --- |
| Transaktionsgebundene Advisory-Sperre je Run-Scope | Behalten | Sie serialisiert gleichzeitig eintreffende Anfragen im einen Hub bis zum Commit. |
| Dauerhafter Abbruchwunsch, Audit-Nachricht und sofortiges Live-Snapshot | Behalten | Ein Abbruch übersteht den Hub-Absturz und bleibt für Clients sichtbar. |
| Owner-Token `gate_owner` und Fencing jedes Fortschritts- und Abschluss-Schreibers | Entfernen | Es gibt keinen zweiten lebenden Hub-Owner. Nach Verlust der Session-Sperre beendet sich der alte Prozess; der neue repariert erst nach Erwerb der Sperre. Terminale Statusbedingungen bleiben als Schutz gegen doppelte Abschlüsse. |
| `gate_lease_at` und 30-Sekunden-Heartbeat | Entfernen | Der Lebenszeit-Lock entscheidet über den Hub; ein Lease würde einen lebenden, aber lange abbrechenden Worker fälschlich freigeben können. |
| Zweiminütiger Sweep mit Token-Rotation und Übernahme gegateter Runs | Entfernen | Der neue Hub repariert nach exklusivem Start sofort. Der periodische Sweep für ungated verwaiste Runs bleibt unabhängig davon erhalten. |
| Remote-Cancel-Beobachtung durch den Heartbeat eines zweiten Service | Entfernen | Abbruchsignal und Worker liegen im einzigen Hub. Der dauerhafte Datenbankeintrag bleibt für Crash-Recovery. |
| PostgreSQL-Zeit für Gate-Start, Abbruch, Terminalzeit und 30-Sekunden-Frische | Behalten | `NOW()` ist an den ohnehin nötigen Schreib- und Lesewegen verfügbar und hält die Frist konsistent. Hub-übergreifende Clock-Skew-Tests entfallen. |
| Atomarer terminaler Statuswechsel und `activity_sync_receipts` | Behalten | Sie verhindern doppelte Abschlussnachrichten und doppelte Job-Projektion bei Replay oder Neustart. |

Die Implementierung darf den Owner-/Lease-Code erst entfernen, wenn die
Einzelinstanz-Sperre samt Fail-Stop und die sofortige Reparatur wirksam und
getestet sind. Zwischenzustände dürfen weder zwei Hubs zulassen noch einen
aktiven Scope vor Rückkehr seines Workers freigeben.

## Offene Review-Hinweise für den Implementierungsschritt

Die folgenden Punkte stammen aus [Review-Runde 5 zu PR #42](https://github.com/florianfinn/docklet-next/pull/42#issuecomment-6175086772)
und sind hier fachlich entschieden, aber noch nicht umgesetzt:

| Hinweis | Umsetzung und Abnahme |
| --- | --- |
| N13 | `cancelRequestedAt` in den Run-Snapshot und das generierte DTO aufnehmen. Nach dem Abbruch darf ein Fortschritts- oder Metadaten-Update `messageCode=activity.cancel_requested` und `latestMessage` nicht überschreiben; die einzelnen Fortschrittsnachrichten bleiben im Verlauf. |
| N14 | Owner-Schreibwege `UpdateActivity`, `AppendMessages` und `PatchActivityMetadata` lösen den Datenbankkontext innerhalb des Service vom abgebrochenen Arbeitskontext, ohne die Owner-Identität beziehungsweise den lokalen Run-Bezug zu verlieren. So kann der Worker beim Abwickeln weiter protokollieren; der Caller braucht kein eigenes `context.WithoutCancel`. |
| N15 | Den Kommentar zu `CancelActivity` auf den gültigen aktuellen Vertrag verkürzen und grammatisch korrigieren. |

## Testmatrix für den Implementierungsschritt

Jede Zeile braucht einen automatisierten Test und einen gezielten Gegenversuch:
Die genannte Mutation muss den Test rot machen. Für Neustarttests werden zwei
aufeinanderfolgende Hub-Lebenszeiten gegen dieselbe Testdatenbank verwendet;
der erste Prozess beziehungsweise seine Session wird tatsächlich beendet. Ein
zweiter Service im selben Prozess ersetzt den Einzelinstanztest nicht.

| Fall | Erwartung | Mutation, die den Test rot machen muss |
| --- | --- | --- |
| 1. Neue wartende Operation | `queued`, Scope belegt, `activity.queued`. | Start direkt als `running` speichern. |
| 2. Neue sofortige Gate-Operation | `running`, `run.started`, genau ein Worker. | Worker zweimal starten. |
| 3. Slot wird frei | `queued` → `running`, `activity.running`. | Status- oder Code-Update weglassen. |
| 4. Gleiche Operation trifft aktiven Run | `run.joined`, gleiche ID, kein weiterer Worker. | Zweiten Run anlegen. |
| 5. Andere Operation trifft aktiven Run | `run.busy`, aktive ID, keine Änderung. | Konflikt zulassen. |
| 6. Gleiche Operation trifft abbrechenden Run | `run.cancelling`, auch mit `Check`, keine Arbeit. | Wieder `run.joined` liefern. |
| 7. Erster Abbruch bei `queued` | Wunsch und Audit-Nachricht dauerhaft, lokaler Wartender signalisiert, Status bleibt `queued`. | `cancel_requested_at` nicht schreiben. |
| 8. Abbruch und Slot-Zuteilung konkurrieren | Ein abbrechender Wartender beginnt keine neue Arbeit und schließt als `cancelled`. | Wartenden trotz `cancel_requested_at` starten. |
| 9. Erster Abbruch bei `running` | Wunsch, Audit-Nachricht, Snapshot und lokales Signal; Scope bleibt belegt. | Nur Kontext abbrechen, ohne dauerhaften Wunsch. |
| 10. Wiederholter Abbruch | Zeitpunkt, Audit-Anzahl und Status bleiben gleich. | Zweite Audit-Nachricht schreiben. |
| 11. Fortschritt ohne Abbruch | Schritt, Fortschritt und validierter Code im Snapshot und Verlauf. | Fortschrittscode nicht aktualisieren. |
| 12. Fortschritt nach Abbruch | Nachricht wird gespeichert; `cancelRequestedAt`, Snapshot-Code und Abbruchtext bleiben sichtbar. | `latestMessage` durch Fortschritt ersetzen. |
| 13. Abgebrochener Worker schreibt weiter | Service-Methoden akzeptieren seinen abgebrochenen Arbeitskontext. | Datenbankzugriff wieder an `ctx.Done()` koppeln. |
| 14. Worker meldet Erfolg | Ein `success`-Abschluss, `activity.success`, `progress=100`, eine Abschlussnachricht. | Terminal-Code weglassen. |
| 15. Worker meldet Fehler | Ein `failed`-Abschluss mit Fehlertext und `activity.failed`. | Status als `success` speichern. |
| 16. Worker meldet nach Abbruch Erfolg oder Fehler | Ein `cancelled`-Abschluss mit `activity.cancelled`, ohne überstimmten Fehlertext. | Abbruchwunsch bei Completion ignorieren. |
| 17. Worker kehrt nach Abbruch lange nicht zurück | Scope bleibt bis Rückkehr oder Prozessende belegt, auch nach mehr als zwei Minuten. | Gegateten Run nach Alter sweepen. |
| 18. Doppelter oder verspäteter Abschluss | Genau ein terminaler Zustand und eine Abschlussnachricht. | Statusbedingung aus dem `UPDATE` entfernen. |
| 19. `Check` innerhalb von 30 Sekunden nach Erfolg | `run.fresh`, gleiche ID, kein Worker. | Vergleich der Frist entfernen. |
| 20. `Check` bei genau 30 Sekunden | Neuer Run an der exklusiven Fristgrenze. | `>=` statt `>` an der Frist verwenden. |
| 21. `Check` nach Fehler | Neuer Run; nur Erfolg zählt als frisch. | Fehler als frisch zählen. |
| 22. `Recheck` innerhalb der Frist | Neuer Run, sofern kein Run aktiv ist. | `Recheck` ignorieren. |
| 23. `Recheck` bei aktivem Run | Join, `run.busy` oder `run.cancelling` hat Vorrang. | Frische-/Recheck-Zweig vor Aktivprüfung ziehen. |
| 24. Gleichzeitige Aufnahme desselben Scopes | Eine Run-ID und höchstens ein Worker über konkurrierende Anfragen. | Scope-Transaktionssperre entfernen. |
| 25. Absturz während `running` | Neuer Hub markiert Run vor API-Freigabe `failed`; Scope ist danach frei. | Reparatur um zwei Minuten verzögern. |
| 26. Absturz während `queued` | Neuer Hub markiert den verwaisten Run `failed`, ohne ihn auszuführen. | Wartenden Run beim Start erneut ausführen. |
| 27. Absturz während des Abbruchs | Neuer Hub markiert Run `cancelled`, mit passendem Code; keine doppelte Audit-Nachricht. | `cancel_requested_at` bei Reparatur ignorieren. |
| 28. Absturz vor dem Completion-Commit | Neuer Hub schreibt genau einen Reparaturabschluss mit einer Nachricht. | Reparatur aus dem Startpfad entfernen. |
| 29. Absturz nach Completion-Commit vor Live-Event | Terminaler Run bleibt unverändert; Reconnect lädt den Snapshot. | Reconnect ohne Snapshot-Neuladung behandeln. |
| 30. Job-Completion nach Neustart oder Pruning erneut synchronisiert | Receipt verhindert zweite Projektion/Nachricht für dieselbe Version. | Receipt-Prüfung entfernen. |
| 31. Zweiter Hub startet bei gehaltener Sperre | Kein HTTP, keine Jobs, Fehler `hub.already_running`. | Session-Sperrenprüfung überspringen. |
| 32. Sperrverbindung des laufenden Hubs fällt aus | Bereitschaft fällt, neue Arbeit stoppt, Prozess endet mit `hub.db_lock_lost`. | Pool-Reconnect ohne Fail-Stop erlauben. |
| 33. Datenbankverbindung kehrt zurück | Nur ein neuer Hub erwirbt die Sperre und repariert vor API-Freigabe; alter Prozess arbeitet nicht weiter. | Reparatur erst nach Öffnen der API starten. |
| 34. Alte Session bleibt während Netztrennung gesperrt | Neustart meldet `hub.already_running` und nimmt keine Arbeit an. | Lock-Fehler als Warnung behandeln. |

## Folgen für #38: HTTP- und Live-Adapter

Der Adapter darf sich auf `id`, `status`, `messageCode`, `latestMessage`,
`startedAt`, `endedAt`, `affectedServices` und das neu auszugebende
`cancelRequestedAt` im Snapshot stützen. Ein aktiver Run mit
`cancelRequestedAt` bleibt `queued` oder `running`; der Zeitstempel ist die
maßgebliche Abbruchanzeige. `run.started`, `run.joined` und `run.fresh` sind
Aufnahmeergebnisse. `run.busy` und `run.cancelling` sind codierte Konflikte mit
der aktiven Run-ID. Nur `run.started` enthält einen ausführbaren Arbeitskontext;
der HTTP-Adapter gibt diesen niemals an den Client weiter.

Der Live-Kanal veröffentlicht bei Start, Fortschritt, Abbruchwunsch und
terminalem Abschluss Activity-Snapshots; beim Abbruch und beim Abschluss
zusätzlich die jeweiligen Nachrichten. Ein Live-Event ist ein Hinweis auf
einen geänderten Snapshot, keine dauerhafte Zustellgarantie. Nach Reconnect
oder einem erkannten Event-Verlust lädt der Adapter betroffene Runs und Themen
neu. So erscheint auch ein Abschluss, dessen Datenbank-Commit vor dem Absturz
lag, dessen Event aber nicht mehr gesendet wurde. Der Adapter interpretiert
`cancelled` nicht als Beweis, dass ein Agent eine bereits angenommene externe
Aktion rückgängig gemacht hat.
