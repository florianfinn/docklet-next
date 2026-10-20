// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { createHttpAdapter } from "./http-adapter";
import { ActivitySource, at } from "./activity-reader.test-support";
import { createCsrfClient } from "./csrf-client";
import { runtimeTokenReads } from "./runtime-token-read";
import { createRuntimeCommands } from "./runtime-commands";
import type { RuntimeDetail, RuntimeRun, RuntimeSnapshot } from "./runtime-mapping";

export const RUN_ID = "1dd08eac-6846-4ba1-bbea-a3e5112cc860";
export const csrf = "example".padEnd(43, "x");
export const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf } });
export const envelope = (data: unknown, status = 200) => response({ success: true, data }, status);
export function snapshot(host = "0", patch: Partial<RuntimeSnapshot> = {}): RuntimeSnapshot {
  return { id: RUN_ID, environmentId: host, type: "container_start", resourceType: "container", resourceId: "canonical-id", resourceName: "Example container", status: "running", startedAt: at, createdAt: at, updatedAt: at, outcomeState: host === "0" ? "known" : "unknown", cancelRequested: false, ...patch };
}
export const runtimeDetail = (run: RuntimeSnapshot, patch: Partial<RuntimeDetail> = {}): RuntimeDetail => ({ activity: run, messages: [], stepTrace: [], output: [], outputTruncated: false, retainedStartSeq: 0, recovery: { executorState: "pending", releaseAcknowledged: false }, ...patch });
export function runtimeFixture() {
  const state = {
    run: snapshot(), detailStatus: 200, admin: true,
    permissions: ["activities:read", "activities:cancel"],
    recovery: { executorState: "pending", releaseAcknowledged: false } as RuntimeDetail["recovery"],
    auth: undefined as undefined | ((url: URL, init: RequestInit) => Promise<Response> | Response),
    activity: undefined as undefined | ((url: URL, init: RequestInit) => Promise<Response> | Response),
    command: undefined as undefined | ((url: URL, init: RequestInit) => Promise<Response> | Response),
    read: undefined as undefined | ((url: URL, init: RequestInit) => Promise<Response> | Response),
  };
  const calls: { url: URL; init: RequestInit }[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(String(input), "https://hub.example.com"); calls.push({ url, init });
    if (url.pathname.endsWith("/capabilities")) return envelope(["runStackCommand", "runContainerCommand", "listActions", "listHosts"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
    if (url.pathname.includes("/auth/") && state.auth) return state.auth(url, init);
    if (url.pathname.includes("/activities") && state.activity) return state.activity(url, init);
    if (url.pathname.endsWith("/auth/mode")) return envelope({ mode: "local" });
    if (url.pathname.endsWith("/auth/me")) return envelope({ id: "example-user", username: "operator", requiresPasswordChange: false, isGlobalAdmin: state.admin, permissionsByEnv: { global: state.permissions } });
    if (url.pathname.includes("/auth/")) return envelope({});
    if (url.pathname.endsWith("/environments")) return response({ success: true, data: [{ id: "direct", isEdge: false }, { id: "edge", isEdge: true }], pagination: { totalItems: 2 } });
    if (url.pathname.endsWith("/activities")) return response({ success: true, data: [], pagination: { totalItems: 0 } });
    if (url.pathname.includes("/runtime-runs/")) {
      if (state.read) return state.read(url, init);
      if (init.method === "POST") return envelope({ ...state.run, cancelRequested: true });
      return state.detailStatus === 200 ? envelope(runtimeDetail(state.run, { recovery: state.recovery })) : response({ code: "http.error" }, state.detailStatus);
    }
    if (state.command) return state.command(url, init);
    return envelope({ activityId: state.run.id, code: "run.started", activity: state.run }, 202);
  });
  const source = new ActivitySource();
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => source as unknown as EventSource });
  const published: RuntimeRun[] = [];
  const clock = { elapsed: 0 };
  const unauthorized = vi.fn();
  const helper = createRuntimeCommands({ baseUrl: "/api", unauthorized, client: createCsrfClient("/api", runtimeTokenReads("/api", fetcher as typeof fetch)), capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), publish: (run) => published.push(run), now: () => clock.elapsed });
  const commands = () => calls.filter(({ url }) => /\/(projects|containers)\//.test(url.pathname));
  return { adapter, helper, state, calls, commands, published, clock, source, fetcher, unauthorized };
}
export type UserDTO = components["schemas"]["UserUser"];
