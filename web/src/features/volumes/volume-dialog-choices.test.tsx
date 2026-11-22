// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #557 tests: volume dialogs keep choices and drafts across host status changes.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import type { VolumePrunePlan, VolumePruneResult } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import type { Host } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { CreateVolumeDialog } from "./CreateVolumeDialog";
import { PruneVolumesDialog } from "./PruneVolumesDialog";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); });

const t = catalogs.de.volumes.prune, tc = catalogs.de.volumes.createDialog;
const at = "2026-10-22T12:00:00Z";
const candidate = (name: string, createdAt = at) => ({ id: `atlas~${name}`, name, hostId: "atlas", kind: "volume" as const, driver: "local", source: "", sizeBytes: 0, sizeUnavailable: true, createdAt, consumers: [], labels: {} });
const plan = (...candidates: ReturnType<typeof candidate>[]): VolumePrunePlan => ({ candidates, all: false, kept: [], skippedHosts: [], failedHosts: [] });
const box = (name: string) => screen.getByRole("checkbox", { name: new RegExp(`^${name}`) });
const checked = (name: string) => box(name).getAttribute("aria-checked") === "true";

/** The dialog with a host scope whose status the test flips; each flip reads the next plan. */
async function prune(plans: (VolumePrunePlan | Error)[]) {
  const adapter = createDemoAdapter(), hosts = await adapter.listHosts();
  const reads = [...plans];
  adapter.planVolumePrune = vi.fn(async () => { const next = reads.length > 1 ? reads.shift()! : reads[0]!; if (next instanceof Error) throw next; return next; });
  const result: VolumePruneResult = { removed: [], skipped: [], failed: [], spaceReclaimed: 0 };
  const done = { resolve: (_: VolumePruneResult) => {} };
  adapter.pruneVolumes = vi.fn(() => new Promise<VolumePruneResult>((resolve) => { done.resolve = resolve; }));
  let flip!: (status: Host["status"]) => void, reorder!: () => void;
  function Page() {
    const [scope, setScope] = useState(hosts.slice(0, 2));
    flip = (status) => setScope((s) => s.map((h, i) => (i === 1 ? { ...h, status } : h)));
    reorder = () => setScope((s) => [...s].reverse());
    return <PruneVolumesDialog open onOpenChange={() => {}} hosts={scope} />;
  }
  renderWithApp(<Page />, adapter);
  return { adapter, result, done, flip: (s: Host["status"]) => act(() => flip(s)), reorder: () => act(() => reorder()) };
}

it("a host status change keeps exclusions, adds new candidates unselected and marked, and drops vanished ones", async () => {
  const x = await prune([plan(candidate("kept"), candidate("excluded"), candidate("vanishes")), plan(candidate("kept"), candidate("excluded"), candidate("arrived"))]);
  await screen.findByRole("checkbox", { name: /^excluded/ });
  expect(checked("kept") && checked("excluded") && checked("vanishes")).toBe(true);
  await userEvent.click(box("excluded"));
  x.flip("offline");
  await screen.findByRole("checkbox", { name: /^arrived/ });
  expect(checked("kept")).toBe(true);
  expect(checked("excluded")).toBe(false);
  expect(checked("arrived")).toBe(false);
  expect(within(box("arrived").closest("li")!).queryByText(new RegExp(t.new)) !== null).toBe(true);
  expect(screen.queryByRole("checkbox", { name: /^vanishes/ }) === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: /löschen/ }));
  await waitFor(() => expect(x.adapter.pruneVolumes).toHaveBeenCalledOnce());
  const sent = vi.mocked(x.adapter.pruneVolumes).mock.calls[0]![0];
  expect(sent.candidates).toEqual([{ id: "atlas~kept", hostId: "atlas", name: "kept", createdAt: at }]);
});

it("a replaced volume (same name, new creation) starts unselected; an unchanged one that returns keeps its choice", async () => {
  const later = "2026-11-01T00:00:00Z";
  const x = await prune([plan(candidate("same"), candidate("swap")), plan(candidate("swap", later)), plan(candidate("same"), candidate("swap", later))]);
  await screen.findByRole("checkbox", { name: /^same/ });
  expect(checked("same") && checked("swap")).toBe(true);
  x.flip("offline");
  await waitFor(() => expect(screen.queryByRole("checkbox", { name: /^same/ }) === null && screen.queryByRole("checkbox", { name: /^swap/ }) !== null).toBe(true));
  expect(checked("swap")).toBe(false);
  x.flip("online");
  await screen.findByRole("checkbox", { name: /^same/ });
  expect(checked("same")).toBe(true);
  expect(checked("swap")).toBe(false);
});

it("host order changes and a failed first plan do not reset the choices", async () => {
  const x = await prune([new Error("agent.unavailable"), plan(candidate("first")), plan(candidate("first"), candidate("later"))]);
  // The first plan fails; the first successful plan initialises the selection.
  await screen.findByRole("button", { name: /erneut|Retry/i });
  await userEvent.click(screen.getByRole("button", { name: /erneut|Retry/i }));
  await screen.findByRole("checkbox", { name: /^first/ });
  expect(checked("first")).toBe(true);
  await userEvent.click(box("first"));
  x.reorder();
  await screen.findByRole("checkbox", { name: /^first/ });
  expect(checked("first")).toBe(false);
  x.flip("offline");
  await screen.findByRole("checkbox", { name: /^later/ });
  expect(checked("first")).toBe(false);
  expect(checked("later")).toBe(false);
});

it("a running prune keeps its result through a host status change", async () => {
  const x = await prune([plan(candidate("gone")), plan(candidate("gone")), plan(candidate("gone"))]);
  await screen.findByRole("checkbox", { name: /^gone/ });
  await userEvent.click(screen.getByRole("button", { name: /löschen/ }));
  await waitFor(() => expect(x.adapter.pruneVolumes).toHaveBeenCalledOnce());
  const reads = () => vi.mocked(x.adapter.planVolumePrune).mock.calls.length;
  const settle = () => act(async () => { await new Promise((r) => setTimeout(r, 20)); });
  x.flip("offline");
  await waitFor(() => expect(reads()).toBe(2));
  await settle();
  await act(async () => { x.done.resolve({ ...x.result, removed: [{ id: "atlas~gone", name: "gone", hostId: "atlas", sizeBytes: 0 }] }); });
  expect(screen.queryByRole("region", { name: t.removed }) !== null).toBe(true);
  // A later status change reads a new plan but keeps the shown result.
  x.flip("online");
  await waitFor(() => expect(reads()).toBe(3));
  await settle();
  expect(screen.queryByRole("region", { name: t.removed }) !== null).toBe(true);
});

it("the create draft survives a host status change; an offline host only holds the submit", async () => {
  const adapter = createDemoAdapter(), hosts = (await adapter.listHosts()).slice(0, 2);
  let flip!: (status: Host["status"]) => void;
  function Page() {
    const [scope, setScope] = useState(hosts);
    flip = (status) => setScope((s) => s.map((h, i) => (i === 0 ? { ...h, status } : h)));
    return <CreateVolumeDialog open onOpenChange={() => {}} hosts={scope} volumes={[]} defaultHostId={hosts[0]!.id} />;
  }
  renderWithApp(<Page />, adapter);
  await userEvent.type(await screen.findByLabelText(tc.name), "app_data");
  await userEvent.type(screen.getByLabelText(tc.labels), "team=example");
  const submit = () => screen.getByRole("button", { name: tc.submit });
  await waitFor(() => expect(submit().hasAttribute("disabled")).toBe(false));
  act(() => flip("offline"));
  expect((screen.getByLabelText(tc.name) as HTMLInputElement).value).toBe("app_data");
  expect((screen.getByLabelText(tc.labels) as HTMLTextAreaElement).value).toBe("team=example");
  expect(submit().hasAttribute("disabled")).toBe(true);
  expect(screen.queryByText(tc.hostUnavailable) !== null).toBe(true);
  act(() => flip("online"));
  await waitFor(() => expect(submit().hasAttribute("disabled")).toBe(false));
  expect(screen.queryByText(tc.hostUnavailable) === null).toBe(true);
});
