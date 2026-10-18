// Source: docklet-next/issues/83
// Revision: b15b60e94f1b0bd361d32a03477fdf82647ad108
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original resource inventory contract tests.

import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { imageUsage } from "@/domain/images";
import type { MetricsSnapshot } from "@/domain/model";
import ImagesPage from "@/features/images/ImagesPage";
import VolumePage from "@/features/volumes/VolumePage";
import VolumesPage from "@/features/volumes/VolumesPage";
import NetworksPage from "@/features/networks/NetworksPage";
import { OpsSummary } from "@/features/dashboard/OpsSummary";
import { HostPulse } from "@/features/dashboard/HostPulse";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { setPreference } from "@/shared/preferences/preferences-store";
import { renderRoutes, renderWithApp } from "@/test/render";
import { imageHref } from "@/features/images/image-links";
import { mapHostMetrics } from "./metrics";
import { at, resourceFixture } from "./resources.test-support";

afterEach(() => { vi.useRealTimers(); setAppLocale("de"); setPreference("hostSelection", []); });

it("maps image consumers from running and stopped containers, unused and dangling states", async () => {
  const { adapter } = resourceFixture();
  const images = await adapter.listImages();
  expect(images.map((i) => i.id)).toEqual(["first~sha256:user", "first~unused", "first~dangling", "second~sha256:user", "second~unused", "second~dangling"]);
  expect(images.slice(0, 3).map(imageUsage)).toEqual(["in-use", "unused", "dangling"]);
  expect(images[0]?.consumers).toMatchObject([{ containerId: "first~web", stackId: "first~project", state: "running" }, { containerId: "first~db", state: "stopped" }]);
  expect(images[0]).toMatchObject({ repository: "example", tag: "1.0.0", sizeMb: 100, aliases: ["example:stable"] });
});
it("maps volume consumers and unavailable sizes, plus scoped volume detail", async () => {
  const { adapter } = resourceFixture();
  const volumes = await adapter.listVolumes();
  expect(volumes.map((v) => v.id)).toEqual(["first~data", "second~data"]);
  expect(volumes[0]).toMatchObject({ stackId: "first~project", sizeUnavailable: true, consumers: [{ containerId: "first~web", target: "/data", readOnly: false, running: true }] });
  expect(await adapter.getVolume("second~data")).toMatchObject({ hostId: "second", backups: [] });
});
it("maps network membership and depends_on without traffic fields or cross-host edges", async () => {
  const { adapter } = resourceFixture();
  const graph = await adapter.getTopology();
  expect(graph.networks.map((n) => n.id)).toEqual(["first~net", "second~net"]);
  expect(graph.networks[0]).toMatchObject({ subnet: "192.0.2.0/24", internal: true, ipv6: true, stackId: "first~project" });
  expect(graph.attachments).toHaveLength(4);
  expect(graph.attachments[0]).toMatchObject({ containerId: "first~web", networkId: "first~net", ip: "192.0.2.2", ip6: "2001:db8::2" });
  expect(graph.dependencies).toEqual([{ from: "first~web", to: "first~db" }, { from: "second~web", to: "second~db" }]);
  expect(Object.keys(graph).sort()).toEqual(["attachments", "dependencies", "networks"]);
});
it("maps published ports per host from real container summaries", async () => {
  const { adapter } = resourceFixture();
  const containers = await adapter.listContainers();
  expect(containers.filter((c) => c.ports.length)).toMatchObject([{ hostId: "first", ports: [{ host: "8080", container: "80", protocol: "tcp" }] }, { hostId: "second", ports: [{ host: "8080", container: "80", protocol: "tcp" }] }]);
});
it.each(["listImages", "listVolumes", "getTopology"] as const)("retains %s timestamps and refreshes on reconnect", async (operation) => {
  const f = resourceFixture();
  await f.adapter[operation]();
  f.state.reason = "agent.unavailable";
  const retained = await f.adapter[operation]();
  const rows = Array.isArray(retained) ? retained : retained.networks;
  expect(rows[0]?.inventory).toEqual({ stale: true, reason: "agent.unavailable", observedAt: at });
  f.state.reason = ""; f.state.observedAt = "2026-10-17T12:05:00Z";
  const fresh = await f.adapter[operation]();
  expect((Array.isArray(fresh) ? fresh : fresh.networks)[0]?.inventory).toMatchObject({ stale: false, observedAt: f.state.observedAt });
});
it.each(["agent.update-required", "agent.invalid-token"])("keeps reason %s without a retained snapshot", async (reason) => {
  const f = resourceFixture(); f.state.reason = reason; f.state.noSnapshot = true;
  expect(await f.adapter.listImages()).toEqual([]);
  expect((await f.adapter.listHosts())[0]?.inventory).toEqual({ stale: true, reason });
});
it("marks malformed resource data with a host-local reason", async () => {
  const f = resourceFixture(); f.state.malformed = true;
  expect(await f.adapter.listImages()).toEqual([]);
  expect((await f.adapter.listHosts()).every((h) => h.inventory?.reason === "inventory.mapping-error")).toBe(true);
});
it("blocks all storage and image writes without sending a request", async () => {
  const f = resourceFixture();
  for (const promise of [f.adapter.pullImage({ hostId: "first", reference: "example:1" }), f.adapter.removeImage("first~unused"), f.adapter.pruneImages({ hostIds: ["first"], mode: "unused" }), f.adapter.deleteVolume("first~data"), f.adapter.removeNetwork("first~net"), f.adapter.connectNetwork({ containerId: "first~web", networkId: "first~net" })]) await expect(promise).rejects.toMatchObject({ outcome: { kind: "not-available" } });
  expect(f.fetcher).not.toHaveBeenCalled();
});
it.each([ImagesPage, VolumesPage, NetworksPage])("filters the rendered list page %s by host and shows retained time", async (Page) => {
  const f = resourceFixture(); f.state.reason = "agent.unavailable";
  setPreference("hostSelection", ["second"]);
  const view = renderWithApp(<Page />, f.adapter);
  await waitFor(() => expect(screen.getByRole("status").textContent?.includes("second")).toBe(true));
  expect(screen.getByRole("status").textContent?.includes("first")).toBe(false);
  expect(screen.getByRole("status").textContent?.includes("Stand")).toBe(true);
  if (Page === ImagesPage) expect(view.container.querySelectorAll("tbody tr").length).toBe(3);
  if (Page === VolumesPage) expect(view.container.querySelectorAll('a[href="/volumes/first~data"]').length).toBe(0);
  if (Page === NetworksPage) expect(screen.getAllByRole("radio", { name: /second/ }).length).toBe(1);
  act(() => setPreference("hostSelection", ["first"]));
  await waitFor(() => expect(screen.getByRole("status").textContent?.includes("first")).toBe(true));
  expect(screen.getByRole("status").textContent?.includes("second")).toBe(false);
});
it("maps measured host units, retains two hosts, then recovers from disconnect", async () => {
  vi.useFakeTimers();
  const f = resourceFixture(); const stopChanges = f.adapter.subscribeChanges(() => {}); let snapshot: MetricsSnapshot | undefined;
  const stop = f.adapter.subscribeMetrics((s) => { snapshot = s; });
  await vi.advanceTimersByTimeAsync(0);
  expect(f.webSocket).toHaveBeenCalledTimes(2);
  f.sample("first", 25); f.sample("second", 50);
  expect(snapshot?.simulated).toBe(false);
  expect(snapshot?.hosts.first).toMatchObject({ cpu: 25, mem: 25, disk: 25, cpuCores: 4, memTotalGb: 4, diskTotalGb: 8 });
  expect(snapshot?.hosts.second?.cpu).toBe(50);
  expect(snapshot?.containers["first~web"]).toEqual({ cpu: 2.5, memMb: 100 });
  const observedAt = snapshot?.hosts.first?.inventory?.observedAt;
  f.state.status = "offline"; f.source.onmessage?.({ data: JSON.stringify({ timestamp: at, channel: "environments" }), lastEventId: "" } as MessageEvent); await vi.advanceTimersByTimeAsync(5000);
  expect(snapshot?.hosts.first).toMatchObject({ cpu: 25, inventory: { stale: true, observedAt, reason: "agent.unavailable" } });
  f.state.status = "online"; f.source.onmessage?.({ data: JSON.stringify({ timestamp: at, channel: "environments" }), lastEventId: "" } as MessageEvent); await vi.advanceTimersByTimeAsync(5000); f.sample("first", 35);
  expect(snapshot?.hosts.first).toMatchObject({ cpu: 35, inventory: { stale: false } });
  stop(); expect([...f.sockets.values()].every((s) => vi.mocked(s.close).mock.calls.length > 0)).toBe(true);
  stopChanges();
});
it("rejects malformed metrics and keeps unknown disk/growth explicitly unavailable", () => {
  expect(() => mapHostMetrics({ cpuUsage: "bad" }, "first")).toThrow();
  expect(mapHostMetrics({ cpuUsage: 20, memoryUsage: 100, memoryTotal: 200, cpuCount: 2 }, "first")).toMatchObject({ diskUnavailable: true, growthUnavailable: true });
});
it("renders two dashboard host cards with distinct real measurements", async () => {
  const f = resourceFixture();
  function Probe() { const hosts = useHostScope(); return <>{hosts.visible.map((h) => <HostPulse key={h.id} host={h} stacks={1} containers={2} />)}</>; }
  renderWithApp(<Probe />, f.adapter);
  await waitFor(() => expect(f.webSocket).toHaveBeenCalledTimes(2));
  act(() => { f.sample("first", 25); f.sample("second", 50); });
  expect(screen.getAllByRole("article")).toHaveLength(2);
  expect(screen.getAllByRole("article")[0]?.textContent?.includes("25")).toBe(true);
  expect(screen.getAllByRole("article")[1]?.textContent?.includes("50")).toBe(true);
  act(() => setPreference("hostSelection", ["second"]));
  expect(screen.getAllByRole("article")).toHaveLength(1);
  expect(screen.getByRole("article").textContent?.includes("second")).toBe(true);
});

it("opens the image details using the host-scoped Docker ID", async () => {
  const f = resourceFixture();
  const [container] = await f.adapter.listContainers();
  expect(imageHref(container!)).toBe("/images?image=first~sha256%3Auser");
});
it.each(["de", "en-US"] as const)("shows retained inventory notices in %s", async (locale) => {
  const f = resourceFixture(); f.state.reason = "agent.update-required"; setAppLocale(locale);
  renderWithApp(<ImagesPage />, f.adapter);
  await waitFor(() => expect(screen.getByRole("status").textContent?.includes(locale === "de" ? "Agent-Update erforderlich" : "Agent update required")).toBe(true));
  expect(screen.getByRole("status").textContent?.includes(locale === "de" ? "Stand" : "as of")).toBe(true);
});
it("leaves measurements absent until the first observed sample and blocks incompatible-host streams", async () => {
  vi.useFakeTimers(); const f = resourceFixture(); f.state.status = "update-required";
  let snapshot: MetricsSnapshot | undefined;
  const stop = f.adapter.subscribeMetrics((s) => { snapshot = s; });
  await vi.advanceTimersByTimeAsync(0);
  expect(f.webSocket).not.toHaveBeenCalled(); expect(snapshot?.hosts).toEqual({}); stop();
});

it("does not claim unchecked updates or unavailable security are healthy", async () => {
  const f = resourceFixture(); const [hosts, stacks, containers] = await Promise.all([f.adapter.listHosts(), f.adapter.listStacks(), f.adapter.listContainers()]);
  const view = renderWithApp(<OpsSummary hosts={hosts} allHostsCount={2} stacks={stacks} containers={containers} attention={[]} attentionText={() => ""} updates={0} critical={undefined} runningActions={0} editing={false} animate={false} duration={0} />, f.adapter);
  expect(view.container.textContent?.includes("Alle Images aktuell")).toBe(false);
  expect(view.container.textContent?.includes("noch nicht geprüft")).toBe(true);
  expect(view.container.textContent?.includes("Sicherheitsstatus unbekannt")).toBe(true);
});

it("keeps bind identities collision-free and stable through route decoding", async () => {
  const f = resourceFixture(); f.state.bind = true;
  const binds = (await f.adapter.listVolumes()).filter((v) => v.kind === "bind");
  expect(binds).toHaveLength(2);
  expect(binds[0]?.id).not.toBe(binds[1]?.id);
  const id = binds[0]!.id;
  expect(decodeURIComponent(id)).toBe(id);
  expect(await f.adapter.getVolume(decodeURIComponent(id))).toMatchObject({ kind: "bind", source: "/srv/example~data", sizeUnavailable: true, consumers: [{ target: "/data", readOnly: false }] });
});

it("renders a bind detail route without fabricated size or non-finite host usage", async () => {
  const f = resourceFixture(); f.state.bind = true;
  const bind = (await f.adapter.listVolumes()).find((v) => v.kind === "bind")!;
  const view = renderRoutes([{ path: "/volumes/:volumeId", element: <VolumePage /> }], [`/volumes/${bind.id}`], f.adapter);
  await waitFor(() => expect(screen.getByRole("heading", { name: "/srv/example~data" }).textContent).toBe("/srv/example~data"));
  expect(view.container.textContent?.includes("NaN")).toBe(false);
  expect(view.container.textContent?.includes("Unbekannt")).toBe(true);
});

it("displays Docker built-in and plugin drivers without rejecting a healthy host", async () => {
  const f = resourceFixture();
  const base = f.state.networks[0]!;
  f.state.networks = [["bridge", "bridge"], ["host", "host"], ["none", "null"], ["custom", "vendor/plugin"]].map(([name, driver]) => ({ ...base, id: name!, name: name!, driver: driver! }));
  const graph = await f.adapter.getTopology();
  expect(graph.networks.filter((n) => n.hostId === "first").map((n) => [n.name, n.driver, n.builtIn])).toEqual([["bridge", "bridge", true], ["host", "host", true], ["none", "null", true], ["custom", "vendor/plugin", false]]);
});
