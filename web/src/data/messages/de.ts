import type { enBackend } from "./en";

export const deBackend: typeof enBackend = {
  errors: {
    "image": {
      "active-update-protected": "Ein aktiver oder ungeklärter Lauf schützt dieses Image.",
      "busy": "Ein anderer Image-Vorgang läuft. Bitte erneut versuchen.",
      "changed": "Das Image hat sich während des Vorgangs geändert. Aktualisieren und erneut versuchen.",
      "in-use": "Dieses Image wird von einem laufenden oder gestoppten Container verwendet.",
      "invalid-archive": "Das Archiv kann nicht sicher geprüft werden.",
      "invalid-prune": "Einen gültigen Bereinigungsmodus und ein Alter oder einen Zeitstempel wählen.",
      "invalid-reference": "Eine gültige Image-Referenz oder einen Suchbegriff eingeben.",
      "load-failed": "Das Image-Archiv konnte nicht geladen werden. Docker-Verbindung prüfen und erneut versuchen.",
      "not-found": "Das Image wurde nicht gefunden. Image-Liste aktualisieren.",
      "prune-failed": "Einige Images konnten nicht bereinigt werden. Vor einem erneuten Versuch das Ergebnis prüfen.",
      "pull-failed": "Das Image konnte nicht heruntergeladen werden. Referenz, Registry-Zugriff und Docker-Verbindung prüfen.",
      "remove-failed": "Das Image konnte nicht entfernt werden. Verwendung aktualisieren und erneut versuchen.",
      "reserved-reference": "Diese Referenz ist für die interne Aufbewahrung reserviert.",
      "restore-point-protected": "Ein Wiederherstellungspunkt schützt dieses Image.",
      "system-protected": "Dieses Image wird vom System benötigt.",
      "tag-failed": "Der Tag konnte nicht erstellt werden. Quell-Image und Zielreferenz prüfen.",
      "usage-unavailable": "Image-Informationen oder die Verwendung konnten nicht geprüft werden. Vor einem erneuten Versuch die Docker- und Registry-Verbindung prüfen."
    },
    "registry": {
      "authentication-failed": "Die Registry hat die gespeicherten Zugangsdaten abgelehnt. Zugangsdaten ersetzen und erneut testen.",
      "authentication-required": "Diese Registry erfordert Zugangsdaten. Zugangsdaten speichern und erneut testen.",
      "credential-unavailable": "Registry-Zugangsdaten benötigen einen gültigen Verschlüsselungsschlüssel.",
      "exists": "Eine Registry mit dieser Identität ist bereits vorhanden.",
      "inline-credentials-not-allowed": "Gespeicherte Registry-Zugangsdaten oder anonymen Zugriff verwenden.",
      "invalid-config": "Registry-URL, Typ und erforderliche Einstellungen prüfen.",
      "not-found": "Registry nicht gefunden.",
      "persistence-failed": "Registry-Einstellungen konnten nicht gespeichert werden. Erneut versuchen.",
      "rate-limited": "Das Anfragelimit der Registry wurde erreicht. Vor einem erneuten Versuch warten.",
      "sync-failed": "Registry-Einstellungen konnten nicht mit dem Agent synchronisiert werden. Verbindung prüfen und erneut versuchen.",
      "unavailable": "Die Registry konnte nicht erreicht werden. Adresse und Netzwerkverbindung prüfen."
    },
    "restore": {
      "unavailable": "Dieser Wiederherstellungspunkt ist unvollständig oder seine aufbewahrten Images sind nicht verfügbar.",
      "shared-reference": "Ein anderer Workload verhindert das Verschieben einer gemeinsamen Image-Referenz. Den Konflikt vor der Wiederherstellung beheben.",
      "image-mismatch": "Die wiederhergestellten Image-Zuordnungen konnten nicht geprüft werden. Den Lauf vor einem erneuten Versuch prüfen.",
      "in-use": "Ein aktiver oder ungeklärter Lauf hält diesen Wiederherstellungspunkt noch fest.",
      "not-found": "Der Wiederherstellungspunkt wurde nicht gefunden. Die Liste der Punkte aktualisieren.",
      "deploy-failed": "Die Bereitstellung zur Wiederherstellung ist fehlgeschlagen. Das Teilergebnis vor einem weiteren Wiederherstellungsversuch prüfen."
    },
    "update": {
      "agent-local-job-disabled": "Update-Jobs werden vom Hub verwaltet und können auf einem Agent nicht eigenständig laufen.",
      "auto-excluded": "Dieser Stack enthält einen Dienst, der von automatischen Updates ausgeschlossen ist.",
      "invalid-policy": "Eine gültige Stack-Richtlinie oder einen Update-Zeitplan mit sechs Feldern wählen.",
      "policy-changed": "Die Update-Richtlinie oder der Zeitplan wurde geändert. Vor einem neuen Versuch aktualisieren.",
      "schedule-hub-owned": "Update-Prüfungen und automatische Updates auf dem Hub planen.",

      "config-drift": "Die gespeicherten Stack-Änderungen vor dem Update bereitstellen oder verwerfen.",
      "baseline-unavailable": "Den Stack einmal bereitstellen, um eine geprüfte Konfigurationsbasis zu erfassen.",
      "snapshot-unavailable": "Ein vollständiger Wiederherstellungspunkt konnte nicht erfasst werden. Stack-Eingaben und verfügbare Images prüfen.",
      "target-changed": "Der zugelassene Stack oder seine Image-Zuordnung hat sich geändert. Vor einem erneuten Versuch aktualisieren.",
      "image-binding-unsupported": "Diese Image-Zuordnung oder der Image-Speicher des Daemons unterstützt das gewählte Update nicht sicher.",
      "path-retired": "Die Stack-Image-Update-Aktion statt dieses stillgelegten Update-Einstiegs verwenden.",
      "target-not-found": "Das Stack-Update-Ziel ist nicht mehr vorhanden. Die Stack-Liste aktualisieren.",

      "check-unavailable": "Die Update-Prüfung konnte nicht abgeschlossen werden.",
      "deploy-failed": "Das Update konnte nicht bereitgestellt werden. Ergebnis und Host-Verbindung prüfen.",
      "invalid-target": "Ein gültiges ausdrückliches Update-Ziel auswählen.",
      "persistence-failed": "Das Update-Ergebnis konnte nicht gespeichert werden. Vor einem erneuten Versuch den Status aktualisieren.",
      "requires-hub-update": "Vor diesem Agent-Upgrade zuerst den Hub aktualisieren.",
      "system-version-unknown": "Ein kompatibles Release-Ziel konnte nicht bestätigt werden."
    }
  },
  title: "Kein Backend verbunden",
  hint: "Dieser Build ist noch nicht mit einem Hub verbunden. Lokale Darstellungseinstellungen sind verfügbar.",
};
