// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { createHttpAdapter } from "@/data/http/http-adapter";
import type { TailnetSettings, TailnetStatus } from "@/domain/tailnet";

export type Environment = components["schemas"]["EnvironmentEnvironment"];
export const at = "2026-11-22T12:00:00Z";
export function connectedStatus(): TailnetStatus {
  return { enabled: true, generation: 1, credentialRevision: "example-boot:1", connection: { state: "connected" }, provisioning: { state: "ok" }, mode: "oauth", hostCount: 1, onlineHosts: 1, hubAddress: "192.0.2.10", tailnetName: "example.com", routeAvailable: true };
}
export const peerHost: Environment = { id: "example-agent", name: "Example agent", apiUrl: "edge://pending", enabled: true, isEdge: true, status: "online", color: null, registrationRevision: 1, networkRoute: "tailnet", tailnet: { state: "online", path: "relay", lastSeen: at, tagged: true, failingLink: "none" } };

class Source {
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  close() {}
  seq = 0;
  send(status: TailnetStatus | undefined, rows: Environment[]) {
    // EnvironmentHandler.RunStreamProducer and types/stream: one snapshot, same projected hosts and hub status.
    this.onmessage?.({ data: JSON.stringify({ timestamp: at, channel: "environments", environment: { type: "snapshot", timestamp: at, environments: rows, ...(status ? { tailnet: status } : {}) } }), lastEventId: String(++this.seq) } as MessageEvent);
  }
}

// Exact text of backend tailnet.PolicySnippet() (HubTag, AgentTag, IngressPort 3552).
export const PRODUCER_POLICY = `{
  "tagOwners": {
    "tag:docklet-hub": ["autogroup:admin"],
    "tag:docklet-agent": ["autogroup:admin"]
  },
  "grants": [
    {"src": ["tag:docklet-agent"], "dst": ["tag:docklet-hub"], "ip": ["tcp:3552"]}
  ]
}`;

export function tailnetFixture(initial: TailnetStatus = connectedStatus(), admin = true) {
  const state = { status: initial, rows: [peerHost], settings: { enabled: initial.enabled, mode: initial.mode, clientId: "example-client", secretConfigured: true, hostname: "docklet-hub", customControl: false, policySnippet: PRODUCER_POLICY } as TailnetSettings, admin, test: undefined as undefined | (() => Response | Promise<Response>) };
  const sources: Source[] = [];
  const writes: { path: string; body: Record<string, unknown> }[] = [];
  const requests: string[] = [];
  const values = { setupCompleted: "false", hubName: "Example hub", hubAddress: "192.0.2.10:3552", domainUrl: "https://hub.example.com", linkBase: "auto", effectiveTimeZone: "UTC", keepActionOutput: "true", actionPresentation: "dialog", moduleProxyEnabled: "false", defaultHostConnection: "edge", agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/projects/docklet-agent", defaultPuid: "", defaultPgid: "", detectSocketGroup: "true" };
  const json = (data: unknown, more?: object) => new Response(JSON.stringify({ success: true, data, ...more }), { headers: { "Content-Type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    requests.push(path);
    const method = init?.method ?? "GET";
    if (path === "/api/auth/me") return json({ id: state.admin ? "admin" : "reader", username: "Example", isAdmin: state.admin, requiresPasswordChange: false, assignments: [] });
    if (path === "/api/auth/mode") return json({ mode: "none", setupPending: false });
    if (path === "/api/capabilities") return json(["listHosts", "registerHost", "discoverHub", "removeHost"].map((operationId) => ({ operationId, available: state.admin || operationId === "listHosts", resourceRef: { kind: "hub", id: "0" } })));
    if (method !== "GET") {
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : {};
      writes.push({ path, body });
      if (path === "/api/settings/connections/tailnet") {
        // tailnet.Manager.updateCredential: a same-mode credential for a connected, enabled run keeps its generation and connection.
        const kept = body.enabled === true && state.status.enabled && (body.mode ?? state.settings.mode) === state.settings.mode && state.status.connection.state === "connected";
        state.settings = { ...state.settings, enabled: body.enabled === true, ...(body.mode ? { mode: String(body.mode) } : {}), secretConfigured: true };
        if (!kept) state.status = { ...state.status, generation: state.status.generation + 1, enabled: body.enabled === true, connection: { state: body.enabled ? "joining" : "off" }, routeAvailable: false };
        return json(state.settings);
      }
      if (path === "/api/tailnet/test") return state.test ? state.test() : json({ credentialRevision: state.status.credentialRevision, stale: false, checks: [{ id: "credential", state: "pass" }, { id: "agent-key", state: "pass" }, { id: "cleanup", state: "pass" }, { id: "hub-joined", state: "pass" }, { id: "hub-tag", state: "pass" }, { id: "hub-address", state: "pass" }] });
      if (path === "/api/tailnet/reset") { state.status = { ...state.status, generation: state.status.generation + 1, enabled: false, connection: { state: "off" }, routeAvailable: false }; return json(state.status); }
      if (path === "/api/environments") {
        const row = { ...peerHost, status: "pending", apiKey: "example", tailnetAuthKey: body.tailnetAuthKey ?? "tskey-auth-example-once", tailnetManagerUrl: "http://192.0.2.10:3552" };
        return json(row);
      }
      if (path === "/api/environments/example-agent") return json(peerHost);
      if (path === "/api/environments/0/settings") return json([]);
    }
    if (path === "/api/tailnet/status") return json(state.status);
    if (path === "/api/settings/connections/tailnet") return json(state.settings);
    if (path === "/api/environments") return json(state.rows, { pagination: { totalItems: state.rows.length } });
    if (path === "/api/environments/0/settings" || path === "/api/environments/0/settings/public") return new Response(JSON.stringify(Object.entries(values).map(([key, value]) => ({ key, value }))));
    if (path === "/api/hub/discovery") return json({ detectedAt: at, agentImage: "registry.example.com/docklet/agent:1.2.3", candidates: [{ id: "lan", url: "http://192.0.2.10:3552", kind: "ip", source: "configured", reach: "unknown" }] });
    if (path === "/api/app-version") return json({ updateAvailable: false });
    if (path.endsWith("/activities")) return json([], { pagination: { totalItems: 0 } });
    throw new Error(`Unexpected test request ${method} ${path}`);
  });
  const adapter = createHttpAdapter({ fetch: fetcher, eventSource: () => { const source = new Source(); sources.push(source); return source; } });
  return { adapter, fetcher, state, sources, writes, requests, send: (status: TailnetStatus | undefined, rows: Environment[] = state.rows) => { state.rows = rows; if (status) state.status = status; sources.at(-1)!.send(status, rows); } };
}
