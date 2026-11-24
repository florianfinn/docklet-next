// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers. M37 #586: policy parts for the form alternative.

import type { components } from "@/api/generated/schema";
import type { Host } from "./model";

export type TailnetStatus = components["schemas"]["TailnetStatus"];
export type TestResult = components["schemas"]["TailnetTestResult"];
export type TestCheck = components["schemas"]["TailnetTestCheck"];
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
  test: () => Promise<TestResult>;
  /** Lifecycle counter; ordinary status observations leave it unchanged. */
  testEpoch: () => number;
  /** Most recently started test request; test() increments this before awaiting transport. */
  testSequence: () => number;
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

/** The pasted policy names the tags, owner and grant; the form alternative shows the same values field by field. */
export function policyParts(snippet: string): { tags: { name: string; owners: string[] }[]; src: string; dst: string; port: string } | undefined {
  try {
    const policy = JSON.parse(snippet) as { tagOwners?: Record<string, string[]>; grants?: { src?: string[]; dst?: string[]; ip?: string[] }[] };
    const tags = Object.entries(policy.tagOwners ?? {}).map(([name, owners]) => ({ name, owners }));
    const grant = policy.grants?.[0];
    if (!tags.length || tags.some((tag) => !Array.isArray(tag.owners) || !tag.owners.length) || !grant?.src?.[0] || !grant.dst?.[0] || !grant.ip?.[0]) return undefined;
    return { tags, src: grant.src[0], dst: grant.dst[0], port: grant.ip[0] };
  } catch { return undefined; }
}
