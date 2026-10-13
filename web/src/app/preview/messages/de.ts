// German texts of the honest preview pages for sections outside phase one.
import type { enPreview } from "./en";

export const dePreview: typeof enPreview = {
  badge: "Später · Vorschau",
  lead: "Dieser Bereich ist noch nicht Teil der ersten Prototyp-Phase. Geplanter Umfang laut Produktentscheidungen:",
  noFake: "Hier gibt es bewusst keine Schaltflächen ohne Wirkung.",
  back: "Zum Dashboard",
  sections: {
    proxy: {
      title: "Proxy",
      points: ["Traefik/Caddy-Zielbild mit Routen, Zielen, Middleware und Zertifikaten", "Vorhandene Installation berücksichtigen", "Projektbasis noch in Evaluation"],
    },
    games: {
      title: "Games",
      points: ["Spielkonsole über Docker Attach/RCON", "Status und Spielerzahl", "Start/Stop, Updates, Dateien, Backups über die allgemeine Verwaltung"],
    },
  },
};
