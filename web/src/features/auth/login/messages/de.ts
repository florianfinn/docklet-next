// German texts for connected sign-in and its demo preview.
import type { enLogin } from "./en";

export const deLogin: typeof enLogin = {
  badge: (import.meta.env.DEV ? "Demo · keine echte Anmeldung" : "Anmeldung"),
  title: "Bei {hub} anmelden",
  exit: (import.meta.env.DEV ? "Demo verlassen" : "Zurück zum Hub"),
  loading: "Lade Anmeldeart …",
  settingsError: "Die Anmeldeart konnte nicht geladen werden. Bitte erneut versuchen.",
  retry: "Erneut laden",
  proxyRejected: "Melde dich über deinen Proxy an und lade diese Seite neu. Der Hub hat die Proxy-Identität abgelehnt.",
  sessionError: "Die Sitzung konnte nicht geprüft werden.",
  setupStatusError: "Der Status der Ersteinrichtung konnte nicht geprüft werden.",
  username: "Benutzername",
  password: "Passwort",
  required: "Pflichtfeld",
  submit: "Anmelden",
  submitting: "Melde an …",
  localHint: (import.meta.env.DEV ? "Demo: Der Benutzername muss „{user}“ sein. Das Passwort wird weder geprüft noch gespeichert oder gesendet." : "Zugangsdaten für „{user}“ eingeben."),
  proxyLead: "Die Anmeldung übernimmt dein Reverse-Proxy, z. B. Authentik. Docklet übernimmt den Benutzer aus dem Header {header}, wenn die Anfrage von {proxies} kommt.",
  proxySubmit: (import.meta.env.DEV ? "Über den Proxy fortfahren (simuliert)" : "Über den Proxy fortfahren"),
  proxySubmitting: "Prüfe Proxy-Header …",
  errors: {
    invalid: "Benutzername oder Passwort stimmen nicht.",
    unavailable: "Der Hub ist gerade nicht erreichbar. Versuche es gleich noch einmal.",
    locked: "Zu viele Versuche. Die Anmeldung ist für kurze Zeit gesperrt.",
    proxyUnavailable: "Der Proxy hat keinen Benutzer-Header gesendet oder ist nicht als vertrauenswürdig eingetragen.",
  },
  demo: {
    title: (import.meta.env.DEV ? "Demo-Steuerung" : "Anmeldeinformationen"),
    hint: (import.meta.env.DEV ? "Nur für die Vorschau: Anmeldeart und Ergebnis des nächsten Versuchs." : "Der Betreiber legt die Anmeldeart beim Start fest."),
    variant: "Anmeldeart",
    variants: { settings: "Aus Einstellungen", local: "Lokal", proxy: "Proxy", none: "Ohne" },
    outcome: "Nächster Versuch",
    outcomes: { none: "Erfolg", invalid: "Falsche Daten", unavailable: "Nicht erreichbar", locked: "Gesperrt" },
    noneHint: (import.meta.env.DEV ? "Ist im Hub „Ohne Anmeldung“ eingestellt, leitet /login direkt zum Dashboard weiter. Lokal und Proxy bleiben über diese Auswahl und das Design-Labor (Abläufe) erreichbar." : "Ohne Anmeldung führt /login direkt zum Dashboard."),
  },
};
