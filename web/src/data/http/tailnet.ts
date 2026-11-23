// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import type { components } from "@/api/generated/schema";
import { CancelledError } from "@tanstack/react-query";
import type { TailnetApi, TailnetSettings, TailnetStatus, TestResult, TestCheck } from "@/domain/tailnet";
import type { CsrfClient } from "./csrf-client";
import { runtimeResponseError } from "./runtime-mapping";

type Environment = components["schemas"]["EnvironmentEnvironment"];
type PeerProjection = Pick<Environment, "registrationRevision" | "networkRoute" | "tailnet">;

export function createTailnet(baseUrl: string, client: CsrfClient, identity: () => number) {
  const peers = new Map<string, PeerProjection>();
  let frame = 0;
  let current: TailnetStatus | undefined;
  let revision = 0;
  let testEpoch = 0, testSequence = 0;
  let streamEpoch = 0, observedEpoch = 0;
  // While any credential write settles, same-epoch stream statuses up to its pre-write generation are the replaced lifecycle.
  const writes = new Set<{ floor: number; epoch: number }>();
  let lifetime = new AbortController();
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((listener) => listener());
  function observe(next: TailnetStatus): boolean {
    if (next.credentialRevision !== undefined && (typeof next.credentialRevision !== "string" || !next.credentialRevision)) return false;
    if (!Number.isSafeInteger(next.generation) || next.generation < 0 || (current && observedEpoch === streamEpoch && next.generation < current.generation)) return false;
    ++revision; observedEpoch = streamEpoch; current = next; notify(); return true;
  }
  async function request<T>(path: string, method = "GET", body?: object): Promise<T> {
    const owner = identity();
    let response: Response;
    try { response = await client.fetch(`${baseUrl}${path}`, { method, signal: lifetime.signal, cache: "no-store", headers: { Accept: "application/json", ...(body ? { "Content-Type": "application/json" } : {}) }, ...(body ? { body: JSON.stringify(body) } : {}) }); }
    catch (error) { if (owner !== identity()) throw new CancelledError(); throw error; }
    const payload = await response.json();
    if (owner !== identity()) throw new CancelledError();
    if (!response.ok) throw runtimeResponseError(payload, response.status);
    if (payload.success !== true || !payload.data) throw new Error("tailnet.invalid-response");
    return payload.data;
  }
  async function status() {
    const started = revision;
    const next = await request<TailnetStatus>("/tailnet/status");
    if (started === revision) observe(next);
    return current;
  }
  const api: TailnetApi = {
    current: () => current,
    subscribe: (listener) => { listeners.add(listener); return () => listeners.delete(listener); },
    status,
    testEpoch: () => testEpoch,
    testSequence: () => testSequence,
    async test() {
      const epoch = testEpoch, sequence = ++testSequence;
      const result = decodeTest(await request<unknown>("/tailnet/test", "POST"));
      if (epoch !== testEpoch || sequence !== testSequence || result.stale || current?.credentialRevision !== result.credentialRevision) throw new CancelledError();
      return result;
    },
    settings: () => request<TailnetSettings>("/settings/connections/tailnet"),
    async save(input) {
      // The hub keeps the generation when a same-mode credential replaces a connected run, so the post-write read decides.
      const owned = { floor: current?.generation ?? -1, epoch: streamEpoch };
      ++testEpoch; ++revision; writes.add(owned);
      try {
        const saved = await request<TailnetSettings>("/settings/connections/tailnet", "PUT", input);
        observe(await request<TailnetStatus>("/tailnet/status"));
        return saved;
      } finally { writes.delete(owned); }
    },
    async reset() { ++testEpoch; ++revision; observe(await request<TailnetStatus>("/tailnet/reset", "POST")); },
  };
  function observeHosts(rows: Environment[]) {
    ++frame; peers.clear();
    for (const row of rows) peers.set(row.id, { registrationRevision: row.registrationRevision, networkRoute: row.networkRoute, tailnet: row.tailnet });
  }
  /** `readFrom` is frame() when the catalogue read started: a read after the latest frame keeps the peer fields it carries. */
  function project(row: Environment, readFrom = -1): Environment {
    const peer = peers.get(row.id);
    if (!peer || typeof peer.registrationRevision !== "number" || peer.registrationRevision !== row.registrationRevision) return row;
    if (readFrom >= frame) return { ...row, networkRoute: row.networkRoute ?? peer.networkRoute, tailnet: row.tailnet ?? peer.tailnet };
    return { ...row, ...peer };
  }
  // Generations are process-local. An SSE reopen starts a fresh observation boundary while preserving owned writes.
  async function reconnect() {
    ++testEpoch; ++streamEpoch; ++revision; peers.clear();
    if (current) await status();
  }
  function observeStream(next: TailnetStatus): boolean {
    for (const write of writes) if (write.epoch === streamEpoch && next.generation <= write.floor) return false;
    return observe(next);
  }
  return { api, observe: observeStream, observeHosts, frame: () => frame, project, reconnect, clear: () => { ++testEpoch; ++revision; peers.clear(); lifetime.abort(); lifetime = new AbortController(); current = undefined; notify(); } };
}

const checkIds = ["credential", "agent-key", "cleanup", "hub-joined", "hub-tag", "hub-address"] as const;
const checkStates = ["pass", "fail", "skipped", "unknown"] as const;
const checkCodes = ["tailnet.credential-rejected", "tailnet.provider-unavailable", "tailnet.not-connected", "tailnet.mint-refused", "tailnet.test-cleanup-failed", "tailnet.test-running"] as const;
function decodeTest(value: unknown): TestResult {
  const invalid = (): never => { throw new Error("tailnet.invalid-response"); };
  if (!value || typeof value !== "object") invalid();
  const row = value as Record<string, unknown>;
  if (typeof row.credentialRevision !== "string" || !row.credentialRevision || typeof row.stale !== "boolean" || !Array.isArray(row.checks) || row.checks.length !== checkIds.length) invalid();
  const seen = new Set<string>();
  const checks = (row.checks as unknown[]).map((value): TestCheck => {
    if (!value || typeof value !== "object") invalid();
    const c = value as Record<string, unknown>;
    if (!checkIds.includes(c.id as TestCheck["id"]) || seen.has(c.id as string) || !checkStates.includes(c.state as TestCheck["state"]) ||
      (c.code !== undefined && !checkCodes.includes(c.code as typeof checkCodes[number])) ||
      (c.providerStatus !== undefined && (!Number.isInteger(c.providerStatus) || (c.providerStatus as number) < 100 || (c.providerStatus as number) > 599))) invalid();
    seen.add(c.id as string);
    return { id: c.id as TestCheck["id"], state: c.state as TestCheck["state"], ...(c.code === undefined ? {} : { code: c.code as TestCheck["code"] }), ...(c.providerStatus === undefined ? {} : { providerStatus: c.providerStatus as number }) };
  });
  return { credentialRevision: row.credentialRevision as string, stale: row.stale as boolean, checks };
}
