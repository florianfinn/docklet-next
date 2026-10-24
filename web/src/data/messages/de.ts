import type { enBackend } from "./en";

export const deBackend: typeof enBackend = {
  errors: {
    "image": {
      "active-update-protected": "Ein aktiver oder ungeklärter Lauf schützt dieses Image.",
      "busy": "Ein anderer Image-Vorgang läuft. Bitte erneut versuchen.",
      "changed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "in-use": "Dieses Image wird von einem laufenden oder gestoppten Container verwendet.",
      "invalid-archive": "Das Archiv kann nicht sicher geprüft werden.",
      "invalid-prune": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "invalid-reference": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "load-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "not-found": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "prune-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "pull-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "remove-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "reserved-reference": "Diese Referenz ist für die interne Aufbewahrung reserviert.",
      "restore-point-protected": "Ein Wiederherstellungspunkt schützt dieses Image.",
      "system-protected": "Dieses Image wird vom System benötigt.",
      "tag-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "usage-unavailable": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden."
    },
    "registry": {
      "authentication-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "authentication-required": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "credential-unavailable": "Registry-Zugangsdaten benötigen einen gültigen Verschlüsselungsschlüssel.",
      "exists": "Eine Registry mit dieser Identität ist bereits vorhanden.",
      "inline-credentials-not-allowed": "Gespeicherte Registry-Zugangsdaten oder anonymen Zugriff verwenden.",
      "invalid-config": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "not-found": "Registry nicht gefunden.",
      "persistence-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "rate-limited": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "sync-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "unavailable": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden."
    },
    "update": {
      "check-unavailable": "Die Update-Prüfung konnte nicht abgeschlossen werden.",
      "deploy-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "invalid-target": "Ein gültiges ausdrückliches Update-Ziel auswählen.",
      "persistence-failed": "Der Image- oder Registry-Vorgang konnte nicht abgeschlossen werden.",
      "requires-hub-update": "Vor diesem Agent-Upgrade zuerst den Hub aktualisieren.",
      "system-version-unknown": "Ein kompatibles Release-Ziel konnte nicht bestätigt werden."
    }
  },
  title: "Kein Backend verbunden",
  hint: "Dieser Build ist noch nicht mit einem Hub verbunden. Lokale Darstellungseinstellungen sind verfügbar.",
};
