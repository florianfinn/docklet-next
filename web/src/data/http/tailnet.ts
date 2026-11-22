// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import type { components } from "@/api/generated/schema";
import { CancelledError } from "@tanstack/react-query";
import type { TailnetApi, TailnetSettings, TailnetStatus } from "@/domain/tailnet";
import type { CsrfClient } from "./csrf-client";
import { runtimeResponseError } from "./runtime-mapping";

type Environment = components["schemas"]["EnvironmentEnvironment"];
type PeerProjection = Pick<Environment, "registrationRevision" | "networkRoute" | "tailnet">;

export function createTailnet(baseUrl: string, client: CsrfClient, identity: () => number) {
  const peers = new Map<string, PeerProjection>();
  let current: TailnetStatus | undefined;
  let revision = 0;
  let streamEpoch = 0, observedEpoch = 0;
  let lifetime = new AbortController();
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((listener) => listener());
  function observe(next: TailnetStatus): boolean {
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
    settings: () => request<TailnetSettings>("/settings/connections/tailnet"),
    async save(input) {
      const before = current?.generation ?? 0, epoch = streamEpoch;
      ++revision;
      const saved = await request<TailnetSettings>("/settings/connections/tailnet", "PUT", input);
      // The write response has no generation. Fence the old lifecycle before reading its replacement.
      if (epoch === streamEpoch && current && current.generation <= before) observe({ ...current, generation: before + 1, enabled: input.enabled, connection: { state: input.enabled ? "joining" : "off" }, routeAvailable: false });
      await status();
      return saved;
    },
    async reset() { ++revision; observe(await request<TailnetStatus>("/tailnet/reset", "POST")); },
  };
  function observeHosts(rows: Environment[]) {
    peers.clear();
    for (const row of rows) peers.set(row.id, { registrationRevision: row.registrationRevision, networkRoute: row.networkRoute, tailnet: row.tailnet });
  }
  function project(row: Environment): Environment {
    const peer = peers.get(row.id);
    return peer && typeof peer.registrationRevision === "number" && peer.registrationRevision === row.registrationRevision ? { ...row, ...peer } : row;
  }
  // Generations are process-local. An SSE reopen starts a fresh observation boundary while preserving owned writes.
  async function reconnect() {
    ++streamEpoch; ++revision; peers.clear();
    if (current) await status();
  }
  return { api, observe, observeHosts, project, reconnect, clear: () => { ++revision; peers.clear(); lifetime.abort(); lifetime = new AbortController(); current = undefined; notify(); } };
}
