// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 87bcad9941f6f9663bdf816d55e647faa57b922c
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { applyLabRoute, deleteLabRoute, discoverLab, labRouteStatus, labStore, listLabRoutes, reapplyLab, removeLabRoutes } from "./lab-bridge";
import { INITIAL_ROUTES, proxyStore } from "./model";
import { routeText } from "./route-text";
const route = { ...INITIAL_ROUTES[0]!, name: "lab-example", proxy: "lab-example", resolver: "", backends: ["http://192.0.2.81:8080"] };
const fetchMock = vi.fn();
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
beforeEach(() => {
  proxyStore.reset(); labStore.reset(); vi.stubGlobal("fetch", fetchMock); fetchMock.mockReset();
  proxyStore.set((s) => ({ ...s, instances: [...s.instances, { id: route.proxy, kind: "caddy", name: "Lab", hostId: "example" }], connections: { [route.proxy]: { mode: "discovered", container: "example", host: "example", image: "caddy:2.10.2", configPath: "/etc/caddy", apiUrl: "", lab: "caddy" } } }));
});
afterEach(() => { proxyStore.reset(); labStore.reset(); vi.unstubAllGlobals(); });
describe("same-origin lab transport and observations", () => {
  it("quietly hides an unavailable or malformed bridge", async () => {
    fetchMock.mockRejectedValueOnce(new Error("unavailable")); await discoverLab(); expect(labStore.get().info).toBeNull();
    fetchMock.mockResolvedValueOnce(response({ proxies: "wrong" })); await discoverLab(); expect(labStore.get().info).toBeNull();
    expect(fetchMock.mock.calls[0]![0]).toBe("/lab-api/info");
  });
  it("deduplicates concurrent discovery and reads runtime-only suggestions", async () => {
    const info = { proxies: [{ kind: "caddy", container: "example", image: "caddy:2.10.2", reachable: true, capabilities: {}, hostnames: ["lab.example.com"] }], backends: ["http://192.0.2.80:8080"] };
    fetchMock.mockResolvedValue(response(info)); await Promise.all([discoverLab(), discoverLab()]);
    expect(fetchMock).toHaveBeenCalledTimes(1); expect(labStore.get().info).toEqual(info);
  });
  it("sends generated text and four probes; changed drafts lose applied status", async () => {
    fetchMock.mockResolvedValue(response({ ok: true, probe: [{ status: 200, backend: "a" }, { status: 200, backend: "b" }] }));
    expect(labRouteStatus(route)).toBe("labNotApplied"); await applyLabRoute(route);
    const [url, request] = fetchMock.mock.calls[0]!;
    expect(url).toBe("/lab-api/caddy/routes/lab-example");
    expect(JSON.parse(request.body)).toEqual({ text: routeText(route), probe: { domain: route.domain, path: "/", tls: true, count: 4 } });
    expect(labRouteStatus(route, labStore.get().records[route.id])).toBe("labApplied");
    expect(labRouteStatus({ ...route, url: "http://192.0.2.82:8080" }, labStore.get().records[route.id])).toBe("labNotApplied");
    expect(labRouteStatus(route, undefined)).toBe("labNotApplied");
  });
  it("retains real stage, snippet line and probe/router errors", async () => {
    const failed = { ok: false, stage: "adapt", error: "snippet line 4: unknown directive" };
    fetchMock.mockResolvedValueOnce(response(failed, 422)); expect(await applyLabRoute(route, "invalid")).toEqual(failed);
    expect(labRouteStatus(route, labStore.get().records[route.id])).toBe("labError");
    fetchMock.mockResolvedValueOnce(response({ ok: true, errors: ["middleware missing"] })); await applyLabRoute(route);
    expect(labRouteStatus(route, labStore.get().records[route.id])).toBe("labError");
    fetchMock.mockResolvedValueOnce(response({ ok: true, probe: [{ error: "reset" }] })); await applyLabRoute(route);
    expect(labRouteStatus(route, labStore.get().records[route.id])).toBe("labError");
  });
  it("keeps pending results unknown until the actual response arrives", async () => {
    let resolve!: (value: Response) => void;
    fetchMock.mockReturnValue(new Promise<Response>((r) => { resolve = r; }));
    const pending = applyLabRoute(route); expect(labRouteStatus(route, labStore.get().records[route.id])).toBe("labNotApplied");
    resolve(response({ ok: true, probe: [{ status: 200 }] })); await pending;
    expect(labRouteStatus(route, labStore.get().records[route.id])).toBe("labApplied");
  });
  it("cleans the old saved name on rename, including after observations reset", async () => {
    proxyStore.set((s) => ({ ...s, routes: [route] }));
    fetchMock.mockImplementation(async () => response({ ok: true })); const renamed = { ...route, name: "renamed" }; await applyLabRoute(renamed);
    expect(fetchMock.mock.calls.map(([url, req]) => [url, req.method])).toEqual([["/lab-api/caddy/routes/renamed", "PUT"], ["/lab-api/caddy/routes/lab-example", "DELETE"]]);
    await deleteLabRoute(renamed); expect(fetchMock.mock.lastCall![1].method).toBe("DELETE");
    expect(labStore.get().records[route.id]).toBeUndefined();
  });
  it("retains attempted lab names across a reload after a failed rename", async () => {
    proxyStore.set((s) => ({ ...s, routes: [route] }));
    fetchMock.mockResolvedValueOnce(response({ ok: false, stage: "load", error: "unreachable" }, 502));
    await applyLabRoute({ ...route, name: "renamed" });
    expect(proxyStore.get().routes[0]!.labNames).toEqual(["lab-example", "renamed"]);
    labStore.reset(); fetchMock.mockReset(); fetchMock.mockImplementation(async () => response({ ok: true }));
    await deleteLabRoute(proxyStore.get().routes[0]!);
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(["/lab-api/caddy/routes/lab-example", "/lab-api/caddy/routes/renamed"]);
  });
  it("retains delete failures for retry and deletes saved names after reload", async () => {
    fetchMock.mockResolvedValueOnce(response({ ok: false, stage: "load", error: "unreachable" }, 502));
    expect((await deleteLabRoute(route)).ok).toBe(false); expect(labStore.get().records[route.id]?.result.error).toBe("unreachable");
    fetchMock.mockImplementation(async () => response({ ok: true })); await deleteLabRoute(route); expect(labStore.get().records[route.id]).toBeUndefined();
  });
  it("re-applies only this instance's file routes and clears busy after errors", async () => {
    proxyStore.set((s) => ({ ...s, routes: [...s.routes, route, { ...route, id: "labels", source: "labels" }] }));
    fetchMock.mockRejectedValue(new Error("network")); await reapplyLab(route.proxy);
    expect(fetchMock).toHaveBeenCalledTimes(1); expect(labStore.get().busy).toBe(false);
    expect(labRouteStatus(route, labStore.get().records[route.id])).toBe("labError");
  });
  it("cleans up listed lab routes explicitly and keeps browser routes", async () => {
    proxyStore.set((s) => ({ ...s, routes: [{ ...route, labNames: ["lab-example", "stale"] }] }));
    fetchMock.mockResolvedValue(response({ ok: true })); await applyLabRoute(route);
    fetchMock.mockReset(); fetchMock.mockResolvedValueOnce(response([{ name: "lab-example" }, { name: "foreign" }]));
    expect(await listLabRoutes("caddy")).toEqual(["lab-example", "foreign"]);
    fetchMock.mockImplementation(async () => response({ ok: true }));
    expect((await removeLabRoutes("caddy", ["lab-example", "foreign"])).ok).toBe(true);
    expect(fetchMock.mock.calls.slice(1).map(([url, req]) => [url, req.method])).toEqual([["/lab-api/caddy/routes/lab-example", "DELETE"], ["/lab-api/caddy/routes/foreign", "DELETE"]]);
    expect(proxyStore.get().routes.map((r) => [r.id, r.labNames])).toEqual([[route.id, ["stale"]]]);
    expect(labStore.get().records[route.id]).toBeUndefined();
  });
  it("stops cleanup at the first failure and keeps tracking of names not removed", async () => {
    proxyStore.set((s) => ({ ...s, routes: [{ ...route, labNames: ["lab-example", "second"] }] }));
    fetchMock.mockResolvedValueOnce(response({ ok: true })).mockResolvedValueOnce(response({ ok: false, stage: "load", error: "unreachable" }, 502));
    const result = await removeLabRoutes("caddy", ["lab-example", "second"]);
    expect([result.ok, result.error]).toEqual([false, "unreachable"]);
    expect(proxyStore.get().routes[0]!.labNames).toEqual(["second"]);
  });
});
