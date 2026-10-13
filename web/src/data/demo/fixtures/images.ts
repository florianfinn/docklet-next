import { daysAgo } from "./time";

/**
 * Images that no container references: earlier versions left after updates,
 * build bases and dangling images whose tag moved to a newer pull. Images in
 * use are derived from the containers, so they are not listed here.
 */
export interface ImageSeed {
  hostId: string;
  repository: string;
  /** null = dangling (`<none>`); the repository is the last one known from its digest. */
  tag: string | null;
  version?: string;
  createdDays: number;
  sizeMb?: number;
}

export const EXTRA_IMAGES: ImageSeed[] = [
  { hostId: "atlas", repository: "jellyfin/jellyfin", tag: "10.9.11", version: "10.9.11", createdDays: 96, sizeMb: 1380 },
  { hostId: "atlas", repository: "ghcr.io/home-assistant/home-assistant", tag: "2026.8.4", version: "2026.8.4", createdDays: 44, sizeMb: 1910 },
  { hostId: "atlas", repository: "redis", tag: null, createdDays: 31, sizeMb: 41 },
  { hostId: "atlas", repository: "alpine", tag: "3.20", version: "3.20", createdDays: 120, sizeMb: 8 },
  { hostId: "boreal", repository: "nextcloud", tag: "31.0.8-apache", version: "31.0.8", createdDays: 50, sizeMb: 1240 },
  { hostId: "boreal", repository: "grafana/grafana", tag: null, createdDays: 27, sizeMb: 655 },
  { hostId: "boreal", repository: "itzg/minecraft-server", tag: "2025.8.1", version: "2025.8.1", createdDays: 60, sizeMb: 720 },
  { hostId: "cedar", repository: "node", tag: "22-alpine", version: "22", createdDays: 70, sizeMb: 160 },
  { hostId: "delta", repository: "adguard/adguardhome", tag: "v0.107.63", version: "0.107.63", createdDays: 58, sizeMb: 74 },
];

/** Rough synthetic sizes per repository, so lists look plausible; unknown ones get a hashed size. */
export const IMAGE_SIZES_MB: Record<string, number> = {
  "jellyfin/jellyfin": 1420,
  "ghcr.io/home-assistant/home-assistant": 1960,
  "eclipse-mosquitto": 13,
  redis: 41,
  postgres: 276,
  "ghcr.io/paperless-ngx/paperless-ngx": 1290,
  nextcloud: 1270,
  "itzg/minecraft-server": 735,
  "grafana/grafana": 660,
  "prom/prometheus": 290,
  "prom/node-exporter": 24,
  "adguard/adguardhome": 76,
  "mvance/unbound-rpi": 38,
  "codeberg.org/forgejo/forgejo": 205,
  "ghcr.io/requarks/wiki": 470,
  "lscr.io/linuxserver/duplicati": 610,
  "binhex/arch-krusader": 1120,
  "netdata/netdata": 540,
  "louislam/uptime-kuma": 410,
  "traefik/whoami": 7,
};

export const createdAt = (days: number) => daysAgo(days);
