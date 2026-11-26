// Source: docklet-next/issues/640
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original guided tour texts (German).

// German texts of the guided tour offered after first-run setup.
import type { enTour } from "./en";

export const deTour: typeof enTour = {
  offer: {
    title: "Einrichtung abgeschlossen",
    text: "Kurze Tour durch die wichtigsten Seiten? Sie dauert etwa eine Minute und ändert nichts.",
    start: "Tour starten",
    later: "Nicht jetzt",
    hint: "Die Tour lässt sich jederzeit unter Einstellungen › Erklärungen starten.",
  },
  progress: "Schritt {current} von {total}",
  back: "Zurück",
  next: "Weiter",
  finish: "Fertig",
  skip: "Tour beenden",
  openNav: "Öffne „{page}“ in der Navigation links oder wähle Weiter.",
  openMenu: "Öffne oben links das Menü und dann „{page}“ oder wähle Weiter.",
  label: "Geführte Tour",
  steps: {
    dashboard: {
      title: "Dashboard",
      text: "Die Lage aller Hosts auf einen Blick: Der Ring zeigt Stacks und einzelne Container der gewählten Hosts nach Zustand, die Tafel daneben nennt zuerst, was Aufmerksamkeit braucht. Über Anpassen ordnest du die Widgets.",
    },
    hostFilter: {
      title: "Hostfilter",
      text: "Wähle, welche Hosts die Seiten zeigen. Der Filter gilt für Dashboard, Stacks, Container und Ressourcen gleichermaßen.",
    },
    hosts: {
      title: "Hosts",
      text: "Hier verbindest du weitere Docker-Hosts, direkt oder per Edge für Hosts hinter NAT. Jeder Host zeigt Status, Agent-Version und Wartung.",
    },
    stacks: {
      title: "Stacks",
      text: "Compose-Projekte: Stack anlegen, Compose-Datei und Umgebung bearbeiten, ausrollen und Images aktuell halten.",
    },
    containers: {
      title: "Container",
      text: "Alle Container der gewählten Hosts mit Status und Last. Ein Container öffnet Logs, Terminal, Dateien und Einstellungen.",
    },
    security: {
      title: "Sicherheit",
      text: "Ergebnisse der Image-Scans nach Schweregrad. Jeder Befund zeigt die betroffenen Container und ob es eine behobene Version gibt.",
    },
    backups: {
      title: "Backups",
      text: "Volumes nach Zeitplan sichern und wiederherstellen. Die Hub-Datenbank hat eigene Sicherungen unter Hub-Datenbank.",
    },
    notifications: {
      title: "Benachrichtigungen",
      text: "Lass dich über Ausfälle, Updates und Befunde informieren, etwa per E-Mail, ntfy, Telegram oder Discord.",
    },
    search: {
      title: "Suche und Aktivität",
      text: "Die Suche findet Seiten, Ressourcen und Schnellaktionen (Strg+K). Aktivitäten zeigt laufende und beendete Aktionen. Die Tour startest du jederzeit neu unter Einstellungen › Erklärungen.",
    },
  },
};
