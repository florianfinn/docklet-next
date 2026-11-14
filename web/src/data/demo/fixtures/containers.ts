// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/data/demo/fixtures/containers.ts
// Revision: 18638c941fcdf97383db376c1611362873a9d1d8358b727e9b43d58c413659d3
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: None; byte-identical L4 snapshot. See web/PROVENANCE.md. M26 #462: bounded shell counts and raw inventory ownership.

import type { ContainerInfo } from "@/domain/model";

import { img } from "./stacks";
import { daysAgo, hoursAgo } from "./time";

const duplicati = img("lscr.io/linuxserver/duplicati", "2.1.0", "2.1.0.5", 40);
const krusader = img("binhex/arch-krusader", "latest", "2.8.1", 50);
const netdata = img("netdata/netdata", "v2.6.3", "2.6.3", 12);
const kuma = img("louislam/uptime-kuma", "1.23.16", "1.23.16", 80);
const whoami = img("traefik/whoami", "v1.11", "1.11.0", 120);

/** Containers outside Compose stacks: Unraid templates, plain docker run, unclear origin. */
export const STANDALONE: (ContainerInfo & { bindSources: string[]; volumeNames?: string[] })[] = [
  {
    id: "c-duplicati",
    name: "duplicati",
    hostId: "atlas",
    origin: "unraid",
    originNote: "Unraid-Vorlage my-duplicati.xml erkannt",
    image: duplicati,
    state: "running",
    health: "none",
    ports: [{ host: "8200", container: "8200" }],
    networks: ["bridge"],
    mounts: ["/mnt/user/appdata/duplicati:/config", "/mnt/user/backups:/backups"],
    bindSources: ["/mnt/user/appdata/duplicati", "/mnt/user/backups"],
    env: { PUID: "99", PGID: "100", TZ: "Europe/Berlin" },
    restartPolicy: "unless-stopped",
    memLimitMb: 1024,
    startedAt: daysAgo(9),
    update: { state: "current", current: duplicati, checkedAt: hoursAgo(1.5), registry: "LinuxServer Registry · anonym" },
  },
  {
    id: "c-krusader",
    name: "krusader",
    hostId: "atlas",
    origin: "unraid",
    originNote: "Unraid-Vorlage binhex-krusader.xml erkannt",
    image: krusader,
    state: "running",
    health: "none",
    ports: [{ host: "6080", container: "6080" }],
    networks: ["bridge"],
    mounts: ["/mnt/user:/media"],
    bindSources: ["/mnt/user"],
    env: { PUID: "99", PGID: "100" },
    restartPolicy: "unless-stopped",
    memLimitMb: 512,
    startedAt: daysAgo(9),
    update: {
      state: "rebuild",
      current: krusader,
      latest: { repository: krusader.repository, tag: "latest", published: daysAgo(1) },
      checkedAt: hoursAgo(1.5),
      registry: "Docker Hub · anonym",
    },
  },
  {
    id: "c-netdata",
    name: "netdata",
    hostId: "atlas",
    origin: "unknown",
    originNote: "Weder Compose-Labels noch eine Unraid-Vorlage gefunden",
    image: netdata,
    state: "running",
    health: "healthy",
    ports: [{ host: "19999", container: "19999" }],
    networks: ["bridge"],
    mounts: ["netdata_config:/etc/netdata"],
    bindSources: [], volumeNames: ["netdata_config"],
    env: {},
    restartPolicy: "always",
    memLimitMb: 512,
    startedAt: daysAgo(3),
    update: { state: "current", current: netdata, checkedAt: hoursAgo(1.5), registry: "Docker Hub · anonym" },
  },
  {
    id: "c-uptime-kuma",
    name: "uptime-kuma",
    hostId: "boreal",
    origin: "standalone",
    originNote: "Mit docker run erstellt",
    image: kuma,
    state: "running",
    health: "healthy",
    ports: [{ host: "3005", container: "3001" }],
    networks: ["bridge"],
    mounts: ["kuma_data:/app/data"],
    bindSources: [], volumeNames: ["kuma_data"],
    env: { TZ: "Europe/Berlin" },
    restartPolicy: "unless-stopped",
    memLimitMb: 512,
    startedAt: daysAgo(14),
    update: {
      state: "version",
      current: kuma,
      latest: { repository: kuma.repository, tag: "2.0.2", version: "2.0.2", published: daysAgo(5) },
      level: "major",
      checkedAt: hoursAgo(1.5),
      registry: "Docker Hub · anonym",
    },
  },
  {
    id: "c-whoami-test",
    name: "whoami-test",
    hostId: "boreal",
    origin: "standalone",
    originNote: "Mit docker run erstellt",
    image: whoami,
    state: "stopped",
    health: "none",
    ports: [{ host: "8082", container: "80" }],
    networks: ["bridge"],
    mounts: [], bindSources: [],
    env: { WHOAMI_NAME: "test" },
    restartPolicy: "no",
    memLimitMb: 64,
    update: { state: "current", current: whoami, checkedAt: hoursAgo(1.5), registry: "Docker Hub · anonym" },
  },
];
