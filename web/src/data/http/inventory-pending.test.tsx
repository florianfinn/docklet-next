// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #556 tests: lists never report empty while a host in scope has not answered (stacks since the candidate run).

import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { useEffect, type ComponentType } from "react";
import { catalogs } from "@/app/i18n/catalog";
import { copyInventoryRecovery, inventoryHostsPending, inventoryRecovery, inventoryStructuralSharing } from "@/data/inventory-recovery";
import { useContainers, useImages, useStacks, useVolumes } from "@/data/queries";
import ContainersPage from "@/features/containers/ContainersPage";
import ImagesPage from "@/features/images/ImagesPage";
import StacksPage from "@/features/stacks/overview/StacksPage";
import VolumesPage from "@/features/volumes/VolumesPage";
import { useHostsPending } from "@/shared/hooks/use-host-scope";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";
import { resourceFixture } from "./resources.test-support";

beforeEach(() => { preferencesStore.set((p) => ({ ...p, containersLayout: "table", stacksLayout: "table", imagesLayout: "table", volumesLayout: "table" })); });
afterEach(() => { preferencesStore.reset(); localStorage.clear(); });

type Hook = () => { data?: readonly { hostId: string; name?: string }[]; refetch: () => Promise<unknown> };
const pages: [string, ComponentType, string, Hook][] = [
  ["containers", ContainersPage, catalogs.de.containers.emptyAll.title, useContainers as unknown as Hook],
  ["images", ImagesPage, catalogs.de.images.emptyAll.title, useImages as unknown as Hook],
  ["volumes", VolumesPage, catalogs.de.volumes.emptyAll.title, useVolumes as unknown as Hook],
  ["stacks", StacksPage, catalogs.de.stacks.emptyAll.title, useStacks as unknown as Hook],
];
type Answer = "rows" | "empty" | "failure";

/**
 * Reads of the second host wait while `held`; a release answers every waiting read. A read started while
 * `rename` is set returns renamed (obsolete) rows; the catalogue read can be made to fail transiently.
 */
function heldSecond(user = false) {
  const f = resourceFixture(), original = f.fetcher.getMockImplementation()!;
  const state = { held: true, rename: false, catalogueDown: false };
  const waiting: ((answer: Answer) => void)[] = [];
  f.fetcher.mockImplementation(async (input) => {
    const url = new URL(String(input), "https://hub.example.com");
    if (url.pathname.endsWith("/backups/storage")) return new Response(null, { status: 401 });
    // A simple user with stacks on both hosts reads through the assigned-stack reader.
    if (user && url.pathname === "/api/auth/me") return new Response(JSON.stringify({ success: true, data: { id: "viewer", username: "viewer", isAdmin: false, requiresPasswordChange: false, assignments: ["first", "second"].map((environmentId) => ({ environmentId, projectId: "project", level: "restricted" })) } }), { headers: { "content-type": "application/json" } });
    if (state.catalogueDown && url.pathname === "/api/environments") return new Response(JSON.stringify({ success: false, error: { code: "http.error" } }), { status: 503, headers: { "content-type": "application/json" } });
    if (!url.pathname.includes("/environments/second/")) return original(input);
    const rename = state.rename;
    const how = state.held ? await new Promise<Answer>((resolve) => waiting.push(resolve)) : "rows";
    if (how === "failure") return new Response(JSON.stringify({ success: false, error: { code: "docker.error", message: "raw diagnostic" } }), { status: 500, headers: { "content-type": "application/json" } });
    const response = await original(input);
    if (!/\/(containers|images|volumes|projects)$/.test(url.pathname)) return response;
    const body = await response.json() as { data: object[] };
    const data = how === "empty" ? [] : rename ? body.data.map((row) => ({ ...row, name: "obsolete", names: ["/obsolete"] })) : body.data;
    return new Response(JSON.stringify({ ...body, data, pagination: { totalItems: data.length } }), { status: response.status, headers: response.headers });
  });
  return { f, state, release: (answer: Answer) => waiting.splice(0).forEach((resolve) => resolve(answer)) };
}
// The first strip value is the page's total; table rows are the loaded resources.
const total = () => document.querySelector("dl [role=group] dd > span")?.textContent;
const loadedRows = () => document.querySelectorAll("tbody tr").length;
let refetch: () => Promise<unknown> = async () => {};

/** Observes the page's own query: "first-host rows:pending:obsolete rows". The first-host batch is the barrier. */
function probe(useInventory: Hook) {
  return function Probe() {
    const q = useInventory(), pending = useHostsPending(q.data);
    refetch = q.refetch;
    const first = q.data?.filter((row) => row.hostId === "first").length ?? 0;
    return <output data-testid="probe">{`${first}:${pending}:${q.data?.some((row) => row.name === "obsolete") ?? false}`}</output>;
  };
}
const probeText = () => screen.getByTestId("probe").textContent ?? "";
const published = () => waitFor(() => expect(Number(probeText().split(":")[0])).toBeGreaterThan(0));
const settle = () => act(async () => { await new Promise((r) => setTimeout(r, 30)); });

for (const [name, Page, empty, hook] of pages) {
  const Probe = probe(hook);
  for (const answer of ["rows", "empty", "failure"] as const) {
    it(`${name}: with only the second host selected, no empty list or total until it answers (${answer})`, async () => {
      preferencesStore.set((p) => ({ ...p, hostSelection: ["second"] }));
      const { f, release } = heldSecond();
      renderWithApp(<><Page /><Probe /></>, f.adapter);
      await published();
      expect(probeText().endsWith(":true:false")).toBe(true);
      expect(screen.queryByText(empty) === null).toBe(true);
      expect(total() ?? "–").toBe("–");
      release(answer);
      await waitFor(() => expect(probeText().split(":")[1]).toBe("false"));
      if (answer === "rows") expect(total()).not.toBe("–");
      else expect(screen.queryByText(empty) !== null || (total() !== undefined && total() !== "–")).toBe(true);
    });
  }
  it(`${name}: with all hosts in scope, the first host's rows stay visible while the total waits for the second`, async () => {
    const { f, release } = heldSecond();
    renderWithApp(<><Page /><Probe /></>, f.adapter);
    await published();
    await settle();
    expect(loadedRows() > 0).toBe(true);
    expect(total() ?? "–").toBe("–");
    expect(screen.queryByText(empty) === null).toBe(true);
    release("rows");
    await waitFor(() => expect(total()).not.toBe("–"));
  });
}

const [, Stacks, stacksEmpty, stacksHook] = pages[3]!;
const StacksProbe = probe(stacksHook);
// The finished read is the barrier for settled assertions.
function StacksFetch() { return <output data-testid="fetch">{useStacks().fetchStatus}</output>; }
const finished = async () => { await waitFor(() => expect(screen.getByTestId("fetch").textContent).toBe("idle")); await settle(); };
for (const answer of ["rows", "empty", "failure"] as const) {
  it(`stacks for a simple user: the total waits for the held host, then settles (${answer})`, async () => {
    const { f, release } = heldSecond(true);
    expect((await f.adapter.getCurrentUser())?.isAdmin).toBe(false);
    renderWithApp(<><Stacks /><StacksProbe /><StacksFetch /></>, f.adapter);
    await published();
    expect(probeText().endsWith(":true:false")).toBe(true);
    expect(total() ?? "–").toBe("–");
    await act(async () => { release(answer); });
    await finished();
    expect(probeText().split(":")[1]).toBe("false");
    const first = Number(probeText().split(":")[0]);
    expect(total()).toBe(String(answer === "rows" ? loadedRows() : first));
    expect(answer === "rows" ? loadedRows() > first : true).toBe(true);
  });
}

it("stacks for a simple user with only the second host selected settle on an empty answer", async () => {
  preferencesStore.set((p) => ({ ...p, hostSelection: ["second"] }));
  const { f, release } = heldSecond(true);
  await f.adapter.getCurrentUser();
  renderWithApp(<><Stacks /><StacksProbe /><StacksFetch /></>, f.adapter);
  await published();
  expect(probeText().endsWith(":true:false")).toBe(true);
  expect(screen.queryByText(stacksEmpty) === null).toBe(true);
  await act(async () => { release("empty"); });
  await finished();
  expect(probeText().split(":")[1]).toBe("false");
  expect(total()).toBe("0");
  expect(screen.queryByText(stacksEmpty) !== null).toBe(true);
});

const [, Containers, , containersHook] = pages[0]!;
const ContainersProbe = probe(containersHook);

it("a stale host catalogue keeps the answered hosts through the owner's copy", async () => {
  const x = heldSecond();
  // A first complete read retains the catalogue; the next catalogue read fails, so the owner annotates copies.
  x.state.held = false;
  await x.f.adapter.listContainers();
  x.state.held = true; x.state.catalogueDown = true;
  // An environments notification ends the cached catalogue, so the next read meets the failing catalogue.
  const stop = x.f.adapter.subscribeChanges(() => {});
  x.f.source.onmessage?.({ data: JSON.stringify({ channel: "environments", timestamp: "2026-11-21T00:00:00Z" }), lastEventId: "" } as MessageEvent);
  stop();
  // The page's own host list fails too (no known host, so nothing counts as pending there); the cached result
  // itself must still name the answered hosts, checked against the fixed scope of both hosts.
  let cached: unknown;
  function CacheProbe() { const q = useContainers(); useEffect(() => { cached = q.data; }); return <output data-testid="probe">{`${q.data?.filter((c) => c.hostId === "first").length ?? 0}:-:-`}</output>; }
  renderWithApp(<CacheProbe />, x.f.adapter);
  await published();
  await settle();
  expect(x.f.fetcher.mock.calls.filter(([url]) => new URL(String(url), "https://hub.example.com").pathname === "/api/environments").length >= 2).toBe(true);
  expect(inventoryHostsPending(cached, ["first", "second"])).toBe(true);
  expect(inventoryHostsPending(cached, ["first"])).toBe(false);
});

it("a cancelled read's late answer leaves neither its rows nor its pending host", async () => {
  const x = heldSecond();
  x.state.rename = true;
  renderWithApp(<><Containers /><ContainersProbe /></>, x.f.adapter);
  await published();
  expect(probeText()).toBe("2:true:false");
  // The refetch cancels the first read and answers at once; the first read's renamed answer comes last.
  // A stream notification ends request sharing, as after any change, so the new read sends its own requests.
  x.state.held = false; x.state.rename = false;
  const stop = x.f.adapter.subscribeChanges(() => {});
  x.f.source.onmessage?.({ data: JSON.stringify({ channel: "events", timestamp: "2026-11-21T00:00:00Z", eventLog: { type: "info" } }), lastEventId: "" } as MessageEvent);
  await act(async () => { await refetch(); });
  stop();
  await waitFor(() => expect(probeText().split(":")[1]).toBe("false"));
  await act(async () => { x.release("rows"); });
  await settle();
  expect(probeText().endsWith(":false:false")).toBe(true);
  expect(screen.queryByText("obsolete") === null).toBe(true);
});

it("a cancelled read's late annotated partial is not published by the query", async () => {
  // An adapter that ignores the abort signal: only the query's publication guard can drop the late callback.
  const f = resourceFixture();
  const rows = await f.adapter.listContainers();
  type Rows = typeof rows;
  const reads: { publish: (rows: Rows) => void; finish: (rows: Rows) => void }[] = [];
  f.adapter.listContainers = (publish = () => {}) => new Promise((finish) => { reads.push({ publish, finish }); });
  const recovery = inventoryRecovery();
  recovery.answered("first");
  const initial = recovery.partial(rows.filter((row) => row.hostId === "first"));
  const obsolete = recovery.partial(initial.map((row) => ({ ...row, name: "obsolete" })));
  renderWithApp(<><Containers /><ContainersProbe /></>, f.adapter);
  await waitFor(() => expect(reads.length).toBe(1));
  act(() => reads[0]!.publish(initial));
  await published();
  expect(probeText()).toBe("2:true:false");
  let refresh!: Promise<unknown>;
  act(() => { refresh = refetch(); });
  await waitFor(() => expect(reads.length).toBe(2));
  await act(async () => { reads[1]!.publish(rows); reads[1]!.finish(rows); await refresh; });
  await waitFor(() => expect(probeText()).toBe("2:false:false"));
  await act(async () => { reads[0]!.publish(obsolete); reads[0]!.finish(obsolete); });
  await settle();
  expect(probeText()).toBe("2:false:false");
});

it("a scope change ends pending for a host that already answered", async () => {
  preferencesStore.set((p) => ({ ...p, hostSelection: ["second"] }));
  const x = heldSecond();
  renderWithApp(<><Containers /><ContainersProbe /></>, x.f.adapter);
  await published();
  expect(probeText()).toBe("2:true:false");
  // The second host is still held; narrowing the scope to the answered host settles the page at once.
  act(() => preferencesStore.set((p) => ({ ...p, hostSelection: ["first"] })));
  await settle();
  expect(probeText()).toBe("2:false:false");
  expect(total()).toBe("2");
  await act(async () => { x.release("rows"); });
});

it("an identity reset after a partial discards the late answer and its pending host", async () => {
  const x = heldSecond();
  x.state.rename = true;
  renderWithApp(<><Containers /><ContainersProbe /></>, x.f.adapter);
  await published();
  expect(probeText()).toBe("2:true:false");
  // An unrelated 401 is the adapter's authoritative identity reset (inventory-scope.test.tsx L4-f).
  await act(async () => { await x.f.adapter.getHubBackupStorage().catch(() => undefined); });
  await act(async () => { x.release("rows"); });
  await settle();
  expect(probeText()).toBe("0:false:false");
  expect(screen.queryByText("obsolete") === null).toBe(true);
});

it("answered hosts survive structural sharing of an equal result and the owner copy", () => {
  const recovery = inventoryRecovery();
  recovery.answered("first");
  const partial = recovery.partial<string[]>([]);
  expect(inventoryHostsPending(partial, ["first", "second"])).toBe(true);
  expect(inventoryHostsPending(partial, ["first"])).toBe(false);
  // An equal array is reused only while the answered hosts are the same; a new answer yields a new reference.
  const again = recovery.partial<string[]>([]);
  expect(inventoryStructuralSharing(partial, again) === partial).toBe(true);
  recovery.answered("second");
  const final = recovery.result<string[]>([]);
  const shared = inventoryStructuralSharing(partial, final) as object;
  expect(shared === partial).toBe(false);
  expect(inventoryHostsPending(shared, ["second"])).toBe(false);
  const copy: string[] = [];
  copyInventoryRecovery(partial, copy);
  expect(inventoryHostsPending(copy, ["second"])).toBe(true);
  // A result without this metadata (cancelled or older) counts as settled.
  expect(inventoryHostsPending([], ["second"])).toBe(false);
});
