// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original actual navigation and dense Dashboard consumers against a six-connection HTTP/1.1 scheduler.

import { act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useMetrics } from "../live";
import { useNavCounts } from "@/app/shell/use-nav-counts";
import { useActionsQuery, useContainers, useHosts, useImages, useNetworkSummaries, useSecurity, useStacks, useVolumes } from "../queries";
import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { renderWithApp } from "@/test/render";
import { hostReadPool } from "./host-read-pool.test-support";

const hooks = { stacks: useStacks, containers: useContainers, images: useImages, volumes: useVolumes, networks: useNetworkSummaries };
afterEach(() => vi.useRealTimers());

function Probe() {
  const counts = useNavCounts(); useActionsQuery();
  const hosts = useHosts(); const containers = useContainers(); const security = useSecurity();
  const stacks = useStacks(), images = useImages(), volumes = useVolumes(), networks = useNetworkSummaries();
  const batches = { stacks: stacks.data, containers: containers.data, images: images.data, volumes: volumes.data, networks: networks.data };
  return <>
    <p data-testid="counts">{JSON.stringify(counts)}</p>
    {Object.entries(batches).map(([name, rows]) => <p key={name} data-testid={name}>{["0", "edge", "direct"].map((host) => `${host}:${rows?.filter((row) => row.hostId === host).length ?? 0}`).join(",")}</p>)}
    <p data-testid="security">{security.data?.unavailable?.map((h) => h.hostId).sort().join(",")}</p>
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

it("D4-4 recovers the mounted lists within 15 s after release with no catalogue or identity change", async () => {
  vi.useFakeTimers(); const pool = hostReadPool();
  const view = renderWithApp(<Probe />, pool.adapter);
  await act(() => vi.advanceTimersByTimeAsync(35_000));
  expect(view.getByTestId("containers").textContent).toBe("0:201,edge:201,direct:0");
  pool.resume();
  await act(() => vi.advanceTimersByTimeAsync(15_000));
  for (const name of Object.keys(hooks)) expect(view.getByTestId(name).textContent).toBe("0:201,edge:201,direct:201");
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
  return <Probe />;
}

it("C1-1d completes dense Dashboard host inventories within 30 s with slow metrics pages and six physical connections", async () => {
  vi.useFakeTimers(); const pool = hostReadPool({ dense: true });
  const view = renderWithApp(<DenseDashboard />, pool.adapter);
  await act(() => vi.advanceTimersByTimeAsync(29_900));
  expect(view.getByTestId("stacks").textContent).toBe("0:21,edge:201,direct:201");
  expect(view.getByTestId("containers").textContent).toBe("0:336,edge:201,direct:201");
  expect(view.getByTestId("images").textContent).toBe("0:336,edge:201,direct:201");
  expect(view.getByTestId("security").textContent).toBe("");
  expect(pool.maximum).toBe(6);
  for (const resource of ["containers", "images"]) expect(pool.sent.some((path) => path.includes(`/0/${resource}?start=300`))).toBe(true);
  expect(pool.sent.some((path) => path.includes("/0/containers?") && path.includes("sort=cpuUsage"))).toBe(true);
});
