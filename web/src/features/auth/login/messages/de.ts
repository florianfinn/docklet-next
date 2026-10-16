// German texts of the login demo (full-screen route, started from the design lab).
import type { enLogin } from "./en";

export const deLogin: typeof enLogin = {
  badge: "Demo · keine echte Anmeldung",
  title: "Bei {hub} anmelden",
  exit: "Demo verlassen",
  loading: "Lade Anmeldeart …",
  settingsError: "Die Anmeldeart konnte nicht geladen werden. Du kannst sie unten in der Demo-Steuerung wählen.",
  retry: "Erneut laden",
  proxyRejected: "Melde dich über deinen Proxy an und lade diese Seite neu. Der Hub hat die Proxy-Identität abgelehnt.",
  sessionError: "Die Sitzung konnte nicht geprüft werden.",
  username: "Benutzername",
  password: "Passwort",
  required: "Pflichtfeld",
  submit: "Anmelden",
  submitting: "Melde an …",
  localHint: "Demo: Der Benutzername muss „{user}“ sein. Das Passwort wird weder geprüft noch gespeichert oder gesendet.",
  proxyLead: "Die Anmeldung übernimmt dein Reverse-Proxy, z. B. Authentik. Docklet übernimmt den Benutzer aus dem Header {header}, wenn die Anfrage von {proxies} kommt.",
  proxySubmit: "Über den Proxy fortfahren (simuliert)",
  proxySubmitting: "Prüfe Proxy-Header …",
  errors: {
    invalid: "Benutzername oder Passwort stimmen nicht.",
    unavailable: "Der Hub ist gerade nicht erreichbar. Versuche es gleich noch einmal.",
    locked: "Zu viele Versuche. Die Anmeldung ist für kurze Zeit gesperrt.",
    proxyUnavailable: "Der Proxy hat keinen Benutzer-Header gesendet oder ist nicht als vertrauenswürdig eingetragen.",
  },
  demo: {
    title: "Demo-Steuerung",
    hint: "Nur für die Vorschau: Anmeldeart und Ergebnis des nächsten Versuchs.",
    variant: "Anmeldeart",
    variants: { settings: "Aus Einstellungen", local: "Lokal", proxy: "Proxy", none: "Ohne" },
    outcome: "Nächster Versuch",
    outcomes: { none: "Erfolg", invalid: "Falsche Daten", unavailable: "Nicht erreichbar", locked: "Gesperrt" },
    noneHint: "Ist im Hub „Ohne Anmeldung“ eingestellt, leitet /login direkt zum Dashboard weiter. Lokal und Proxy bleiben über diese Auswahl und das Design-Labor (Abläufe) erreichbar.",
  },
};
