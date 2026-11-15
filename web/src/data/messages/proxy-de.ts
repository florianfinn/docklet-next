// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original total proxy wire vocabulary and bilingual presentation keys.

export const deProxyContract = {
  "states": {
    "pending": "Ausstehend",
    "verified": "Verifiziert",
    "applied-unverified": "Angewendet, nicht verifiziert",
    "rejected": "Abgelehnt",
    "restored": "Wiederhergestellt",
    "recovery-required": "Wiederherstellung erforderlich",
    "released": "Freigegeben",
    "not-applied": "Nicht angewendet"
  },
  "phases": {
    "admitted": "Angenommen",
    "staging": "Vorbereiten",
    "publishing": "Veröffentlichen",
    "reloading": "Neu laden",
    "restarting": "Neu starten",
    "observing": "Prüfen",
    "marking": "Laden der Konfiguration prüfen",
    "restoring": "Wiederherstellen"
  },
  "kinds": {
    "traefik": "Traefik",
    "caddy": "Caddy"
  },
  "sources": {
    "project": "Projektverzeichnis",
    "volume": "Benanntes Volume",
    "bind": "Bind-Mount",
    "single-file": "Einzeldatei-Mount",
    "unknown": "Unbekannte Quelle"
  },
  "references": {
    "localReference": "Lokal",
    "plugin": "Plugin",
    "internalReference": "Intern",
    "external": "Extern",
    "opaque": "In einer ungelesenen Datei",
    "unresolved": "Nicht aufgelöst"
  },
  "reasons": {
    "no-observable-difference": "Kein beobachtbarer Unterschied.",
    "not-observable": "Die Wirkung kann nicht beobachtet werden.",
    "container-changed": "Der Container hat sich während des Vorgangs geändert.",
    "logs-unavailable": "Container-Protokolle sind nicht verfügbar.",
    "generation-mismatch": "Das Agent-Journal gehört zu einer anderen Generation.",
    "operation-conflict": "Die Vorgangs-ID gehört zu einer anderen Anfrage.",
    "module-busy": "Ein anderer Vorgang ist noch nicht abgeschlossen.",
    "stale-hash": "Eine Konfigurationsdatei wurde geändert.",
    "source-read-only": "Die Konfigurationsquelle ist schreibgeschützt.",
    "unknown-operation": "Der Vorgang ist unbekannt.",
    "operation-fenced": "Die Vorgangs-ID wurde gesperrt.",
    "registration-changed": "Die Host-Registrierung wurde geändert.",
    "journal-lost": "Das aufgezeichnete Journal kann nicht bestätigt werden.",
    "invalid-input": "Die Anfrage ist ungültig.",
    "input-limit": "Eine Grenze für Ein- oder Ausgaben wurde überschritten.",
    "exec-failed": "Der Proxy-Befehl ist fehlgeschlagen.",
    "exec-running": "Der Proxy-Befehl wird möglicherweise noch ausgeführt.",
    "proxy-rejected": "Der Proxy hat die Konfiguration abgelehnt.",
    "restore-failed": "Die vorherige Konfiguration konnte nicht wiederhergestellt werden.",
    "invalid-graph": "Die Konfiguration enthält ungültige Eingaben.",
    "incomplete-inventory": "Die Proxy-Bestandsaufnahme ist unvollständig.",
    "opaque-input": "Diese Eingabe darf nicht gelesen werden.",
    "single-file-mount": "Die Konfiguration ist als Einzeldatei eingebunden.",
    "refused-bind": "Diese Bind-Quelle darf nicht beschrieben werden.",
    "read-only-mount": "Die Quelle ist schreibgeschützt eingebunden.",
    "unknown-source": "Die Art der Quelle ist unbekannt.",
    "unsupported-metadata": "Die Dateimetadaten können nicht sicher erhalten werden.",
    "watch-off": "Die Dateiüberwachung ist deaktiviert.",
    "unsupported-format": "Das Konfigurationsformat wird nicht unterstützt.",
    "template-input": "Vorlageneingaben können nicht bearbeitet werden.",
    "rule-syntax-v2": "Diese Route verwendet die Regelsyntax v2.",
    "shared-across-files": "Der Dienst wird dateiübergreifend verwendet.",
    "persistence-off": "Das dauerhafte Speichern der Konfiguration ist deaktiviert.",
    "relative-import": "Dieser relative Import wird nicht unterstützt.",
    "import-missing": "Der Routenimport fehlt.",
    "acknowledgement-required": "Vor dem Schreiben ist eine Bestätigung erforderlich.",
    "probe-unavailable": "Der Prüfbefehl ist nicht verfügbar.",
    "probe-option-unavailable": "Eine erforderliche Prüfoption ist nicht verfügbar.",
    "admin-unavailable": "Die aktive Konfiguration kann nicht gelesen werden.",
    "agent-unavailable": "Der Host ist nicht erreichbar.",
    "engine-unavailable": "Diese Proxy-Engine ist nicht verfügbar.",
    "target-changed": "Das Ziel weicht von beiden aufgezeichneten Zuständen ab."
  }
};
