import type { ImageVersion, Service, Stack, UpdateInfo, UpdateLevel } from "@/domain/model";
import { levelFromVersions } from "@/domain/updates";

import { daysAgo, hoursAgo, minutesAgo } from "./time";

const DOCKER_HUB = "Docker Hub · anonym";
const GHCR = "GitHub Container Registry · anonym";

export function img(repository: string, tag: string, version?: string, publishedDays = 20): ImageVersion {
  return { repository, tag, version, published: daysAgo(publishedDays) };
}

function current(image: ImageVersion, registry = DOCKER_HUB): UpdateInfo {
  return { state: "current", current: image, checkedAt: hoursAgo(1.5), registry };
}

function version(image: ImageVersion, tag: string, latestVersion: string, registry = DOCKER_HUB, level?: UpdateLevel): UpdateInfo {
  return {
    state: "version",
    current: image,
    latest: { repository: image.repository, tag, version: latestVersion, published: daysAgo(3) },
    level: level ?? levelFromVersions(image.version ?? image.tag, latestVersion),
    checkedAt: hoursAgo(1.5),
    registry,
  };
}

function rebuild(image: ImageVersion, latestVersion?: string, registry = DOCKER_HUB): UpdateInfo {
  return {
    state: "rebuild",
    current: image,
    latest: { repository: image.repository, tag: image.tag, version: latestVersion, published: daysAgo(2) },
    checkedAt: hoursAgo(1.5),
    registry,
  };
}

interface SvcSeed extends Partial<Service> {
  name: string;
  image: ImageVersion;
}

function svc(seed: SvcSeed): Service {
  return {
    state: "running",
    health: "none",
    ports: [],
    networks: [],
    restartPolicy: "unless-stopped",
    memLimitMb: 1024,
    startedAt: daysAgo(4),
    restarts24h: 0,
    update: current(seed.image),
    ...seed,
  };
}

const jellyfin = img("jellyfin/jellyfin", "10.10.3", "10.10.3", 34);
const redisMedia = img("redis", "7.4-alpine", "7.4.2", 41);
const paperless = img("ghcr.io/paperless-ngx/paperless-ngx", "2.18.4", "2.18.4", 12);
const pg17 = img("postgres", "17-alpine", "17.6", 18);
const ha = img("ghcr.io/home-assistant/home-assistant", "2026.9.3", "2026.9.3", 16);
const mosquitto = img("eclipse-mosquitto", "2.0.22", "2.0.22", 60);
const nextcloud = img("nextcloud", "31.0.9-apache", "31.0.9", 25);
const minecraft = img("itzg/minecraft-server", "2025.9.0", "2025.9.0", 30);
const prometheus = img("prom/prometheus", "latest", "3.5.0", 21);
const grafana = img("grafana/grafana", "12.1.1", "12.1.1", 19);
const nodeExporter = img("prom/node-exporter", "v1.9.1", "1.9.1", 90);
const adguard = img("adguard/adguardhome", "v0.107.65", "0.107.65", 14);
const unbound = img("mvance/unbound-rpi", "1.22.0", "1.22.0", 70);
const forgejo = img("codeberg.org/forgejo/forgejo", "12.0.2", "12.0.2", 22);
const wikijs = img("ghcr.io/requarks/wiki", "2.5.308", "2.5.308", 120);
const pg16 = img("postgres", "16-alpine", "16.10", 45);

export const STACKS: Stack[] = [
  {
    id: "media",
    name: "Media",
    hostId: "atlas",
    description: "Jellyfin-Mediathek mit Redis-Cache",
    labels: ["media", "produktiv"],
    url: "https://media.example.com",
    autoUpdate: { mode: "check" },
    lastDeployAt: daysAgo(6),
    services: [
      svc({
        name: "jellyfin",
        image: jellyfin,
        health: "healthy",
        ports: [{ host: "8096", container: "8096" }],
        networks: ["media_default"],
        memLimitMb: 4096,
        update: version(jellyfin, "10.11.1", "10.11.1"),
      }),
      svc({ name: "redis", image: redisMedia, networks: ["media_default"], memLimitMb: 256, update: rebuild(redisMedia, "7.4.3") }),
    ],
  },
  {
    id: "paperless",
    name: "Paperless",
    hostId: "atlas",
    description: "Dokumentenarchiv mit OCR",
    labels: ["dokumente"],
    url: "https://docs.example.com",
    autoUpdate: { mode: "scheduled", schedule: { mode: "weekly", day: "wed", time: "03:30" } },
    lastDeployAt: daysAgo(12),
    services: [
      svc({
        name: "webserver",
        image: paperless,
        health: "healthy",
        ports: [{ host: "8010", container: "8000" }],
        networks: ["paperless_default"],
        memLimitMb: 2048,
        update: current(paperless, GHCR),
      }),
      svc({ name: "db", image: pg17, health: "healthy", networks: ["paperless_default"], memLimitMb: 1024 }),
      svc({ name: "broker", image: redisMedia, networks: ["paperless_default"], memLimitMb: 256, update: rebuild(redisMedia, "7.4.3") }),
    ],
  },
  {
    id: "home",
    name: "Home",
    hostId: "atlas",
    description: "Home Assistant mit MQTT",
    labels: ["smart-home"],
    url: "https://home.example.com",
    autoUpdate: { mode: "check" },
    lastDeployAt: daysAgo(16),
    services: [
      svc({
        name: "homeassistant",
        image: ha,
        health: "healthy",
        networks: ["host"],
        memLimitMb: 2048,
        restartPolicy: "unless-stopped",
        update: version(ha, "2026.10.1", "2026.10.1", GHCR, "minor"),
      }),
      svc({ name: "mosquitto", image: mosquitto, ports: [{ host: "1883", container: "1883" }], networks: ["home_default"], memLimitMb: 128 }),
    ],
  },
  {
    id: "cloud",
    name: "Cloud",
    hostId: "boreal",
    description: "Nextcloud mit PostgreSQL und Cache",
    labels: ["produktiv", "intern"],
    url: "https://cloud.example.com",
    autoUpdate: { mode: "scheduled", schedule: { mode: "weekly", day: "sun", time: "04:00" } },
    lastDeployAt: daysAgo(2),
    services: [
      svc({
        name: "nextcloud",
        image: nextcloud,
        health: "healthy",
        ports: [{ host: "8443", container: "80" }],
        networks: ["cloud_web", "cloud_internal"],
        memLimitMb: 2048,
        update: version(nextcloud, "32.0.1-apache", "32.0.1"),
      }),
      svc({ name: "postgres", image: pg17, health: "healthy", networks: ["cloud_internal"], memLimitMb: 2048 }),
      svc({ name: "cache", image: redisMedia, networks: ["cloud_internal"], memLimitMb: 256, update: rebuild(redisMedia) }),
    ],
  },
  {
    id: "craft",
    name: "Craft",
    hostId: "boreal",
    description: "Minecraft-Server für die Familie",
    labels: ["spiele"],
    url: "mc.example.com:25565",
    autoUpdate: { mode: "check" },
    lastDeployAt: hoursAgo(20),
    services: [
      svc({
        name: "minecraft",
        image: minecraft,
        health: "unhealthy",
        ports: [{ host: "25565", container: "25565" }],
        networks: ["craft_default"],
        memLimitMb: 6144,
        restarts24h: 3,
        startedAt: minutesAgo(38),
      }),
    ],
  },
  {
    id: "monitoring",
    name: "Monitoring",
    hostId: "boreal",
    description: "Prometheus, Grafana und Node-Exporter",
    labels: ["intern", "monitoring"],
    url: "https://grafana.example.com",
    autoUpdate: { mode: "check" },
    lastDeployAt: daysAgo(9),
    services: [
      svc({ name: "prometheus", image: prometheus, ports: [{ host: "9090", container: "9090" }], networks: ["monitoring_default"], memLimitMb: 2048, update: rebuild(prometheus) }),
      svc({
        name: "grafana",
        image: grafana,
        health: "healthy",
        ports: [{ host: "3000", container: "3000" }],
        networks: ["monitoring_default"],
        memLimitMb: 1024,
        update: version(grafana, "12.1.2", "12.1.2"),
      }),
      svc({ name: "node-exporter", image: nodeExporter, networks: ["host"], memLimitMb: 128 }),
    ],
  },
  {
    id: "dns",
    name: "DNS",
    hostId: "delta",
    description: "AdGuard Home mit Unbound",
    labels: ["netz"],
    autoUpdate: { mode: "check" },
    lastDeployAt: daysAgo(21),
    services: [
      svc({
        name: "adguard",
        image: adguard,
        health: "healthy",
        ports: [
          { host: "53", container: "53", protocol: "udp" },
          { host: "3080", container: "80" },
        ],
        networks: ["dns_default"],
        memLimitMb: 512,
      }),
      svc({ name: "unbound", image: unbound, networks: ["dns_default"], memLimitMb: 256 }),
    ],
  },
  {
    id: "git",
    name: "Git",
    hostId: "delta",
    labels: [],
    autoUpdate: { mode: "check" },
    lastDeployAt: daysAgo(40),
    services: [
      svc({
        name: "forgejo",
        image: forgejo,
        state: "stopped",
        ports: [
          { host: "3001", container: "3000" },
          { host: "2222", container: "22" },
        ],
        networks: ["git_default"],
        memLimitMb: 1024,
        startedAt: undefined,
      }),
    ],
  },
  {
    id: "wiki",
    name: "Wiki",
    hostId: "cedar",
    description: "Wiki.js für Notizen",
    labels: ["intern"],
    autoUpdate: { mode: "check" },
    lastDeployAt: daysAgo(30),
    services: [
      svc({
        name: "wikijs",
        image: wikijs,
        state: "unknown",
        ports: [{ host: "3002", container: "3000" }],
        networks: ["wiki_default"],
        update: { state: "error", current: wikijs, message: "Host Cedar ist nicht verbunden.", checkedAt: hoursAgo(2.2) },
      }),
      svc({
        name: "db",
        image: pg16,
        state: "unknown",
        networks: ["wiki_default"],
        update: { state: "error", current: pg16, message: "Host Cedar ist nicht verbunden.", checkedAt: hoursAgo(2.2) },
      }),
    ],
  },
];

/** Simulation baselines per container id (CPU %, memory MB). */
export const BASELINES: Record<string, { cpu: number; mem: number }> = {
  "c-media-jellyfin": { cpu: 14, mem: 1450 },
  "c-media-redis": { cpu: 1, mem: 48 },
  "c-paperless-webserver": { cpu: 6, mem: 820 },
  "c-paperless-db": { cpu: 3, mem: 260 },
  "c-paperless-broker": { cpu: 1, mem: 32 },
  "c-home-homeassistant": { cpu: 9, mem: 690 },
  "c-home-mosquitto": { cpu: 1, mem: 18 },
  "c-cloud-nextcloud": { cpu: 11, mem: 860 },
  "c-cloud-postgres": { cpu: 4, mem: 420 },
  "c-cloud-cache": { cpu: 1, mem: 64 },
  "c-craft-minecraft": { cpu: 38, mem: 5400 },
  "c-monitoring-prometheus": { cpu: 7, mem: 980 },
  "c-monitoring-grafana": { cpu: 3, mem: 210 },
  "c-monitoring-node-exporter": { cpu: 1, mem: 22 },
  "c-dns-adguard": { cpu: 4, mem: 120 },
  "c-dns-unbound": { cpu: 2, mem: 60 },
  "c-duplicati": { cpu: 2, mem: 210 },
  "c-krusader": { cpu: 1, mem: 140 },
  "c-netdata": { cpu: 5, mem: 310 },
  "c-uptime-kuma": { cpu: 2, mem: 150 },
};
