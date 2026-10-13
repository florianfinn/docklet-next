// Synthetic content of the service widgets. No request leaves the browser;
// values are generated from the demo clock so they look alive but mean nothing.

import { createStore } from "@/shared/lib/store";

import type { ServiceKind } from "../dashboard-layout";

/** Services with a simulated connect in flight; their widgets show skeletons. */
export const connectingStore = createStore<ServiceKind[]>([]);

export interface ServiceMeta {
  kind: ServiceKind;
  /** Brand-like colour for tints and accents. */
  color: string;
  exampleAddress: string;
  /** Matching demo stack, when there is one. */
  stackId?: string;
}

export const SERVICE_META: Record<ServiceKind, ServiceMeta> = {
  plex: { kind: "plex", color: "#e5a00d", exampleAddress: "http://192.0.2.21:32400" },
  minecraft: { kind: "minecraft", color: "#6aa84f", exampleAddress: "192.0.2.22:25565", stackId: "craft" },
  overseerr: { kind: "overseerr", color: "#8b5cf6", exampleAddress: "http://192.0.2.23:5055" },
};

export interface PlexStream {
  id: string;
  user: string;
  title: string;
  subtitle: string;
  quality: string;
  mode: "direct" | "transcode";
  device: string;
  durationMin: number;
  startOffsetMin: number;
}

export const PLEX_STREAMS: PlexStream[] = [
  { id: "s1", user: "Mara", title: "Nordlicht", subtitle: "Staffel 2 · Folge 4", quality: "1080p", mode: "direct", device: "Wohnzimmer-TV", durationMin: 48, startOffsetMin: 21 },
  { id: "s2", user: "Jonas", title: "Die Brücke am Fluss", subtitle: "Film · 2019", quality: "4K → 1080p", mode: "transcode", device: "Tablet", durationMin: 112, startOffsetMin: 64 },
  { id: "s3", user: "Ida", title: "Klangwelten", subtitle: "Album · Titel 7", quality: "FLAC", mode: "direct", device: "Küche", durationMin: 5, startOffsetMin: 2 },
];

/** Progress of a stream at a point in time, looping so the demo never ends. */
export function streamProgress(s: PlexStream, now: number): number {
  const minutes = (now / 60_000 + s.startOffsetMin) % s.durationMin;
  return Math.round((minutes / s.durationMin) * 100);
}

export interface MinecraftStatus {
  version: string;
  motd: string;
  max: number;
  players: { name: string; minutes: number }[];
  tps: number;
}

export const MINECRAFT: MinecraftStatus = {
  version: "Java 1.21.8",
  motd: "Werkstatt-Welt (Demo)",
  max: 20,
  tps: 19.8,
  players: [
    { name: "Erzsucherin", minutes: 142 },
    { name: "Blockbauer42", minutes: 87 },
    { name: "Nachteule", minutes: 31 },
    { name: "Kiesel_7", minutes: 6 },
  ],
};

export interface MediaRequest {
  id: string;
  title: string;
  kind: "movie" | "series";
  user: string;
  ageHours: number;
  status: "pending" | "approved" | "declined" | "available";
}

export const OVERSEERR_REQUESTS: MediaRequest[] = [
  { id: "r1", title: "Sternenfahrer", kind: "movie", user: "Jonas", ageHours: 2, status: "pending" },
  { id: "r2", title: "Küstenwache", kind: "series", user: "Mara", ageHours: 9, status: "pending" },
  { id: "r3", title: "Das stille Tal", kind: "movie", user: "Ida", ageHours: 26, status: "pending" },
  { id: "r4", title: "Uhrwerk", kind: "series", user: "Mara", ageHours: 50, status: "approved" },
  { id: "r5", title: "Morgenrot", kind: "movie", user: "Jonas", ageHours: 74, status: "available" },
];

/** Simulated connect: an empty address is missing config, the error toggle or "fail" in the address fails. */
export function simulateConnect(address: string, simulateError: boolean): "unconfigured" | "connected" | "error" {
  if (!address.trim()) return "unconfigured";
  if (simulateError || /fail|fehler/i.test(address)) return "error";
  return "connected";
}
