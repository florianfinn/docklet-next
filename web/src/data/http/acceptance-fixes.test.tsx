// Source: docklet-next/issues/85
// Revision: 37ddf907edabe127fe061523c7aba1c764d87d85
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original acceptance correction and regression coverage.

import { useQueryClient } from "@tanstack/react-query";
import type { ContainerInfo } from "@/domain/model";
import { queryKeys } from "@/data/query-keys";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { useContainers, useImages, useStacks, useTopology, useVolumes } from "@/data/queries";
import { HostCard } from "@/features/hosts/HostCard";
import ImagesPage from "@/features/images/ImagesPage";
import ContainersPage from "@/features/containers/ContainersPage";
import VolumesPage from "@/features/volumes/VolumesPage";
import NetworksPage from "@/features/networks/NetworksPage";
import { ContainerMemory } from "@/shared/components/container-memory";
import { LoadView } from "@/shared/components/container-load";
import { renderWithApp } from "@/test/render";
import { resourceFixture } from "./resources.test-support";

afterEach(() => { vi.useRealTimers(); setAppLocale("de"); localStorage.clear(); });

const hooks = [useStacks, useContainers, useImages, useVolumes, useTopology];
it.each(hooks)("D2 renders the online host within 1.5s while another host remains pending (%s)", async (hook) => {
  vi.useFakeTimers(); const f = resourceFixture(); const original = f.fetcher.getMockImplementation()!;
  const blocked = new Promise<void>((resolve) => setTimeout(resolve, 10_000));
  f.fetcher.mockImplementation(async (input) => {
    if (String(input).includes("/environments/second/")) await blocked;
    return original(input);
  });
  function Probe() {
    const query = hook(); const data = query.data;
    const rows = Array.isArray(data) ? data : data?.networks;
    return <p>{query.isPending ? "pending" : rows?.map((row) => row.hostId).join(",")}</p>;
  }
  const view = renderWithApp(<Probe />, f.adapter);
  await act(() => vi.advanceTimersByTimeAsync(100));
  await act(() => vi.advanceTimersByTimeAsync(100));
  expect(view.container.textContent).toContain("first");
  expect(view.container.textContent).not.toContain("second");
  await act(() => vi.advanceTimersByTimeAsync(10_000));
  expect(view.container.textContent).toContain("second");
});

it.each([ImagesPage, VolumesPage, NetworksPage])("D4 renders stack names while links retain opaque IDs (%s)", async (Page) => {
  const f = resourceFixture();
  const view = renderWithApp(<Page />, f.adapter);
  await waitFor(() => expect(view.container.textContent).toContain("app"));
  expect(view.container.textContent).not.toContain("first~project");
  expect(view.container.textContent).not.toContain("second~project");
  if (Page !== NetworksPage) expect(view.container.querySelector('a[href="/stacks/first~project"]') !== null).toBe(true);
  const graph = await f.adapter.getTopology();
  expect(graph.networks[0]?.stackName).toBe("app");
});

it("D3 permits removing a registered host with workloads and explains what keeps running", async () => {
  const adapter = createDemoAdapter(); const [host] = await adapter.listHosts(); if (!host) throw new Error("fixture missing");
  renderWithApp(<HostCard host={host} system={undefined} stacks={2} containers={3} allIds={[host.id]} />, adapter);
  const button = await screen.findByRole("button", { name: "Entfernen" });
  await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false));
  await userEvent.click(button);
  const dialog = await screen.findByRole("alertdialog");
  expect(dialog.textContent).toContain("Container und Stacks laufen auf dem Host weiter");
  expect(dialog.textContent).toContain("Hub-Registrierung");
});

it("D7 never calls live image data synthetic", async () => {
  const f = resourceFixture(); const view = renderWithApp(<ImagesPage />, f.adapter);
  await waitFor(() => expect(view.container.textContent).toContain("6 von 6"));
  expect(view.container.textContent).not.toMatch(/synthetisch|synthetic/i);
});

it("D8 shows an authenticated report without a contradictory missing-agent message or empty version suffix", async () => {
  const adapter = createDemoAdapter(); const [base] = await adapter.listHosts(); if (!base) throw new Error("fixture missing");
  const host = { ...base, agentVersion: "0.1.0", dockerVersion: "", agentReport: { reportedAt: "2026-10-17T12:00:00Z", projectDirOwner: { uid: 1000, gid: 1000, name: "1000:1000" }, projectDirWritable: true, note: "ok" as const, simulated: false } };
  const view = renderWithApp(<HostCard host={host} system={undefined} stacks={1} containers={1} allIds={[host.id]} />, adapter);
  expect(view.container.textContent).toContain("1000:1000");
  expect(view.container.textContent).not.toContain("Noch kein Agent gemeldet");
  expect(view.container.textContent).not.toContain("0.1.0 /");
  expect(view.container.textContent).not.toContain("Agent / Docker");
});

it("D9 shows memory usage without a limit or percent for unbounded containers in tiles and rings", () => {
  const view = renderWithApp(<><ContainerMemory usedMb={12} limitMb={0} /><LoadView values={{ cpu: 5, memMb: 12 }} memLimitMb={0} display="rings" /></>);
  expect(view.container.textContent).toContain("12 MB");
  expect(view.container.textContent).not.toContain("/ 0 MB");
  expect(screen.queryByRole("meter", { name: "RAM" }) === null).toBe(true);
  expect(screen.getByRole("img", { name: /RAM: 12 MB/ }) !== null).toBe(true);
  view.unmount();
  renderWithApp(<ContainerMemory usedMb={12} limitMb={100} />);
  expect(screen.getByRole("meter", { name: "RAM" }).getAttribute("aria-valuenow")).toBe("12");
});

it("D10 budgets the containers page and shares environments queries across every row and polling reader", async () => {
  vi.useFakeTimers(); const f = resourceFixture();
  const view = renderWithApp(<ContainersPage />, f.adapter);
  await act(() => vi.advanceTimersByTimeAsync(100));
  expect(within(view.container).queryAllByText("web").length).toBeGreaterThan(0);
  const loadRequests = f.fetcher.mock.calls.length;
  const environments = () => f.fetcher.mock.calls.filter(([url]) => /\/api\/environments\?/.test(String(url))).length;
  expect(loadRequests).toBeLessThanOrEqual(40);
  expect(environments()).toBe(1);
  const initial = environments();
  await act(() => vi.advanceTimersByTimeAsync(30_000));
  const steady = environments() - initial;
  expect(steady).toBeLessThanOrEqual(4);
  console.log(`D10 measured: load=${loadRequests}; load environments=${initial}; steady environments/30s=${steady}`);
});

it("D2 ignores partial batches from a cancelled inventory read", async () => {
  const f = resourceFixture(); const rows = await f.adapter.listContainers(); const original = rows[0];
  if (!original) throw new Error("fixture missing");
  const pending: { publish: (items: typeof rows) => void; finish: (items: typeof rows) => void }[] = [];
  f.adapter.listContainers = (publish = () => {}) => new Promise((finish) => { pending.push({ publish, finish }); });
  let cachedName: () => string | undefined = () => undefined;
  function Probe() { const q = useContainers(); const client = useQueryClient(); cachedName = () => client.getQueryData<ContainerInfo[]>(queryKeys.containers)?.[0]?.name; return <><p>{q.data?.[0]?.name}</p><button onClick={() => void q.refetch()}>Refresh</button></>; }
  const view = renderWithApp(<Probe />, f.adapter);
  await waitFor(() => expect(pending.length).toBe(1));
  act(() => pending[0]!.publish([{ ...original, name: "old" }]));
  await waitFor(() => expect(view.container.textContent).toContain("old"));
  act(() => screen.getByRole("button", { name: "Refresh" }).click());
  await waitFor(() => expect(pending.length).toBe(2));
  await act(async () => { pending[1]!.publish([{ ...original, name: "new" }]); pending[1]!.finish([{ ...original, name: "new" }]); });
  await waitFor(() => expect(view.container.textContent).toContain("new"));
  await act(async () => { pending[0]!.publish([{ ...original, name: "old" }]); pending[0]!.finish([{ ...original, name: "old" }]); });
  expect(cachedName()).toBe("new");
  await waitFor(() => expect(view.container.textContent).toContain("new"));
  expect(view.container.textContent).not.toContain("old");
});
