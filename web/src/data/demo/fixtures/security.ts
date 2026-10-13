import type { SecurityOverview } from "@/domain/model";

import { daysAgo, hoursAgo } from "./time";

/** Synthetic findings with invented identifiers; not real advisories. */
export const SECURITY: SecurityOverview = {
  enabled: true,
  schedule: { mode: "daily", time: "03:00" },
  images: [
    { ref: "jellyfin/jellyfin:10.10.3", hostId: "atlas", stackId: "media", usage: "running", counts: { critical: 0, high: 2, medium: 5, low: 9 }, scannedAt: hoursAgo(7) },
    { ref: "redis:7.4-alpine", hostId: "atlas", stackId: "media", usage: "running", counts: { critical: 0, high: 0, medium: 1, low: 2 }, scannedAt: hoursAgo(7) },
    { ref: "lscr.io/linuxserver/duplicati:2.1.0", hostId: "atlas", usage: "running", counts: { critical: 1, high: 1, medium: 3, low: 4 }, scannedAt: hoursAgo(7) },
    { ref: "ghcr.io/paperless-ngx/paperless-ngx:2.18.4", hostId: "atlas", stackId: "paperless", usage: "running", counts: { critical: 0, high: 1, medium: 6, low: 11 }, scannedAt: hoursAgo(7) },
    { ref: "ghcr.io/home-assistant/home-assistant:2026.9.3", hostId: "atlas", stackId: "home", usage: "running", counts: { critical: 0, high: 0, medium: 3, low: 7 }, scannedAt: hoursAgo(7) },
    { ref: "nextcloud:31.0.9-apache", hostId: "boreal", stackId: "cloud", usage: "running", counts: { critical: 0, high: 1, medium: 4, low: 12 }, scannedAt: hoursAgo(7) },
    { ref: "postgres:17-alpine", hostId: "boreal", stackId: "cloud", usage: "running", counts: { critical: 0, high: 0, medium: 1, low: 3 }, scannedAt: hoursAgo(7) },
    { ref: "itzg/minecraft-server:2025.9.0", hostId: "boreal", stackId: "craft", usage: "running", counts: { critical: 0, high: 1, medium: 2, low: 6 }, scannedAt: hoursAgo(7) },
    { ref: "grafana/grafana:12.1.1", hostId: "boreal", stackId: "monitoring", usage: "running", counts: { critical: 0, high: 1, medium: 2, low: 4 }, scannedAt: hoursAgo(7) },
    { ref: "louislam/uptime-kuma:1.23.16", hostId: "boreal", usage: "running", counts: { critical: 1, high: 2, medium: 4, low: 5 }, scannedAt: hoursAgo(7) },
    { ref: "adguard/adguardhome:v0.107.65", hostId: "delta", stackId: "dns", usage: "running", counts: { critical: 0, high: 0, medium: 1, low: 2 }, scannedAt: hoursAgo(7) },
    { ref: "codeberg.org/forgejo/forgejo:12.0.2", hostId: "delta", stackId: "git", usage: "stopped", counts: { critical: 0, high: 1, medium: 2, low: 3 }, scannedAt: hoursAgo(7) },
    { ref: "jellyfin/jellyfin:10.9.11", hostId: "atlas", usage: "unused", counts: { critical: 1, high: 3, medium: 6, low: 9 }, scannedAt: hoursAgo(7) },
  ],
  findings: [
    { id: "CVE-2026-10421", severity: "critical", pkg: "openssl", installed: "3.0.13", fixedIn: "3.0.15", image: "lscr.io/linuxserver/duplicati:2.1.0", hostId: "atlas", cvss: 9.1, title: "Pufferüberlauf bei Zertifikatsprüfung (synthetisch)" },
    { id: "CVE-2026-10388", severity: "critical", pkg: "libxml2", installed: "2.11.5", fixedIn: "2.11.9", image: "jellyfin/jellyfin:10.9.11", hostId: "atlas", cvss: 9.0, title: "Speicherfehler beim Parsen (synthetisch)" },
    { id: "CVE-2026-10302", severity: "critical", pkg: "sqlite3", installed: "5.1.6", fixedIn: "5.1.7", image: "louislam/uptime-kuma:1.23.16", hostId: "boreal", cvss: 8.9, title: "Codeausführung über präparierte Datenbank (synthetisch)" },
    { id: "CVE-2026-09977", severity: "high", pkg: "ffmpeg", installed: "7.0.1", fixedIn: "7.0.2", image: "jellyfin/jellyfin:10.10.3", hostId: "atlas", cvss: 8.1, title: "Lesefehler im Demuxer (synthetisch)" },
    { id: "CVE-2026-09831", severity: "high", pkg: "libcurl", installed: "8.5.0", fixedIn: "8.9.1", image: "nextcloud:31.0.9-apache", hostId: "boreal", cvss: 7.5, title: "Header-Injektion (synthetisch)" },
    { id: "CVE-2026-09720", severity: "high", pkg: "netty-codec", installed: "4.1.100", fixedIn: "4.1.108", image: "itzg/minecraft-server:2025.9.0", hostId: "boreal", cvss: 7.5, title: "Ressourcenerschöpfung (synthetisch)" },
    { id: "CVE-2026-09688", severity: "high", pkg: "golang.org/x/net", installed: "0.24.0", fixedIn: "0.33.0", image: "grafana/grafana:12.1.1", hostId: "boreal", cvss: 7.3, title: "HTTP/2-Rücksetzflut (synthetisch)" },
    { id: "CVE-2026-09601", severity: "high", pkg: "pillow", installed: "10.3.0", fixedIn: "10.4.0", image: "ghcr.io/paperless-ngx/paperless-ngx:2.18.4", hostId: "atlas", cvss: 7.1, title: "Bildverarbeitung außerhalb der Grenzen (synthetisch)" },
    { id: "CVE-2026-09514", severity: "medium", pkg: "busybox", installed: "1.36.1", fixedIn: "1.36.1-r5", image: "redis:7.4-alpine", hostId: "atlas", cvss: 5.5, title: "Fehlerhafte Eingabeprüfung (synthetisch)" },
    { id: "CVE-2026-09402", severity: "medium", pkg: "zlib", installed: "1.3", fixedIn: "1.3.1", image: "postgres:17-alpine", hostId: "boreal", cvss: 5.3, title: "Integerüberlauf (synthetisch)" },
    { id: "CVE-2026-09333", severity: "medium", pkg: "git", installed: "2.45.1", fixedIn: "2.45.3", image: "codeberg.org/forgejo/forgejo:12.0.2", hostId: "delta", cvss: 5.0, title: "Pfadprüfung umgehbar (synthetisch)" },
    { id: "CVE-2026-09233", severity: "low", pkg: "tar", installed: "1.34", image: "nextcloud:31.0.9-apache", hostId: "boreal", cvss: 3.3, title: "Metadatenleck (synthetisch)" },
  ],
  history: [
    { at: daysAgo(9), trigger: "scheduled", counts: { critical: 5, high: 18, medium: 44, low: 82 } },
    { at: daysAgo(8), trigger: "scheduled", counts: { critical: 5, high: 17, medium: 44, low: 81 } },
    { at: daysAgo(7), trigger: "scheduled", counts: { critical: 4, high: 17, medium: 43, low: 80 } },
    { at: daysAgo(6), trigger: "manual", counts: { critical: 4, high: 15, medium: 42, low: 80 } },
    { at: daysAgo(5), trigger: "scheduled", counts: { critical: 4, high: 15, medium: 41, low: 79 } },
    { at: daysAgo(4), trigger: "scheduled", counts: { critical: 3, high: 14, medium: 41, low: 78 } },
    { at: daysAgo(3), trigger: "scheduled", counts: { critical: 3, high: 14, medium: 40, low: 78 } },
    { at: daysAgo(2), trigger: "manual", counts: { critical: 3, high: 13, medium: 40, low: 77 } },
    { at: daysAgo(1), trigger: "scheduled", counts: { critical: 3, high: 13, medium: 40, low: 77 } },
    { at: hoursAgo(7), trigger: "scheduled", counts: { critical: 3, high: 13, medium: 40, low: 77 } },
  ],
};
