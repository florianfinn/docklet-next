/*
 * Simulated one-time GitHub/Docker Hub import; no network request is made.
 * Adapted from the Opus Arcane mockup compose.ts simulateImport (copied 2026-10-10).
 */
import type { ImportRequest, ImportResult } from "../adapter";

export class ImportInputError extends Error {
  readonly kind: ImportRequest["kind"];
  constructor(kind: ImportRequest["kind"]) {
    super("import-ref-missing");
    this.kind = kind;
    this.name = "ImportInputError";
  }
}

export function simulateImport({ kind, ref }: ImportRequest): ImportResult {
  const r = ref.trim().replace(/^https?:\/\//, "").replace(/\/+$/, "");
  if (!r) throw new ImportInputError(kind);
  if (kind === "github") {
    const repo = r.replace(/^github\.com\//, "");
    if (repo === "example/notes-app") {
      return {
        title: `${repo} · deploy/compose.yaml`,
        simulated: true,
        found: ["Compose-Datei unter deploy/compose.yaml", "2 Services: app, db", "Port 3000 veröffentlicht"],
        missing: ["DATABASE_URL hat keinen Wert", "POSTGRES_PASSWORD hat keinen Wert"],
        services: [
          {
            key: "app",
            body: {
              image: "ghcr.io/example/notes-app:1.4.0",
              restart: "unless-stopped",
              ports: ["3000:3000"],
              environment: { APP_URL: "https://notes.example.com", DATABASE_URL: "" },
              depends_on: ["db"],
            },
          },
          {
            key: "db",
            body: {
              image: "postgres:17-alpine",
              restart: "unless-stopped",
              volumes: ["notes_db:/var/lib/postgresql/data"],
              environment: { POSTGRES_PASSWORD: "" },
            },
          },
        ],
      };
    }
    const short = repo.split("/").pop() ?? "app";
    return {
      title: `${repo} · keine Compose-Datei`,
      simulated: true,
      found: ["README gefunden", "Dockerfile gefunden – eigenständige Builds sind nicht Teil von Docklet"],
      missing: ["Image fehlt – bitte ein veröffentlichtes Image angeben", "Ports unbekannt"],
      services: [{ key: short, body: { image: "", restart: "unless-stopped" } }],
    };
  }
  const image = r.replace(/^hub\.docker\.com\/(r\/)?/, "");
  if (image.startsWith("traefik/whoami")) {
    return {
      title: "traefik/whoami · Docker Hub",
      simulated: true,
      found: ["Neuester Tag: v1.11", "EXPOSE 80/tcp"],
      missing: ["Host-Port für 80/tcp festlegen"],
      services: [{ key: "whoami", body: { image: "traefik/whoami:v1.11", restart: "unless-stopped", ports: ["80"] } }],
    };
  }
  if (image.includes("syncthing")) {
    return {
      title: "linuxserver/syncthing · Docker Hub",
      simulated: true,
      found: ["Tag: 1.30.0", "EXPOSE 8384, 22000/tcp, 22000/udp", "VOLUME /config"],
      missing: ["Quelle für /config wählen", "PUID/PGID passend zu Dateirechten setzen"],
      services: [
        {
          key: "syncthing",
          body: {
            image: "lscr.io/linuxserver/syncthing:1.30.0",
            restart: "unless-stopped",
            ports: ["8384:8384", "22000:22000"],
            volumes: ["/config"],
            environment: { PUID: "", PGID: "", TZ: "Europe/Berlin" },
          },
        },
      ],
    };
  }
  const base = (image.split("/").pop() ?? "app").split(":")[0] ?? "app";
  const withTag = image.includes(":") ? image : `${image}:latest`;
  return {
    title: `${image} · Docker Hub`,
    simulated: true,
    found: [`Image-Referenz: ${withTag}`],
    missing: ["Keine Metadaten zu Ports oder Volumes vorhanden"],
    services: [{ key: base, body: { image: withTag, restart: "unless-stopped" } }],
  };
}
