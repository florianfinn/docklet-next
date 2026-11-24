// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original tailnet route wire vocabulary texts.

import type { enTailnetContract } from "./tailnet-en";

export const deTailnetContract: typeof enTailnetContract = {
  "codes": {
    "credential-rejected": "Die Tailscale-Zugangsdaten wurden abgelehnt. Speichere sie erneut oder starte den Einrichtungstest nach der Korrektur in der Tailscale-Konsole erneut.",
    "coordination-unreachable": "Der Tailscale-Koordinationsserver ist nicht erreichbar. Docklet versucht es weiter; den Internetzugang des Hubs prüfen.",
    "not-connected": "Der Hub ist nicht mit dem Tailnet verbunden. Tailscale unter Einstellungen → Verbindungen einschalten oder warten, bis die Verbindung steht.",
    "key-mint-failed": "Tailscale konnte keinen Schlüssel für diesen Host erstellen. Gleich noch einmal versuchen.",
    "mtls-unsupported": "Der Tailscale-Weg setzt ausgeschaltetes Edge-mTLS voraus (EDGE_MTLS_MODE=disabled).",
    "provider-unavailable": "Die Tailscale-API kann den Zugang gerade nicht prüfen. Docklet versucht es automatisch erneut; bestehende Hosts bleiben verbunden.",
  },
  "connection": {
    "off": "Aus",
    "joining": "Verbindet",
    "connected": "Verbunden",
    "control-unreachable": "Koordination nicht erreichbar",
    "error": "Nicht verbunden",
  },
  "provisioning": {
    "not-configured": "Kein Zugang",
    "not-applicable": "Schlüssel je Host eingefügt",
    "checking": "Zugang wird geprüft",
    "ok": "Zugang akzeptiert",
    "rejected": "Zugang abgelehnt",
    "unavailable": "Zugangsprüfung nicht möglich",
  },
  "peer": {
    "online": "Online",
    "offline": "Offline",
    "unknown": "Noch nicht gesehen",
  },
};
