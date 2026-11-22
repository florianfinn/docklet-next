// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import type { components } from "@/api/generated/schema";
import type { Host } from "./model";

export type TailnetStatus = components["schemas"]["TailnetStatus"];
export type TailnetSettings = components["schemas"]["TailnetSettings"];
export type TailnetUpdate = components["schemas"]["TailnetSettingsUpdate"];
export type TailnetPeer = components["schemas"]["TailnetHostPeer"];
export type NetworkRoute = "lan" | "domain" | "tailnet";
export type ChainLink = "hub" | "peer" | "tunnel";
export type ChainState = "ok" | "failed" | "unknown";

export interface TailnetApi {
  current: () => TailnetStatus | undefined;
  subscribe: (listener: () => void) => () => void;
  status: () => Promise<TailnetStatus | undefined>;
  settings: () => Promise<TailnetSettings>;
  save: (input: TailnetUpdate) => Promise<TailnetSettings>;
  reset: () => Promise<void>;
}

/** Admission is independent of the existing node's connection and honours the server's mTLS refusal. */
export function tailnetRouteReason(status: TailnetStatus | undefined): "mtls" | "rejected" | "checking" | "disconnected" | undefined {
  if (status?.routeCode === "tailnet.mtls-unsupported") return "mtls";
  if (status?.connection.state !== "connected") return "disconnected";
  if (status.provisioning.state === "rejected") return "rejected";
  if (!status.routeAvailable) return "checking";
  return undefined;
}

/** Hosts whose tunnel failed in backend tailnet.FailingLink (`error` is mapped to offline); pending and update-required are not. */
export function tailnetHostDown(host: Host): boolean {
  return host.status === "offline" || host.status === "token-rejected";
}

/** Mirrors backend tailnet.FailingLink: an unobserved peer is no failure; later links after a failure are not checkable. */
export function tailnetChain(host: Host, status?: TailnetStatus): { link: ChainLink; state: ChainState }[] {
  const links: ChainLink[] = ["hub", "peer", "tunnel"];
  const reported = host.tailnet?.failingLink;
  const first = status ? (status.connection.state !== "connected" ? "hub" : host.tailnet?.state === "offline" ? "peer" : tailnetHostDown(host) ? "tunnel" : "none") : reported;
  const failed = links.indexOf(first as ChainLink);
  return links.map((link, index) => ({ link, state: first === undefined ? "unknown" : failed === -1 || index < failed ? "ok" : index === failed ? "failed" : "unknown" }));
}
