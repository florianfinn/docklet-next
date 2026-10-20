// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

import { vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";
import { ActivitySource, deferred } from "./activity-reader.test-support";
import { LogSocket } from "./logs.test-support";
import { envelope, response, runtimeDetail, snapshot } from "./runtime-commands.test-support";

export function sessionFixture() {
  const state = { authenticated: true, credentialFailure: false, authHeld: undefined as undefined | ReturnType<typeof deferred<Response>>, ambiguous: false, blocked: undefined as undefined | ReturnType<typeof deferred<Response>>, commandHeld: undefined as undefined | ReturnType<typeof deferred<Response>> };
  const sources: ActivitySource[] = [], sockets: { url: string; socket: LogSocket }[] = [];
  const calls: { path: string; init: RequestInit }[] = [];
  const user = { id: "example-user", username: "operator", requiresPasswordChange: false };
  const fetcher = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(String(input), "https://hub.example.com"), path = url.pathname;
    calls.push({ path, init });
    if (path.endsWith("/auth/mode")) return envelope({ mode: "local" });
    if (path.endsWith("/auth/logout")) { state.authenticated = false; return envelope({}); }
    if (path.endsWith("/auth/login")) { if (state.credentialFailure) return response({}, 401); state.authenticated = true; return envelope({ user }); }
    if (path.endsWith("/auth/password")) return state.credentialFailure ? response({}, 401) : envelope({});
    if (path.endsWith("/auth/me") && state.authHeld) return state.authHeld.promise;
    if (path.endsWith("/auth/me")) return state.authenticated ? envelope(user) : response({}, 401);
    if (path.endsWith("/capabilities")) return envelope(["listHosts", "listActions", "subscribeMetrics", "subscribeLogs", "subscribeContainerLogs", "runContainerCommand", "runStackCommand", "listEvents"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
    if (path.endsWith("/environments")) return state.blocked ? state.blocked.promise : response({ success: true, data: [{ id: "0", name: "Example", apiUrl: "https://agent.example.com", enabled: true, status: "online", isEdge: false }], pagination: { totalItems: 1 } });
    if (path.endsWith("/activities") || path.endsWith("/containers") || path.endsWith("/events")) return response({ success: true, data: [], pagination: { totalItems: 0 } });
    if (path.includes("/runtime-runs/")) return envelope(runtimeDetail(snapshot()));
    if (/\/(start|stop|up)$/.test(path)) { if (state.commandHeld) return state.commandHeld.promise; if (state.ambiguous) throw new TypeError("Example lost response"); return envelope({ code: "run.started", activityId: snapshot().id, activity: snapshot() }, 202); }
    return envelope({ effectiveTimeZone: "UTC", settings: [] });
  });
  const adapter = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => { const source = new ActivitySource(); sources.push(source); return source; }, webSocket: (url) => { const socket = new LogSocket(); sockets.push({ url, socket }); return socket; } });
  const sample = (socket: LogSocket, cpu = 25) => socket.send({ cpuUsage: cpu, memoryUsage: 100, memoryTotal: 1000, diskUsage: 10, diskTotal: 100, cpuCount: 2 });
  return { adapter, state, calls, sources, sockets, sample, user };
}
