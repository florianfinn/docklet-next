// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original actual navigation and dense Dashboard consumers against a six-connection HTTP/1.1 scheduler. C2 #271: mounted stale-success recovery through containers and Dashboard consumers. M26 #462: explicit full-volume probe ownership after the shell switches to counts. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags.

import { act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useMetrics } from "../live";
import { useNavCounts } from "@/app/shell/use-nav-counts";
import { useActionsQuery, useEvents, useSchedule, useContainers, useHosts, useImages, useNetworkSummaries, useSecurity, useStacks, useVolumes } from "../queries";
import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { renderWithApp } from "@/test/render";
import { hostReadPool } from "./host-read-pool.test-support";

const hooks = { stacks: useStacks, containers: useContainers, images: useImages, volumes: useVolumes, networks: useNetworkSummaries };
afterEach(() => vi.useRealTimers());

function Probe() {
  // This full-inventory probe owns its volume read; preserve its original start order after the shell stopped mounting it.
  const volumes = useVolumes();
  const counts = useNavCounts(); useActionsQuery();
  const hosts = useHosts(); const containers = useContainers(); const security = useSecurity();
  const stacks = useStacks(), images = useImages(), networks = useNetworkSummaries();
  const batches = { stacks: stacks.data, containers: containers.data, images: images.data, volumes: volumes.data, networks: networks.data };
  return <>
    <p data-testid="counts">{JSON.stringify(counts)}</p>
    {Object.entries(batches).map(([name, rows]) => <p key={name} data-testid={name}>{["0", "edge", "direct"].map((host) => `${host}:${rows?.filter((row) => row.hostId === host).length ?? 0}`).join(",")}</p>)}
    <p data-testid="security" data-ready={security.isSuccess}>{security.data?.unavailable?.map((h) => h.hostId).sort().join(",")}</p>
    <p data-testid="direct-container">{containers.data?.filter((row) => row.hostId === "direct").map((row) => `${row.name}:${row.state}:${!!row.inventory?.stale}`).join(",")}</p>
    <p data-testid="direct-stale">{Object.entries({ stacks: stacks.data, images: images.data, volumes: volumes.data }).map(([name, rows]) => `${name}:${!!rows?.some((row) => row.hostId === "direct" && row.inventory?.stale)}`).join(",")}</p>
    <InventoryNotice hosts={hosts.data ?? []} items={containers.data ?? []} />
  </>;
}

it("D4-1 completes healthy multi-page batches with all nav readers and one stream while Direct is held", async () => {
  vi.useFakeTimers(); const pool = hostReadPool();
  const view = renderWithApp(<Probe />, pool.adapter);
  await act(() => vi.advanceTimersByTimeAsync(35_000));
  for (const name of Object.keys(hooks)) expect(view.getByTestId(name).textContent).toBe("0:201,edge:201,direct:0");
  expect(view.getByTestId("security").textContent).toBe("direct");
  expect(view.getByRole("status").textContent?.includes("direct")).toBe(true);
  expect(view.getByRole("status").textContent?.includes("edge")).toBe(false);
  expect(pool.maximum).toBe(6);
  expect(pool.maximumHosts.get("direct")).toBe(5);
  expect(pool.sent.some((path) => path.includes("/edge/containers?start=200"))).toBe(true);
});

function Containers() {
  const hosts = useHosts(), containers = useContainers();
  return <>
    <p data-testid="direct-container">{containers.data?.filter((row) => row.hostId === "direct").map((row) => `${row.name}:${row.state}:${!!row.inventory?.stale}`).join(",")}</p>
    <InventoryNotice hosts={hosts.data ?? []} items={containers.data ?? []} />
  </>;
}

it("C2-1 recovers retained HTTP 200 snapshot rows in the mounted containers list within 15 s of release", async () => {
  vi.useFakeTimers(); const pool = hostReadPool({ hubBound: true, hubSnapshot: true });
  const view = renderWithApp(<Containers />, pool.adapter);
  await act(() => vi.advanceTimersByTimeAsync(25_000));
  expect(view.getByTestId("direct-container").textContent).toBe("snapshot-0:stopped:true");
  expect(view.getByRole("status").textContent?.includes("direct")).toBe(true);
  pool.resume();
  await act(() => vi.advanceTimersByTimeAsync(15_000));
  expect(view.getByTestId("direct-container").textContent).toBe("fresh-0:running:false");
  expect(view.getByRole("status").textContent).toBe("");
  const reads = pool.sent.filter((path) => /\/(projects|containers)\?/.test(path)).length;
  await act(() => vi.advanceTimersByTimeAsync(60_000));
  expect(pool.sent.filter((path) => /\/(projects|containers)\?/.test(path)).length).toBe(reads);
});

it("D4-4 recovers the mounted lists within 15 s after release with no catalogue or identity change", async () => {
  vi.useFakeTimers(); const pool = hostReadPool();
  const imageCompletions: number[] = [];
  const original = pool.adapter.listImages;
  pool.adapter.listImages = async (...args) => { const rows = await original(...args); imageCompletions.push(Date.now()); return rows; };
  const view = renderWithApp(<Probe />, pool.adapter);
  await act(() => vi.advanceTimersByTimeAsync(35_000));
  expect(view.getByTestId("containers").textContent).toBe("0:201,edge:201,direct:0");
  const releasedAt = Date.now(); pool.resume();
  await act(() => vi.advanceTimersByTimeAsync(15_000));
  expect(imageCompletions).toHaveLength(2);
  expect(imageCompletions[1]! - releasedAt).toBeLessThanOrEqual(15_000);
  // The scope has a separate budget. Check completion before yielding the notification tick at its boundary.
  await act(() => vi.advanceTimersByTimeAsync(1));
  expect(Object.fromEntries(Object.keys(hooks).map((name) => [name, view.getByTestId(name).textContent]))).toEqual(Object.fromEntries(Object.keys(hooks).map((name) => [name, "0:201,edge:201,direct:201"])));
  expect(view.getByRole("status").textContent).toBe("");
});

it("D4-5 performs no periodic inventory reads after every batch succeeds", async () => {
  vi.useFakeTimers(); const pool = hostReadPool(); pool.resume();
  const view = renderWithApp(<Probe />, pool.adapter);
  await act(() => vi.advanceTimersByTimeAsync(15_000));
  expect(view.getByTestId("containers").textContent).toBe("0:201,edge:201,direct:201");
  const reads = () => pool.sent.filter((path) => /\/(projects|containers|images|volumes|networks)\?/.test(path)).length;
  const initial = reads(); expect(initial > 0).toBe(true);
  await act(() => vi.advanceTimersByTimeAsync(60_000));
  expect(reads()).toBe(initial);
});

function DenseDashboard() {
  useMetrics((snapshot) => snapshot?.containers);
  useEvents(); useSchedule(["0", "direct", "edge"]);
  return <Probe />;
}

it("C2-2 recovers the mounted Dashboard's snapshot rows and state within 15 s of release", async () => {
  vi.useFakeTimers(); const pool = hostReadPool({ holdDirect: true, hubBound: true, hubSnapshot: true });
  const view = renderWithApp(<DenseDashboard />, pool.adapter);
  await act(() => vi.advanceTimersByTimeAsync(35_000));
  expect(view.getByTestId("containers").textContent).toBe("0:201,edge:201,direct:1");
  expect(view.getByTestId("direct-container").textContent).toBe("snapshot-0:stopped:true");
  expect(view.getByRole("status").textContent?.includes("direct")).toBe(true);
  // The snapshot fixture holds the stack and volume reads; images arrive fresh through their own read.
  for (const name of ["stacks:true", "volumes:true"]) expect(view.getByTestId("direct-stale").textContent?.includes(name)).toBe(true);
  pool.resume();
  await act(() => vi.advanceTimersByTimeAsync(15_000));
  expect(view.getByTestId("direct-container").textContent).toBe("fresh-0:running:false");
  for (const name of Object.keys(hooks)) expect(view.getByTestId(name).textContent?.endsWith("direct:1")).toBe(true);
  expect(view.getByTestId("direct-stale").textContent).toBe("stacks:false,images:false,volumes:false");
  expect(view.getByRole("status").textContent).toBe("");
  expect(view.getByTestId("security").textContent).toBe("");
});

it("C1-1d completes dense Dashboard host inventories within 30 s with slow metrics pages and six physical connections", async () => {
  vi.useFakeTimers(); const pool = hostReadPool({ dense: true });
  const view = renderWithApp(<DenseDashboard />, pool.adapter);
  await act(() => vi.advanceTimersByTimeAsync(29_900));
  expect(view.getByTestId("stacks").textContent).toBe("0:21,edge:201,direct:201");
  expect(view.getByTestId("containers").textContent).toBe("0:336,edge:201,direct:201");
  expect(view.getByTestId("images").textContent).toBe("0:336,edge:201,direct:201");
  expect(view.getByTestId("security").getAttribute("data-ready")).toBe("true");
  expect(view.getByTestId("security").textContent).toBe("");
  expect(pool.maximum).toBe(6);
  for (const resource of ["containers", "images"]) expect(pool.sent.some((path) => path.includes(`/0/${resource}?start=300`))).toBe(true);
  expect(pool.sent.some((path) => path.includes("/0/containers?") && path.includes("sort=cpuUsage"))).toBe(true);
});

it("C1R1-1 keeps a paused Direct host restrained after the hub answers it unavailable, so dense H completes", async () => {
  vi.useFakeTimers(); const pool = hostReadPool({ dense: true, holdDirect: true, hubBound: true });
  const view = renderWithApp(<DenseDashboard />, pool.adapter);
  // Reads Direct started before its first unavailable answer drain within about two seconds of it.
  await act(() => vi.advanceTimersByTimeAsync(22_500));
  let directAfterAnswer = 0;
  for (let t = 0; t < 7; t++) { await act(() => vi.advanceTimersByTimeAsync(1_000)); directAfterAnswer = Math.max(directAfterAnswer, pool.activeHosts.get("direct") ?? 0); }
  expect(directAfterAnswer <= 1).toBe(true);
  expect(view.getByTestId("stacks").textContent).toBe("0:21,edge:201,direct:0");
  expect(view.getByTestId("containers").textContent).toBe("0:336,edge:201,direct:0");
});
