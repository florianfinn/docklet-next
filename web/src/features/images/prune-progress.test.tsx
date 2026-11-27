// Source: docklet-next/docs/design/image-prune-progress.md
// Revision: b637b8c19497e52b7a62987a497ca4ee05b1b5af
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Per-image cleanup progress, run binding and compact count regression coverage for issue #661.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useSyncExternalStore } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { imageRegistryFixture, mutations } from "@/data/http/image-registry.test-support";
import type { DockerImage } from "@/domain/images";
import type { ActionRun, BulkCheckCounts, Host } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { PruneDialog } from "./ImageDialogs";

let runs: ActionRun[] = [];
const listeners = new Set<() => void>();
const publish = (next: ActionRun[]) => { runs = next; listeners.forEach((l) => l()); };
vi.mock("@/data/live", async (original) => ({
  ...(await original<typeof import("@/data/live")>()),
  useActionRuns: () => useSyncExternalStore((l) => { listeners.add(l); return () => listeners.delete(l); }, () => runs),
}));

const host = { id: "host", name: "Example host", status: "online" } as Host;
const image = { id: "host~sha256:abc", hostId: "host", repository: "example/app", tag: "1", aliases: [], digest: "sha256:abc", consumers: [], sizeMb: 1 } as unknown as DockerImage;
const bulk = (completed: number, planned: number): BulkCheckCounts => ({ unit: "target", planned, plannedKnown: true, completed, succeeded: completed, failed: 0, skipped: 0, updates: 0 });
const run = (id: string, startedAt: string, counts: BulkCheckCounts): ActionRun => ({ id, kind: "prune", sourceOperation: "image_prune", hostId: "host", status: "running", startedAt, bulk: counts } as unknown as ActionRun);
function barrier() { let release!: () => void; const promise = new Promise<void>((resolve) => { release = resolve; }); return { promise, release }; }
async function confirm() { const button = await screen.findByRole("button", { name: "Entfernen" }); await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false)); await userEvent.click(button); }
afterEach(() => publish([]));

it("keeps candidate names collapsed behind the details toggle", async () => {
  const f = imageRegistryFixture();
  renderWithApp(<PruneDialog open onOpenChange={vi.fn()} hosts={[host]} images={[image]} />, f.adapter);
  await userEvent.click(await screen.findByRole("radio", { name: "Alle ungenutzten" }));
  const preview = document.querySelector('[data-slot="prune-preview"]')!;
  expect(preview.textContent).toContain("1 Image wird entfernt");
  expect(preview.textContent).not.toContain("example/app:1");
  await userEvent.click(within(preview as HTMLElement).getByRole("button", { name: "Details anzeigen (1)" }));
  expect(preview.textContent).toContain("example/app:1");
});

it("binds the first cleanup run that appears after the submit, never an earlier one", async () => {
  const f = imageRegistryFixture(), done = barrier(); f.state.delay = () => done.promise;
  // A run of an earlier cleanup is still listed, with a later start time from a skewed host clock.
  publish([run("earlier", "2026-11-27T12:00:00Z", bulk(5, 9))]);
  renderWithApp(<PruneDialog open onOpenChange={vi.fn()} hosts={[host]} images={[image]} />, f.adapter);
  await confirm(); await waitFor(() => expect(mutations(f)).toHaveLength(1));
  const progress = () => document.querySelector('[data-slot="prune-progress"]')?.textContent ?? "";
  expect(progress()).toContain("Wird aufgeräumt");
  expect(progress()).not.toContain("5 von 9");
  await act(async () => publish([...runs, run("ours", "2026-11-27T11:00:00Z", bulk(1, 2))]));
  await waitFor(() => expect(progress()).toContain("1 von 2 Images bearbeitet"));
  // A run that appears later for the same host never replaces the bound one.
  await act(async () => publish([...runs, run("later", "2026-11-27T10:00:00Z", bulk(7, 8))]));
  expect(progress()).toContain("1 von 2 Images bearbeitet");
  await act(async () => done.release());
  await screen.findByText("Aufräumbericht");
});

it("shows the last observed counts as incomplete when the host request has no result", async () => {
  const f = imageRegistryFixture(), done = barrier(); f.state.delay = () => done.promise; f.state.failTransport = true;
  renderWithApp(<PruneDialog open onOpenChange={vi.fn()} hosts={[host]} images={[image]} />, f.adapter);
  await confirm(); await waitFor(() => expect(mutations(f)).toHaveLength(1));
  await act(async () => publish([run("ours", "2026-11-27T11:00:00Z", bulk(3, 9))]));
  await act(async () => done.release());
  await screen.findByText("Aufräumbericht");
  const incomplete = document.querySelector('[data-slot="prune-incomplete"]');
  expect(incomplete?.textContent).toBe("3 Images entfernt · Ergebnis unvollständig – Details in der Aktivität");
  expect(screen.getByRole("alert") !== null).toBe(true);
});

it("binds no cleanup run that first appears after the host request settled", async () => {
  const f = imageRegistryFixture(); f.state.failTransport = true;
  renderWithApp(<PruneDialog open onOpenChange={vi.fn()} hosts={[host]} images={[image]} />, f.adapter);
  await confirm();
  await screen.findByText("Aufräumbericht");
  await act(async () => publish([run("unrelated", "2026-11-27T11:00:00Z", bulk(4, 9))]));
  expect(document.querySelector('[data-slot="prune-incomplete"]') === null).toBe(true);
});

it("keeps the last observed counts when the bound run leaves the list or loses its counts", async () => {
  const f = imageRegistryFixture(), done = barrier(); f.state.delay = () => done.promise; f.state.failTransport = true;
  renderWithApp(<PruneDialog open onOpenChange={vi.fn()} hosts={[host]} images={[image]} />, f.adapter);
  await confirm(); await waitFor(() => expect(mutations(f)).toHaveLength(1));
  await act(async () => publish([run("ours", "2026-11-27T11:00:00Z", bulk(3, 9))]));
  await act(async () => publish([{ ...runs[0]!, bulk: undefined }]));
  await act(async () => publish([]));
  await act(async () => done.release());
  await screen.findByText("Aufräumbericht");
  expect(document.querySelector('[data-slot="prune-incomplete"]')?.textContent).toContain("3 Images entfernt");
});
