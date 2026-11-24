// Source: docklet-next/issues/600
// Revision: 393e645a79236449c35cb8e1a4827c83bbf45da0
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original per-row isolation tests for every host projection, the visible unreadable state and the assigned-user reader.

// #600: one row that cannot be mapped never empties a host. One isolation test per host projection, plus the
// build-only service from the hub's own bytes (testdata/build-only-project.json, written by
// backend/internal/project/build_only_web_fixture_test.go).

import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { HostPulse } from "@/features/dashboard/HostPulse";
import { HostCard } from "@/features/hosts/HostCard";
import StacksPage from "@/features/stacks/overview/StacksPage";
import { setPreference } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";
import { mapHost } from "./hosts";
import { createUserInventory } from "./user-inventory";
import { inventoryId, type ProjectRow } from "./inventory-mapping";
import { resourceFixture } from "./resources.test-support";
import produced from "./testdata/build-only-project.json";

type Kind = "projects" | "containers" | "images" | "volumes" | "networks";
const MALFORMED: Record<Kind, unknown> = {
  projects: { id: "broken-project", status: "running" },
  containers: { id: "broken-container", state: "running", status: "Up" },
  images: { id: "broken-image", repoTags: ["broken:1"] },
  volumes: { id: "broken-volume", driver: "local" },
  networks: { id: "broken-network", driver: "bridge", labels: {} },
};

/** The resource fixture with malformed rows appended to the given host's list responses, and optional real producer rows. */
function isolationFixture(host: string, kinds: Kind[], extra: { projects?: unknown[]; containers?: unknown[] } = {}) {
  const f = resourceFixture();
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => {
    const response = await original(input);
    const url = new URL(String(input), "https://hub.example.com");
    const match = /^\/api\/environments\/([^/]+)\/(projects|containers|images|volumes|networks)$/.exec(url.pathname);
    if (!match || response.status !== 200) return response;
    const kind = match[2] as Kind;
    const body = await response.json();
    const rows: unknown[] = [...body.data, ...(match[1] === host ? (extra[kind as "projects" | "containers"] ?? []) : []), ...(match[1] === host && kinds.includes(kind) ? [MALFORMED[kind]] : [])];
    return new Response(JSON.stringify({ ...body, data: rows, pagination: { totalItems: rows.length } }), { status: 200, headers: response.headers });
  });
  return f;
}

describe("#600 per-row inventory isolation", () => {
  it("maps the hub's build-only, never-created service and keeps every row of the host", async () => {
    const f = isolationFixture("second", [], { projects: produced.projects });
    const stacks = await f.adapter.listStacks();
    const builder = stacks.find((s) => s.id === inventoryId("second", "project-build"));
    expect(builder?.services.find((s) => s.name === "app")).toMatchObject({ buildOnly: true, state: "stopped", update: { state: "local" } });
    expect(builder?.services.find((s) => s.name === "cache")).toMatchObject({ image: { repository: "registry.example.com/cache", tag: "7" } });
    expect(builder?.services.find((s) => s.name === "cache")?.buildOnly).toBeUndefined();
    expect(stacks.filter((s) => s.hostId === "second").map((s) => s.name).sort()).toEqual(["app", "builder", "plain"]);
    expect((await f.adapter.listContainers()).filter((c) => c.hostId === "second").map((c) => c.name).sort()).toEqual(["db", "web"]);
    const host = (await f.adapter.listHosts()).find((h) => h.id === "second");
    expect(host?.inventory?.reason).toBeUndefined();
    expect(host?.inventory?.unreadable).toBeUndefined();
  });

  it("keeps a build-only project's detail readable from the plain get-project bytes", async () => {
    const f = isolationFixture("second", [], { projects: produced.projects });
    const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input) => {
      const url = new URL(String(input), "https://hub.example.com");
      if (url.pathname === "/api/environments/second/projects/project-build") return new Response(JSON.stringify({ success: true, data: produced.detail }), { headers: { "Content-Type": "application/json", "X-Docklet-Observed-At": "2026-10-17T12:00:00Z" } });
      return original(input);
    });
    const detail = await f.adapter.getStack(inventoryId("second", "project-build"));
    expect(detail.name).toBe("builder");
    expect(detail.services.find((s) => s.name === "app")?.buildOnly).toBe(true);
  });

  it("leaves out only a malformed project and reports it on the host", async () => {
    const f = isolationFixture("second", ["projects"], { projects: [{ id: "named-broken", name: "broken", status: "stopped", runtimeServices: [{ name: "x", image: "", status: "running", containerId: "missing" }] }] });
    const stacks = await f.adapter.listStacks();
    expect(stacks.map((s) => s.id).sort()).toEqual(["first~project", "second~project"]);
    const containers = await f.adapter.listContainers();
    expect(containers.filter((c) => c.hostId === "second").map((c) => c.name).sort()).toEqual(["db", "web"]);
    expect(containers.find((c) => c.id === "second~web")?.stackId).toBe("second~project");
    const host = (await f.adapter.listHosts()).find((h) => h.id === "second");
    expect(host?.inventory?.unreadable?.stacks).toEqual({ count: 2, codes: ["inventory.invalid-image", "inventory.invalid-project"], names: ["broken"] });
    expect(host?.inventory?.reason).toBeUndefined();
    const broken = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input) => new URL(String(input), "https://hub.example.com").pathname === "/api/environments/second/projects/named-broken"
      ? new Response(JSON.stringify({ success: true, data: { id: "named-broken", name: "broken", status: "stopped" } }), { headers: { "Content-Type": "application/json", "X-Docklet-Observed-At": "2026-10-17T12:00:00Z" } })
      : broken(input));
    // Only this stack's page says it could not be read; it is not reported as missing.
    await expect(f.adapter.getStack("second~named-broken")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
  });

  it("keeps Compose ownership of every container when one project row has no name", async () => {
    const f = isolationFixture("second", [], { projects: [{ id: "nameless", status: "running", runtimeServices: [] } as unknown as ProjectRow] });
    const containers = await f.adapter.listContainers();
    expect(containers.filter((c) => c.hostId === "second").map((c) => [c.name, c.stackId])).toEqual([["web", "second~project"], ["db", "second~project"]]);
  });

  it("leaves out only a malformed container", async () => {
    const f = isolationFixture("second", ["containers"]);
    const containers = await f.adapter.listContainers();
    expect(containers.filter((c) => c.hostId === "second").map((c) => c.name).sort()).toEqual(["db", "web"]);
    expect((await f.adapter.listStacks()).filter((s) => s.hostId === "second")).toHaveLength(1);
    expect((await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.unreadable?.containers?.count).toBe(1);
  });

  it("leaves out only a malformed image, and a malformed consumer only from that image", async () => {
    const f = isolationFixture("second", ["images", "containers"]);
    const images = await f.adapter.listImages();
    expect(images.filter((i) => i.hostId === "second").map((i) => i.id).sort()).toEqual(["second~dangling", "second~sha256:user", "second~unused"]);
    expect(images.find((i) => i.id === "second~sha256:user")?.consumers.map((c) => c.name)).toEqual(["web", "db"]);
    expect((await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.unreadable?.images?.count).toBe(1);
  });

  it("leaves out only a malformed volume and keeps the consumers of the others", async () => {
    const f = isolationFixture("second", ["volumes", "containers"]);
    const volumes = await f.adapter.listVolumes();
    const second = volumes.filter((v) => v.hostId === "second");
    expect(second.map((v) => v.name)).toEqual(["data"]);
    expect(second[0]?.consumers.map((c) => c.containerName)).toEqual(["web"]);
    // The malformed volume and the consumer container the volume reader could not map.
    expect((await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.unreadable?.volumes?.count).toBe(2);
  });

  it("leaves out only a malformed network and keeps the attachments of the others", async () => {
    const f = isolationFixture("second", ["networks", "containers"]);
    const graph = await f.adapter.getTopology();
    expect(graph.networks.filter((n) => n.hostId === "second").map((n) => n.name)).toEqual(["front"]);
    expect(graph.attachments.filter((a) => a.networkId === "second~net").map((a) => a.containerId)).toEqual(["second~web", "second~db"]);
    // Placement needs only the container id, so the nameless container is not unreadable here.
    expect((await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.unreadable?.networks?.count).toBe(1);
  });

  it("counts every readable container in the navigation when one row is malformed", async () => {
    const f = isolationFixture("second", ["containers"]);
    const nav = await f.adapter.listNavInventory();
    expect(nav.find((b) => b.hostId === "second")?.containers.map((c) => c.name).sort()).toEqual(["db", "web"]);
  });

  it("keeps the other readers' unreadable counts when a reader of another kind observes the host", async () => {
    const f = isolationFixture("second", ["projects"]);
    await f.adapter.listStacks();
    await f.adapter.listImages();
    expect((await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.unreadable).toEqual({ stacks: { count: 1, codes: ["inventory.invalid-project"], names: [] } });
  });
});

/** Appends (or replaces) rows of one list response; `first` puts the injected rows before the producer's. */
function inject(f: ReturnType<typeof resourceFixture>, path: string, rows: unknown[], mode: "append" | "first" | "replace" = "append") {
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => {
    const response = await original(input);
    if (new URL(String(input), "https://hub.example.com").pathname !== path || response.status !== 200) return response;
    const body = await response.json();
    const data = mode === "replace" ? rows : mode === "first" ? [...rows, ...body.data] : [...body.data, ...rows];
    return new Response(JSON.stringify({ ...body, data, pagination: { totalItems: data.length } }), { status: 200, headers: response.headers });
  });
  return original;
}
const unreadableOf = async (f: ReturnType<typeof resourceFixture>) => (await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.unreadable;

describe("#600 context rows and reader ownership", () => {
  it("keeps every network and attachment when one network summary is null, and counts it", async () => {
    const f = resourceFixture(); inject(f, "/api/environments/second/networks", [null]);
    const graph = await f.adapter.getTopology();
    expect(graph.networks.filter((n) => n.hostId === "second").map((n) => n.name)).toEqual(["front"]);
    expect(graph.attachments.filter((a) => a.networkId === "second~net").map((a) => a.containerId)).toEqual(["second~web", "second~db"]);
    expect((await unreadableOf(f))?.networks?.count).toBe(1);
  });

  it("the network summary list leaves out only a malformed summary and never clears the topology's count", async () => {
    const f = resourceFixture(); inject(f, "/api/environments/second/networks", [null, { id: "a~b", name: "tilde" }]);
    await f.adapter.getTopology();
    expect((await f.adapter.listNetworkSummaries()).filter((n) => n.hostId === "second").map((n) => n.id)).toEqual(["net"]);
    expect((await unreadableOf(f))?.networks?.count).toBe(2);
  });

  it("a project whose id cannot be represented never owns the host's Compose containers", async () => {
    const f = resourceFixture(); inject(f, "/api/environments/second/projects", [{ id: "a~broken", name: "app", status: "running" }]);
    const containers = (await f.adapter.listContainers()).filter((c) => c.hostId === "second");
    expect(containers.map((c) => [c.name, c.stackId])).toEqual([["web", "second~project"], ["db", "second~project"]]);
    expect((await f.adapter.listVolumes()).find((v) => v.id === "second~data")?.consumers.map((c) => c.containerName)).toEqual(["web"]);
    expect((await unreadableOf(f))?.stacks?.count).toBe(1);
  });

  it("an unrelated null project before the requested one leaves the stack detail readable", async () => {
    const f = resourceFixture(); inject(f, "/api/environments/second/projects", [null], "first");
    await expect(f.adapter.getStack("second~project")).resolves.toMatchObject({ name: "app" });
  });

  it("reports the requested stack as unreadable when its row has no name or malformed runtime entries", async () => {
    const nameless = resourceFixture(); inject(nameless, "/api/environments/second/projects", [{ id: "project", status: "running" }], "replace");
    await expect(nameless.adapter.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
    const runtime = resourceFixture(); inject(runtime, "/api/environments/second/projects", [{ id: "project", name: "app", status: "running", runtimeServices: [null] }], "replace");
    await expect(runtime.adapter.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
  });

  it("keeps every volume when one bind metadata row is null, and counts the row", async () => {
    const f = resourceFixture(); f.state.binds = [null] as never;
    expect((await f.adapter.listVolumes()).filter((v) => v.hostId === "second").map((v) => v.name)).toEqual(["data"]);
    expect((await unreadableOf(f))?.volumes).toMatchObject({ count: 1, codes: ["inventory.invalid-bind"] });
  });

  it("counts an image consumer it cannot map under images and leaves the containers count to the container readers", async () => {
    const f = resourceFixture(); inject(f, "/api/environments/second/containers", [{ id: "broken", imageId: "sha256:user" }]);
    await f.adapter.listContainers();
    const images = await f.adapter.listImages();
    expect(images.find((i) => i.id === "second~sha256:user")?.consumers.map((c) => c.name)).toEqual(["web", "db"]);
    expect(await unreadableOf(f)).toMatchObject({ containers: { count: 1 }, images: { count: 1 } });
  });

  it("navigation never changes the host's counts", async () => {
    const f = resourceFixture(); const healthy = inject(f, "/api/environments/second/containers", [{ id: "broken" }]);
    await f.adapter.listContainers();
    f.fetcher.mockImplementation(healthy);
    await f.adapter.listNavInventory();
    expect((await unreadableOf(f))?.containers?.count).toBe(1);
  });

  it("metrics keep valid samples, count an unreadable container and clear the count after recovery", async () => {
    vi.useFakeTimers();
    const f = resourceFixture();
    const healthy = inject(f, "/api/environments/second/containers", [
      { id: "bad-sample", names: ["/bad-sample"], image: "example:1", state: "running", status: "Up", resourceSample: { cpuPercent: -1, memoryUsageBytes: 100 } },
      { id: "broken" },
    ]);
    let snapshot: { containers: Record<string, unknown> } | undefined;
    const stop = f.adapter.subscribeMetrics((value) => { snapshot = value; });
    try {
      await vi.advanceTimersByTimeAsync(0);
      expect(snapshot?.containers["second~web"]).toEqual({ cpu: 2.5, memMb: 100 });
      // A readable container with an invalid sample has no sample; only the unreadable row is counted.
      expect(snapshot?.containers["second~bad-sample"]).toBeUndefined();
      expect((await unreadableOf(f))?.containers?.count).toBe(1);
      f.fetcher.mockImplementation(healthy);
      await vi.advanceTimersByTimeAsync(5_000);
      expect(await unreadableOf(f)).toBeUndefined();
    } finally { stop(); vi.useRealTimers(); }
  });

  it("counts a container the topology cannot place under networks and keeps the other attachments", async () => {
    const f = resourceFixture(); inject(f, "/api/environments/second/containers", [null]);
    const graph = await f.adapter.getTopology();
    expect(graph.attachments.filter((a) => a.networkId === "second~net").map((a) => a.containerId)).toEqual(["second~web", "second~db"]);
    expect((await unreadableOf(f))?.networks?.count).toBe(1);
  });

  it("a healthy topology read clears its own count", async () => {
    const f = resourceFixture(); const healthy = inject(f, "/api/environments/second/networks", [null]);
    await f.adapter.getTopology();
    expect((await unreadableOf(f))?.networks?.count).toBe(1);
    f.fetcher.mockImplementation(healthy);
    await f.adapter.getTopology();
    expect(await unreadableOf(f)).toBeUndefined();
  });
});

/** Rewrites the data of one response path. */
function alter(f: ReturnType<typeof resourceFixture>, path: string, change: (data: never) => unknown) {
  const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => {
    const response = await original(input);
    if (new URL(String(input), "https://hub.example.com").pathname !== path || response.status !== 200) return response;
    const body = await response.json();
    return new Response(JSON.stringify({ ...body, data: change(body.data as never) }), { status: 200, headers: response.headers });
  });
}

describe("#600 own-row identity, context targets and single counting", () => {
  it("a dependency target whose id cannot be represented never suppresses a healthy dependency", async () => {
    const f = resourceFixture(); alter(f, "/api/environments/second/containers", (rows: unknown[]) => [rows[0], { ...(rows[1] as object), id: "bad~id" }, rows[1]]);
    const graph = await f.adapter.getTopology();
    expect(graph.dependencies.filter((d) => d.from === "second~web")).toEqual([{ from: "second~web", to: "second~db" }]);
    expect((await unreadableOf(f))?.networks?.count).toBe(1);
  });

  it("counts a malformed network detail once under networks while its summary keeps the network", async () => {
    const f = resourceFixture(); const healthy = f.fetcher.getMockImplementation()!;
    alter(f, "/api/environments/second/networks/net", (row: object) => ({ ...row, ipam: null }));
    const graph = await f.adapter.getTopology();
    expect(graph.networks.some((n) => n.id === "second~net")).toBe(true);
    expect((await unreadableOf(f))?.networks?.count).toBe(1);
    f.fetcher.mockImplementation(healthy);
    await f.adapter.getTopology();
    expect(await unreadableOf(f)).toBeUndefined();
  });

  it("counts one network row once when its summary and its detail both fail", async () => {
    const f = resourceFixture();
    alter(f, "/api/environments/second/networks/net", (row: object) => ({ ...row, ipam: null }));
    alter(f, "/api/environments/second/networks", (rows: { id: string }[]) => rows.map((row) => row.id === "net" ? { ...row, name: undefined } : row));
    const graph = await f.adapter.getTopology();
    expect(graph.networks.some((n) => n.id === "second~net")).toBe(false);
    expect((await unreadableOf(f))?.networks?.count).toBe(1);
  });

  it("reports an administrator stack detail or list row whose own name is not text as unreadable", async () => {
    const f = resourceFixture(); alter(f, "/api/environments/second/projects/project", (row: object) => ({ ...row, name: 42 }));
    await expect(f.adapter.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
    const listed = resourceFixture(); inject(listed, "/api/environments/second/projects", [{ id: "project", name: { text: "app" }, status: "running" }], "replace");
    await listed.adapter.listStacks();
    expect((await unreadableOf(listed))?.stacks?.count).toBe(1);
  });
});

describe("#600 structural boundary: reference entries and detail pages", () => {
  it("leaves out only a usedBy reference it cannot read and keeps the image and its other consumers", async () => {
    const f = resourceFixture();
    alter(f, "/api/environments/second/images", (rows: { id: string }[]) => rows.map((r) => r.id === "sha256:user" ? { ...r, usedBy: [{ type: "container", id: "bad~id", name: "broken" }, { type: "container", id: "hidden", name: "hidden" }, null] } : r));
    const image = (await f.adapter.listImages()).find((i) => i.id === "second~sha256:user");
    expect(image?.consumers.map((c) => c.containerId)).toEqual(["second~web", "second~db", "second~hidden"]);
    expect((await unreadableOf(f))?.images?.count).toBe(2);
  });

  it("counts a container once when it fails both as a consumer row and as a usedBy reference", async () => {
    const f = resourceFixture(); inject(f, "/api/environments/second/containers", [{ id: "broken", imageId: "sha256:user" }]);
    alter(f, "/api/environments/second/images", (rows: { id: string }[]) => rows.map((r) => r.id === "sha256:user" ? { ...r, usedBy: [{ type: "container", id: "broken", name: 7 }] } : r));
    await f.adapter.listImages();
    expect((await unreadableOf(f))?.images?.count).toBe(1);
  });

  it("leaves out only a Docker usage entry it cannot read and keeps the volume", async () => {
    const f = resourceFixture();
    alter(f, "/api/environments/second/volumes", (rows: { name: string }[]) => rows.map((r) => r.name === "data" ? { ...r, containers: ["web", "bad~id", 5] } : r));
    const volume = (await f.adapter.listVolumes()).find((v) => v.id === "second~data");
    expect(volume?.consumers.map((c) => c.containerName)).toEqual(["web"]);
    expect((await unreadableOf(f))?.volumes?.count).toBe(2);
  });

  it("answers unreadable for an empty assigned or administrator stack detail", async () => {
    const f = resourceFixture();
    const hosts = await f.adapter.listHosts();
    const principal = () => ({ id: "u", username: "user", requiresPasswordChange: false, isAdmin: false, locale: null, assignments: [{ environmentId: "second", projectId: "project", level: "restricted" }] }) as never;
    const user = createUserInventory("/api", f.fetcher as typeof fetch, async () => hosts, principal, () => undefined, async () => new Map());
    const empty = (path: string) => { const original = f.fetcher.getMockImplementation()!; f.fetcher.mockImplementation(async (input) => new URL(String(input), "https://hub.example.com").pathname === path ? new Response(JSON.stringify({ success: true, data: null }), { headers: { "Content-Type": "application/json", "X-Docklet-Observed-At": "2026-10-17T12:00:00Z" } }) : original(input)); };
    empty("/api/environments/second/projects/project/runtime");
    await expect(user.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
    empty("/api/environments/second/projects/project");
    await expect(f.adapter.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
  });
});

describe("#600 attachments as reference entries and the administrator detail boundary", () => {
  it("drops only an attachment it cannot read; the container keeps its other attachments and dependencies", async () => {
    const f = resourceFixture(); const n = f.state.networks[0]!;
    f.state.networks = [n, { ...n, id: "badnet", name: "badnet", containers: { web: { IPv4Address: 7 } } }, { ...n, id: "thirdnet", name: "thirdnet", containers: { web: { IPv4Address: "192.0.2.3/24" } } }] as never;
    const graph = await f.adapter.getTopology();
    expect(graph.attachments.some((a) => a.containerId === "second~web" && a.networkId === "second~thirdnet")).toBe(true);
    expect(graph.attachments.some((a) => a.networkId === "second~badnet")).toBe(false);
    expect(graph.dependencies.some((d) => d.from === "second~web" && d.to === "second~db")).toBe(true);
    expect((await unreadableOf(f))?.networks?.count).toBe(1);
  });

  it("answers unreadable when the host returns a list it cannot read, and keeps a transport failure's own outcome", async () => {
    const malformed = resourceFixture(); alter(malformed, "/api/environments/second/projects", () => ({ broken: true }));
    await expect(malformed.adapter.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
    const unnamed = resourceFixture(); alter(unnamed, "/api/environments/second/projects", (rows: object[]) => rows.map((r) => ({ ...r, composeProjectName: 42 })));
    await expect(unnamed.adapter.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
    const down = resourceFixture(); const original = down.fetcher.getMockImplementation()!;
    down.fetcher.mockImplementation(async (input) => new URL(String(input), "https://hub.example.com").pathname === "/api/environments/second/containers" ? new Response(JSON.stringify({ success: false, code: "docker.error" }), { status: 500, headers: { "Content-Type": "application/json" } }) : original(input));
    await expect(down.adapter.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "docker.error", status: 500 } });
    // A host failure whose code happens to look like a page error, or whose body is not an object, keeps its status.
    for (const [body, code] of [[{ success: false, code: "docker.invalid-page" }, "docker.invalid-page"], [null, "http.error"]] as const) {
      const host = resourceFixture(); const healthy = host.fetcher.getMockImplementation()!;
      host.fetcher.mockImplementation(async (input) => new URL(String(input), "https://hub.example.com").pathname === "/api/environments/second/projects" ? new Response(JSON.stringify(body), { status: 500, headers: { "Content-Type": "application/json" } }) : healthy(input));
      await expect(host.adapter.getStack("second~project")).rejects.toMatchObject({ outcome: { code, status: 500 } });
    }
  });
});

describe("#600 visible unreadable state", () => {
  afterEach(() => { setPreference("hostSelection", []); setAppLocale("de"); });
  const broken = { count: 1, codes: ["inventory.invalid-project"], names: ["proxy"] };

  it("shows the unreadable notice instead of an empty Stacks page", async () => {
    setAppLocale("en-US");
    const f = resourceFixture();
    const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input) => new URL(String(input), "https://hub.example.com").pathname === "/api/environments/second/projects"
      ? new Response(JSON.stringify({ success: true, data: [{ id: "proxy", name: "proxy", status: "stopped", runtimeServices: [{ name: "x", image: "", status: "running", containerId: "gone" }] }], pagination: { totalItems: 1 } }), { headers: { "Content-Type": "application/json", "X-Docklet-Observed-At": "2026-10-17T12:00:00Z" } })
      : original(input));
    setPreference("hostSelection", ["second"]);
    // The host catalogue carries the observation of the read before it (the app refreshes it on the change notice).
    await f.adapter.listStacks();
    renderWithApp(<StacksPage />, f.adapter);
    await waitFor(() => expect(screen.getByText("Stacks could not be read")).toBeTruthy());
    expect(document.body.textContent?.includes("second: 1 entry could not be read and is not shown. Affected: proxy.")).toBe(true);
    expect(document.body.textContent?.includes("No stacks yet")).toBe(false);
  });

  it("names unreadable entries on the host card and the dashboard host card", () => {
    setAppLocale("en-US");
    const host = { ...mapHost({ id: "atlas", name: "Example", color: "#112233", registrationRevision: 1, apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false }), inventory: { stale: false, unreadable: { stacks: broken } } };
    const card = renderWithApp(<HostCard host={host} system={undefined} stacks={2} containers={3} allIds={[host.id]} />, createDemoAdapter());
    expect(card.container.textContent?.includes("2 stacks · 3 containers · 1 entry not readable")).toBe(true);
    card.unmount();
    const pulse = renderWithApp(<HostPulse host={host} stacks={2} containers={3} />, createDemoAdapter());
    expect(pulse.container.textContent?.includes("1 entry not readable")).toBe(true);
  });
});

describe("#600 assigned-user reader", () => {
  it("leaves out only a malformed project and reports an unreadable assigned stack", async () => {
    const f = isolationFixture("second", ["projects"]);
    const hosts = await f.adapter.listHosts();
    const observed = new Map<string, unknown>();
    const principal = () => ({ id: "u", username: "user", requiresPasswordChange: false, isAdmin: false, locale: null, assignments: [{ environmentId: "second", projectId: "broken-project", level: "full" }] }) as never;
    const user = createUserInventory("/api", f.fetcher as typeof fetch, async () => hosts, principal, (host, state) => observed.set(host, state), async () => new Map());
    expect((await user.listStacks()).map((s) => s.id)).toEqual(["first~project", "second~project"]);
    expect(observed.get("second")).toMatchObject({ unreadable: { stacks: { count: 1 } } });
    const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input) => new URL(String(input), "https://hub.example.com").pathname === "/api/environments/second/projects/broken-project/runtime"
      ? new Response(JSON.stringify({ success: true, data: { id: "broken-project", status: "running" } }), { headers: { "Content-Type": "application/json", "X-Docklet-Observed-At": "2026-10-17T12:00:00Z" } })
      : original(input));
    await expect(user.getStack("second~broken-project")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
  });

  it("reports an assigned stack detail whose own name is not text as unreadable", async () => {
    const f = resourceFixture();
    const hosts = await f.adapter.listHosts();
    const principal = () => ({ id: "u", username: "user", requiresPasswordChange: false, isAdmin: false, locale: null, assignments: [{ environmentId: "second", projectId: "project", level: "restricted" }] }) as never;
    const user = createUserInventory("/api", f.fetcher as typeof fetch, async () => hosts, principal, () => undefined, async () => new Map());
    const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input) => new URL(String(input), "https://hub.example.com").pathname === "/api/environments/second/projects/project/runtime"
      ? new Response(JSON.stringify({ success: true, data: { id: "project", name: 42, status: "running" } }), { headers: { "Content-Type": "application/json", "X-Docklet-Observed-At": "2026-10-17T12:00:00Z" } })
      : original(input));
    await expect(user.getStack("second~project")).rejects.toMatchObject({ outcome: { code: "get-project.unreadable" } });
  });
});
