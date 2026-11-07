// Source: docklet-next/issues/360
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original real HTTP adapter fixtures for shell reads and terminal ownership.

import { vi } from "vitest";
import { resourceFixture } from "./resources.test-support";
import { createHttpAdapter } from "./http-adapter";
import { ActivitySource, deferred } from "./activity-reader.test-support";
import { envelope, response, snapshot, runtimeDetail } from "./runtime-commands.test-support";

export function hardeningFixture() {
  const inventory = resourceFixture();
  const source = new ActivitySource();
  const state = { stopped: false, run: snapshot("0", { type: "project_down", resourceType: "project", resourceId: "project", resourceName: "app" }),
    holdDetail: false, heldHost: "", holdPhase: "", detailStarted: deferred<void>(), detailGate: deferred<void>(), heldSignals: [] as AbortSignal[], admin: true };
  const calls: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(String(input), "https://hub.example.com"); const path = url.pathname;
    calls.push(`${init.method ?? "GET"} ${path}${url.search}`);
    if (path.endsWith("/auth/mode")) return envelope({ mode: "local" });
    if (path.endsWith("/auth/me")) return envelope({ id: "operator", username: "operator", isAdmin: state.admin, isGlobalAdmin: state.admin, requiresPasswordChange: false, permissionsByEnv: { global: ["activities:read", "activities:cancel"] } });
    if (path.includes("/auth/")) return envelope({});
    if (path.endsWith("/capabilities")) return envelope(["listHosts", "listStacks", "getStack", "listContainers", "listImages", "listVolumes", "getVolume", "getTopology", "getSecurity", "listActions", "runStackCommand", "getBackupOverview"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: url.searchParams.get("kind") ?? "hub", id: url.searchParams.get("id") ?? "0" } })));
    if (path.endsWith("/settings/public")) return response({ featureVulnerabilityManagementEnabled: "false" });
    if (path === "/api/environments/0/settings") return response([{ key: "stackStopMode", value: "down" }, { key: "setupCompleted", value: "true" }]);
    if (path.endsWith("/runtime-conditions")) return envelope({ items: [] });
    if (path.endsWith("/activities")) return response({ success: true, data: path.includes(`/environments/${state.run.environmentId}/`) ? [state.run] : [], pagination: { totalItems: path.includes(`/environments/${state.run.environmentId}/`) ? 1 : 0 } });
    if (path.includes("/activities/")) {
      state.detailStarted.resolve();
      if (state.holdDetail) await state.detailGate.promise;
      return envelope(runtimeDetail(state.run));
    }
    if (path.includes("/runtime-runs/")) return envelope(runtimeDetail(state.run));
    if (init.method === "POST" && /\/projects\/project\/(down|up|restart|redeploy)$/.test(path)) return envelope({ activityId: state.run.id, code: "run.started", activity: state.run }, 202);
    if (state.heldHost && path.includes(`/environments/${state.heldHost}/`) && (!state.holdPhase || path.includes(state.holdPhase))) {
      state.heldSignals.push(init.signal!);
      return new Promise<Response>((_resolve, reject) => {
        const abort = () => reject(init.signal?.reason);
        init.signal?.addEventListener("abort", abort, { once: true });
        if (init.signal?.aborted) abort();
      });
    }
    const result = await inventory.fetcher(input);
    if (!state.stopped || !result.ok || !/\/(projects|containers)(\/project)?$/.test(path)) return result;
    const body = await result.json();
    const stop = (row: Record<string, unknown>) => ({ ...row, status: "stopped", state: "exited", runningCount: 0, runtimeServices: [] });
    body.data = Array.isArray(body.data) ? body.data.map(stop) : stop(body.data);
    return Response.json(body);
  });
  const adapter = createHttpAdapter({ fetch: fetcher, eventSource: () => source, webSocket: inventory.webSocket });
  return { adapter, state, calls, source, inventory, fetcher };
}
