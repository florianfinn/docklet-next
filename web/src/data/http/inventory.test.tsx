// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original multi-host inventory contract tests.

import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "@/api/generated/schema";
import { setAppLocale } from "@/app/i18n/locale-store";
import { StackUpdateIndicator } from "@/shared/components/updates/update-indicator";
import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { useHostScope, useVisibleContainers, useVisibleStacks } from "@/shared/hooks/use-host-scope";
import { setPreference } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";
import { createHttpAdapter } from "./http-adapter";
import { mapActivityDetail, mapEvent } from "./map-dto";
import { stackStatus } from "@/domain/status";
import { inventoryId, mapContainer, type ContainerRow, type ProjectRow } from "./inventory-mapping";

const at = "2026-10-17T12:00:00Z";
const later = "2026-10-17T12:05:00Z";
const project: ProjectRow = { id: "project", name: "app", status: "running", isArchived: false, runningCount: 1, serviceCount: 1, path: "/srv/projects/app", createdAt: at, updatedAt: at, tags: [{ name: "example", color: "blue", sources: ["ui"] }], runtimeServices: [{ name: "web", image: "example:1.0.0", status: "running", containerId: "web" }] };
const container: ContainerRow = { id: "web", names: ["/web"], image: "example:1.0.0", imageId: "example", command: "serve", created: 0, ports: [{ privatePort: 80, publicPort: 8080, type: "tcp" }], labels: { "com.docker.compose.project": "app", "com.docker.compose.service": "web" }, state: "running", status: "Up (healthy)", hostConfig: { restartPolicy: "unless-stopped", memory: 104857600 }, networkSettings: { networks: {} }, mounts: [], autoUpdateEnabled: false };

function fixture() {
  const state = { reason: "", observedAt: at, rows: [container], projects: [project], hostStatus: "online", malformed: false };
  const json = (data: unknown, page = false, status = 200, observation = false) => new Response(JSON.stringify(status === 200 ? { success: true, data, ...(page ? { pagination: { totalItems: Array.isArray(data) ? data.length : 0 } } : {}) } : { success: false, code: data }), { status, headers: { "Content-Type": "application/json", ...(observation ? { "X-Docklet-Observed-At": state.observedAt, ...(state.reason ? { "X-Docklet-Host-Reason": state.reason } : {}) } : {}) } });
  const fetcher = vi.fn(async (input: string | URL | Request) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname === "/api/capabilities") return json(["listHosts", "listStacks", "getStack", "listContainers"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: "hub", id: "0" } })));
    if (url.pathname === "/api/environments") return json(["first", "second"].map((id) => ({ id, name: id, apiUrl: "http://agent.example.com", enabled: true, status: state.hostStatus, isEdge: false } satisfies components["schemas"]["EnvironmentEnvironment"])), true);
    if (url.pathname.endsWith("/projects/project")) return json({ ...project, composeContent: "name: app\nservices: {}", envContent: "EXAMPLE=value" }, false, 200, true);
    if (url.pathname.endsWith("/projects")) return json(state.projects, true, 200, true);
    if (url.pathname.endsWith("/containers")) return json(state.malformed ? [{ id: "broken" }] : state.rows, true, 200, true);
    return json("not-found", false, 404);
  });
  return { state, fetcher, adapter: createHttpAdapter({ fetch: fetcher as typeof fetch }) };
}

afterEach(() => { setPreference("hostSelection", []); setAppLocale("de"); });

describe("host inventory HTTP reads", () => {
  it("hides labeled agent stacks and containers without hiding a user name", async () => {
    const f = fixture();
    f.state.rows.push({ ...container, id: "agent", labels: { "com.docker.compose.project": "system", "com.docker.compose.service": "agent", "io.docklet.system": "agent" } });
    f.state.projects.push({ ...project, id: "system", name: "system" }, { ...project, id: "user", name: "docklet-agent" });
    expect((await f.adapter.listStacks()).map((item) => item.name)).toEqual(["app", "docklet-agent", "app", "docklet-agent"]);
    expect((await f.adapter.listContainers()).map((item) => item.name)).toEqual(["web", "web"]);
  });

  it.each([
    [{ "com.docker.compose.project": "app", "com.docker.compose.service": "web" }, "compose"],
    [{}, "standalone"],
    [{ "com.docker.compose.project": "app" }, "unknown"],
  ] as const)("maps origin and restrictions: %s → %s", (labels, origin) => {
    const mapped = mapContainer({ ...container, labels }, "first", [project], { stale: false });
    expect(mapped.origin).toBe(origin);
    expect(mapped.restrictions?.recreate).toBe(origin !== "standalone");
    expect(mapped.ports).toEqual([{ host: "8080", container: "80", protocol: "tcp" }]);
  });

  it("aggregates two hosts with collision-free IDs and filters both pages to one host", async () => {
    const f = fixture();
    const stacks = await f.adapter.listStacks();
    expect(stacks.map((item) => item.id)).toEqual(["first~project", "second~project"]);
    expect(stacks[0]?.services[0]).toMatchObject({ name: "web", state: "running", health: "healthy", restartPolicy: "unless-stopped", memLimitMb: 100 });
    function Probe() {
      const stacks = useVisibleStacks(); const containers = useVisibleContainers();
      return <><p data-testid="stacks">{stacks.visible.map((item) => item.id).join(",")}</p><p data-testid="containers">{containers.visible.map((item) => item.id).join(",")}</p></>;
    }
    setPreference("hostSelection", ["second"]);
    renderWithApp(<Probe />, f.adapter);
    await waitFor(() => expect(screen.getByTestId("stacks").textContent).toBe("second~project"));
    await waitFor(() => expect(screen.getByTestId("containers").textContent).toBe("second~web"));
    act(() => setPreference("hostSelection", []));
    await waitFor(() => expect(screen.getByTestId("stacks").textContent).toBe("first~project,second~project"));
  });

  it("shows stale values with the original timestamp and becomes fresh on reconnect", async () => {
    const f = fixture();
    await f.adapter.listStacks();
    f.state.reason = "agent.unavailable";
    const stale = await f.adapter.listStacks();
    expect(stale[0]?.inventory).toEqual({ stale: true, observedAt: at, reason: "agent.unavailable" });
    renderWithApp(<InventoryNotice hosts={await f.adapter.listHosts()} items={stale} />);
    expect(screen.getByRole("status").textContent?.includes("Stand")).toBe(true);
    expect(screen.getByRole("status").textContent?.includes("nicht erreichbar")).toBe(true);
    f.state.reason = ""; f.state.observedAt = later;
    const fresh = await f.adapter.listStacks();
    expect(fresh[0]?.inventory).toMatchObject({ stale: false, observedAt: later });
    expect((await f.adapter.listHosts())[0]?.inventory?.reason).toBeUndefined();
  });

  it.each(["agent.update-required", "agent.invalid-token"])("keeps the distinct host state reason %s", async (reason) => {
    const f = fixture(); f.state.reason = reason;
    expect((await f.adapter.listContainers())[0]?.inventory?.reason).toBe(reason);
    expect((await f.adapter.listHosts())[0]?.inventory?.reason).toBe(reason);
  });

  it("reads a scoped stack detail and rejects malformed list responses", async () => {
    const f = fixture();
    const detail = await f.adapter.getStack(inventoryId("second", "project"));
    expect(detail).toMatchObject({ hostId: "second", labels: ["example"], compose: "name: app\nservices: {}", env: "EXAMPLE=value", deployments: [], restorePoints: [] });
    f.state.malformed = true;
    expect(await f.adapter.listContainers()).toEqual([]);
    expect((await f.adapter.listHosts()).every((h) => h.inventory?.reason === "inventory.mapping-error")).toBe(true);
  });

  it("requires explicit command capabilities and never posts an ungranted inventory mutation", async () => {
    const f = fixture();
    await expect(f.adapter.runContainerCommand("first~web", "restart")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    await expect(f.adapter.runStackCommand("first~project", "start")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    await expect(f.adapter.installContainerUpdate("first~web")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    await expect(f.adapter.installStackUpdates("first~project")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    expect(f.fetcher.mock.calls.map(([input]) => String(input))).toEqual([
      "/api/capabilities?kind=container&id=first%7Eweb",
      "/api/capabilities?kind=stack&id=first%7Eproject",
    ]);
  });

  it.each(["de", "en-US"] as const)("renders host state without a retained snapshot in %s", (locale) => {
    setAppLocale(locale);
    const f = fixture();
    function Probe() { const hosts = useHostScope(); return <InventoryNotice hosts={hosts.visible} items={[]} />; }
    f.state.hostStatus = "update-required";
    return (async () => {
      renderWithApp(<Probe />, f.adapter);
      await waitFor(() => expect(screen.getByRole("status").textContent?.includes(locale === "de" ? "Agent-Update erforderlich" : "Agent update required")).toBe(true));
    })();
  });
});

it("does not report unchecked inventory updates as current", async () => {
  const f = fixture();
  const [stack] = await f.adapter.listStacks();
  if (!stack) throw new Error("fixture missing");
  const view = renderWithApp(<StackUpdateIndicator name={stack.name} services={stack.services} />);
  expect(view.container.textContent?.includes("Aktuell")).toBe(false);
  expect(view.container.textContent?.includes("geprüft")).toBe(true);
});

it("keeps activity and event resource links in the inventory host namespace", () => {
  const activity: components["schemas"]["ActivityActivity"] = { id: "run", environmentId: "second", type: "project_deploy", status: "success", resourceType: "project", resourceId: "project", startedAt: at, createdAt: at };
  expect(mapActivityDetail({ activity, outputTruncated: false, messages: [], output: [], stepTrace: [] }).stackId).toBe(inventoryId("second", "project"));
  expect(mapActivityDetail({ activity: { ...activity, resourceType: "container", resourceId: "web" }, outputTruncated: false, messages: [], output: [], stepTrace: [] }).containerId).toBe(inventoryId("second", "web"));
  const event: components["schemas"]["EventEvent"] = { id: "event", type: "project.deploy", messageCode: "project.deploy", severity: "success", title: "Example", timestamp: at, createdAt: at, environmentId: "second", resourceType: "project", resourceId: "project" };
  expect(mapEvent(event).stackId).toBe(inventoryId("second", "project"));
  expect(mapEvent({ ...event, environmentId: undefined }).stackId).toBeUndefined();
});

it("D1 preserves project detail counts, state and path when runtime services are omitted", async () => {
  const f = fixture(); const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input) => {
    const response = await original(input);
    if (String(input).includes("/projects/project")) {
      const body = await response.json(); delete body.data.runtimeServices;
      return new Response(JSON.stringify(body), { headers: response.headers });
    }
    return response;
  });
  const detail = await f.adapter.getStack("second~project");
  expect(detail).toMatchObject({ path: "/srv/projects/app", serviceCount: 1, runningCount: 1, reportedStatus: "running", deployHistoryUnavailable: true });
  expect(detail.services.map((s) => [s.name, s.state])).toEqual([["web", "running"]]);
  expect(stackStatus(detail, undefined)).toBe("running");
});

it("D6 merges address-family port duplicates while preserving protocol and port distinctions", () => {
  const ports = [{ privatePort: 80, publicPort: 8080, type: "tcp", ip: "0.0.0.0" }, { privatePort: 80, publicPort: 8080, type: "tcp", ip: "::" }, { privatePort: 80, publicPort: 8080, type: "udp" }, { privatePort: 80, publicPort: 8081, type: "tcp" }];
  const mapped = mapContainer({ ...container, ports }, "first", [project], { stale: false });
  expect(mapped.ports).toEqual([{ host: "8080", container: "80", protocol: "tcp" }, { host: "8080", container: "80", protocol: "udp" }, { host: "8081", container: "80", protocol: "tcp" }]);
});
