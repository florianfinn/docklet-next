// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

export const deRegistries = {
  title: "Registries", description: "Gespeicherte Registry-Konfiguration wird mit kompatiblen Hosts geteilt. Identitäten und Geheimnisse der Zugangsdaten sind nur schreibbar.",
  create: "Registry hinzufügen", edit: "Bearbeiten", remove: "Löschen", test: "Gespeicherte Zugangsdaten testen", save: "Speichern", cancel: "Abbrechen", close: "Schließen",
  empty: "Keine Registries gespeichert", url: "Registry-Adresse", descriptionLabel: "Beschreibung", type: "Registry-Typ", generic: "Generisch", ecr: "Amazon ECR",
  disabled: "Deaktiviert",
  enabled: "Aktiviert", insecure: "Unsichere Registry erlauben", stored: "Zugangsdaten gespeichert", noCredential: "Keine Zugangsdaten gespeichert",
  username: "Neuer Benutzername", valueLabel: "Neues Passwort oder Token", keyIdentityLabel: "Neue Zugangsschlüssel-ID", keyValueLabel: "Neuer geheimer Zugangsschlüssel", region: "AWS-Region", repositories: "Repository-Namen (einer pro Zeile)",
  genericRequired: "Eine neue generische Registry benötigt Benutzername und Passwort oder Token.",
  saved: "Registry {url} gespeichert", deleted: "Registry {url} gelöscht",
  credentialsHint: "Leere Zugangsdatenfelder erhalten die gespeicherten Zugangsdaten. Bestehende Werte werden weder angezeigt noch vorausgefüllt.",
  clear: "Alle gespeicherten Zugangsdaten entfernen", invalid: "Eine gültige Registry-Adresse ohne Zugangsdaten eingeben.",
  editTitle: "Registry bearbeiten", createTitle: "Registry hinzufügen", deleteTitle: "Registry löschen?", deleteBody: "{url} und die gespeicherten Zugangsdaten löschen? Dies ändert auch die mit Hosts synchronisierte Zugangsdaten-Konfiguration.",
  testTitle: "Registry testen", testHint: "Testet die gespeicherten Zugangsdaten vom Hub aus. Erfolg belegt keinen Zugriff auf jedes Repository oder von jedem Host.",
  testSuccess: "Registry erreichbar; gespeicherte Zugangsdaten akzeptiert.", checkedAt: "Geprüft: {time}", unsupported: "Dieser Registry-Typ kann hier nicht bearbeitet werden.",
};
