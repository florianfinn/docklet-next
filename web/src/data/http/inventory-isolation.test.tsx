// Source: docklet-next/issues/600
// Revision: 393e645a79236449c35cb8e1a4827c83bbf45da0
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original per-row isolation tests for every host projection, the visible unreadable state and the assigned-user reader.

// #600: one row that cannot be mapped never empties a host. One isolation test per host projection, plus the
// build-only service from the hub's own bytes (testdata/build-only-project.json, written by
// backend/internal/project/build_only_web_fixture_test.go).

import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
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
    expect((await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.unreadable?.volumes?.count).toBe(1);
  });

  it("leaves out only a malformed network and keeps the attachments of the others", async () => {
    const f = isolationFixture("second", ["networks", "containers"]);
    const graph = await f.adapter.getTopology();
    expect(graph.networks.filter((n) => n.hostId === "second").map((n) => n.name)).toEqual(["front"]);
    expect(graph.attachments.filter((a) => a.networkId === "second~net").map((a) => a.containerId)).toEqual(["second~web", "second~db"]);
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
});
