// Source: docklet-next/issues/102
// Revision: dd403606fbbf763798d43f4ee854adcb105e01b1
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Controlled activity fetch and stream fixtures with explicit message kinds; transport labels are synthetic.

import { vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { createHttpAdapter } from "./http-adapter";

export const at = "2026-10-18T10:00:00Z";
export type Activity = components["schemas"]["ActivityActivity"];
export type Detail = components["schemas"]["ActivityDetail"];
export const json = (data: unknown, status = 200, total?: number) => new Response(JSON.stringify(status === 200 ? { success: true, data, pagination: { totalItems: total ?? (Array.isArray(data) ? data.length : 0) } } : { code: "http.error" }), { status, headers: { "content-type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
export const activity = (hostId: string, id = `run-${hostId}`): Activity => ({ id, environmentId: hostId, status: "running", type: "project_redeploy", startedAt: at, createdAt: at, resourceName: `Example ${hostId}` });
export const detail = (hostId: string, id?: string): Detail => ({ activity: activity(hostId, id), messages: [], output: [], stepTrace: [], outputTruncated: false });
export class ActivitySource {
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  close = vi.fn();
  open() { this.readyState = 1; this.onopen?.({} as Event); }
  send(id?: string, hostId?: string, type = "updated") { this.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: at, activity: { type, activityId: id, environmentId: hostId } }), lastEventId: "" } as MessageEvent); }
  environments() { this.onmessage?.({ data: JSON.stringify({ channel: "environments", timestamp: at }), lastEventId: "" } as MessageEvent); }
  failClosed() { this.readyState = 2; this.onerror?.({} as Event); }
}
export function activityFixture(isEdge = false, reverse = false) {
  const state = {
    hosts: reverse ? ["bad", "good"] : ["good", "bad"],
    lists: { "0": [] as Activity[], good: [activity("good")], bad: [activity("bad")] } as Record<string, Activity[]>,
    details: { "run-good": detail("good"), "run-bad": detail("bad") } as Record<string, Detail>,
    failure: "" as "" | "network" | "page500" | "page502" | "page403" | "page401" | "detail404" | "missing" | "mapping" | "wrong-host" | "pagination",
    hubStatus: 200,
    user: { id: "operator", username: "operator", requiresPasswordChange: false, locale: "en-US" },
    authStatus: 200,
    hold: undefined as undefined | ((path: string, response: Response, url: URL) => Promise<Response>),
  };
  const sources: ActivitySource[] = [];
  const requests: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "https://hub.example.com");
    const path = url.pathname;
    requests.push(url.pathname + url.search);
    let response: Response;
    if (path === "/api/capabilities") response = json(["listActions", "listHosts"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: "hub", id: "0" } })));
    else if (path === "/api/environments") response = json(state.hosts.map((id) => ({ id, name: id === "good" ? "Available example" : "Unavailable example", apiUrl: "https://agent.example.com", isEdge, enabled: true, status: "online" })), state.hubStatus);
    else if (path.startsWith("/api/auth/")) response = json(path.endsWith("/mode") ? { mode: "local" } : path.endsWith("/me") ? state.user : path.endsWith("/login") ? { user: state.user } : {}, state.authStatus);
    else if (path === "/api/settings/timezone") response = json({ timezone: "UTC" });
    else {
      const match = path.match(/\/environments\/([^/]+)\/activities(?:\/(.+))?$/);
      if (!match) throw new Error("Unexpected activity fixture route");
      const [, hostId, id] = match;
      if (hostId === "bad" && !id && state.failure === "network") throw new TypeError("network unavailable");
      const status = hostId === "bad" && !id && state.failure.startsWith("page") ? Number(state.failure.slice(4)) : 200;
      if (status !== 200) response = json(null, status);
      else if (!id) response = hostId === "bad" && state.failure === "pagination" ? (url.searchParams.get("start") === "0" ? json(state.lists[hostId], 200, 2) : json(null, 502)) : json(state.lists[hostId!] ?? []);
      else if (hostId === "bad" && state.failure === "detail404") response = json(null, 404);
      else if (hostId === "bad" && state.failure === "missing") response = json({});
      else if (hostId === "bad" && state.failure === "mapping") response = json({ ...state.details[id], outputTruncated: undefined });
      else if (hostId === "bad" && state.failure === "wrong-host") response = json({ ...state.details[id], activity: { ...state.details[id]!.activity, environmentId: "good" } });
      else response = json(state.details[id]);
    }
    return state.hold ? state.hold(path, response, url) : response;
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => { const source = new ActivitySource(); sources.push(source); return source as unknown as EventSource; } });
  return { state, sources, requests, fetcher, adapter };
}
export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((r, fail) => { resolve = r; reject = fail; });
  return { promise, resolve, reject };
}
