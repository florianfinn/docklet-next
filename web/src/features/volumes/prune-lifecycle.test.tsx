// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Deferred preview and execution lifecycle regressions for #181.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { VolumePrunePlan, VolumePruneResult } from "@/data/api";
import { volumeFixture } from "@/data/http/volume-commands.test-support";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";
import { PruneVolumesDialog } from "./PruneVolumesDialog";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); });
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; }
const snapshot: VolumePrunePlan = {
  candidates: ["removed", "held", "gone", "replaced", "failed", "excluded"].map((name) => ({ id: `atlas~${name}`, name, hostId: "atlas", kind: "volume", driver: "local", source: "", sizeBytes: 0, sizeUnavailable: true, createdAt: "2026-10-22T12:00:00Z", consumers: [], labels: {} })),
  all: false, kept: [], skippedHosts: ["cedar"], failedHosts: [{ hostId: "delta", code: "agent.unavailable" }], candidateReasons: { "atlas~removed": "unused-anonymous" },
};
const result: VolumePruneResult = { removed: [{ id: "atlas~removed", name: "removed", hostId: "atlas", sizeBytes: 0 }], skipped: [{ id: "atlas~held", reason: "in-use" }, { id: "atlas~gone", reason: "not-found" }, { id: "atlas~replaced", reason: "changed" }], failed: [{ id: "atlas~failed", name: "failed", hostId: "atlas", code: "volume.remove-failed" }], spaceReclaimed: 0 };
async function setup() {
  const adapter = createDemoAdapter(); const hosts = await adapter.listHosts();
  adapter.planVolumePrune = vi.fn(async () => snapshot);
  adapter.pruneVolumes = vi.fn(async () => result);
  function Page() { const [open, setOpen] = useState(true); return <><button onClick={() => setOpen(true)}>Reopen</button><PruneVolumesDialog open={open} onOpenChange={setOpen} hosts={hosts} /></>; }
  return { adapter, Page };
}

it("executes the selected snapshot and renders removed, skipped deviations and failed separately", async () => {
  const f = await setup(); const done = deferred<VolumePruneResult>();
  f.adapter.pruneVolumes = vi.fn(() => done.promise);
  renderWithApp(<f.Page />, f.adapter);
  const dialog = await screen.findByRole("dialog");
  await within(dialog).findByText("excluded");
  expect(within(dialog).getByText(/Delta: Vorschau fehlgeschlagen/) !== null).toBe(true);
  expect(within(dialog).getByText(/Nicht verbunden.*Cedar/) !== null).toBe(true);
  await userEvent.click(within(dialog).getByRole("checkbox", { name: /excluded/ }));
  await userEvent.click(within(dialog).getByRole("button", { name: "5 Volumes löschen" }));
  await waitFor(() => expect(f.adapter.pruneVolumes).toHaveBeenCalledOnce());
  const selected = snapshot.candidates.filter((v) => v.name !== "excluded");
  expect(f.adapter.pruneVolumes).toHaveBeenCalledWith({ hostIds: ["atlas", "boreal", "delta", "cedar"], volumeIds: selected.map((v) => v.id), candidates: selected.map(({ id, hostId, name, createdAt }) => ({ id, hostId, name, createdAt })), all: false });
  // Waiting does not refresh the plan; the execution deviation is authoritative.
  await act(async () => { done.resolve(result); await done.promise; });
  const skipped = await within(dialog).findByRole("region", { name: "Übersprungen" });
  expect(skipped.textContent?.includes("Wird inzwischen verwendet")).toBe(true);
  expect(skipped.textContent?.includes("Nicht mehr vorhanden")).toBe(true);
  expect(skipped.textContent?.includes("Identität seit der Vorschau geändert")).toBe(true);
  expect(within(dialog).getByRole("region", { name: "Entfernt" }).textContent?.includes("removed")).toBe(true);
  expect(within(dialog).getByRole("region", { name: "Fehlgeschlagen" }).textContent?.includes("failed")).toBe(true);
  expect(within(dialog).getByText(/Freigegebener Speicher unbekannt; mindestens 0 B/) !== null).toBe(true);
  expect(f.adapter.planVolumePrune).toHaveBeenCalledOnce();
});

it("ignores a late preview after close and loads a fresh plan when reopened", async () => {
  const f = await setup(); const done = deferred<VolumePrunePlan>();
  f.adapter.planVolumePrune = vi.fn().mockReturnValueOnce(done.promise).mockResolvedValue({ ...snapshot, candidates: [{ ...snapshot.candidates[0]!, name: "fresh" }] });
  renderWithApp(<f.Page />, f.adapter);
  await screen.findByRole("dialog");
  await waitFor(() => expect(f.adapter.planVolumePrune).toHaveBeenCalledOnce());
  await userEvent.click(screen.getByRole("button", { name: "Abbrechen" }));
  await act(async () => { done.resolve(snapshot); await done.promise; });
  expect(screen.queryByRole("dialog") === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Reopen" }));
  const dialog = await screen.findByRole("dialog");
  await within(dialog).findByText("fresh");
  expect(within(dialog).queryByText("excluded") === null).toBe(true);
});

it("ignores execution completion after close and does not reuse its result on reopen", async () => {
  const f = await setup(); const done = deferred<VolumePruneResult>();
  f.adapter.pruneVolumes = vi.fn(() => done.promise);
  renderWithApp(<f.Page />, f.adapter);
  await screen.findByText("excluded");
  await userEvent.click(screen.getByRole("button", { name: "6 Volumes löschen" }));
  await waitFor(() => expect(f.adapter.pruneVolumes).toHaveBeenCalledOnce());
  await userEvent.click(screen.getByRole("button", { name: "Abbrechen" }));
  await act(async () => { done.resolve(result); await done.promise; });
  expect(screen.queryByRole("dialog") === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Reopen" }));
  const dialog = await screen.findByRole("dialog");
  await within(dialog).findByText("excluded");
  expect(within(dialog).queryByRole("region", { name: "Übersprungen" }) === null).toBe(true);
});

it("recovers a transient preview failure through retry", async () => {
  const f = await setup();
  f.adapter.planVolumePrune = vi.fn().mockRejectedValueOnce(new Error("http.error")).mockResolvedValue(snapshot);
  renderWithApp(<f.Page />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Erneut versuchen" }));
  await screen.findByText("excluded");
  expect(f.adapter.planVolumePrune).toHaveBeenCalledTimes(2);
});

it("discards a pending preview when the host scope changes", async () => {
  const f = await setup(); const hosts = await f.adapter.listHosts(); const done = deferred<VolumePrunePlan>();
  f.adapter.planVolumePrune = vi.fn().mockReturnValueOnce(done.promise).mockResolvedValue({ ...snapshot, candidates: [{ ...snapshot.candidates[0]!, id: "boreal~fresh", hostId: "boreal", name: "fresh" }] });
  let changeScope!: () => void;
  function Page() {
    const [scope, setScope] = useState(hosts.filter((h) => h.id === "atlas"));
    changeScope = () => setScope(hosts.filter((h) => h.id === "boreal"));
    return <PruneVolumesDialog open onOpenChange={() => {}} hosts={scope} />;
  }
  renderWithApp(<Page />, f.adapter);
  await waitFor(() => expect(f.adapter.planVolumePrune).toHaveBeenCalledOnce());
  act(() => changeScope());
  await screen.findByText("fresh");
  await act(async () => { done.resolve(snapshot); await done.promise; });
  expect(screen.queryByText("excluded") === null).toBe(true);
  expect(screen.getByText("fresh") !== null).toBe(true);
});


for (const includeNamed of [false, true]) {
  it(`previews anonymous volumes by default and includes named only on request (${includeNamed})`, async () => {
    const f = volumeFixture();
    const original = f.fetcher.getMockImplementation()!;
    const requests: string[] = [];
    let submitted: unknown;
    f.fetcher.mockImplementation(async (input, init) => {
      const url = new URL(String(input), "https://hub.example.com");
      if (url.pathname.endsWith("/volumes/prune")) {
        if (init?.method === "POST") {
          submitted = JSON.parse(String(init.body));
          return f.json({ removed: ["anonymous"], skipped: [], failed: [], spaceReclaimed: 0 });
        }
        requests.push(url.search);
        const candidate = { name: "anonymous", driver: "local", createdAt: "2026-10-22T12:00:00Z", reason: "unused-anonymous" };
        return f.json({ candidates: url.searchParams.get("all") === "true" ? [candidate, { ...candidate, name: "named", reason: "unused" }] : [candidate] });
      }
      return original(input, init);
    });
    const hosts = await f.adapter.listHosts();
    renderWithApp(<PruneVolumesDialog open hosts={hosts} onOpenChange={() => {}} />, f.adapter);
    await screen.findByRole("checkbox", { name: /anonymous/ });
    expect(requests).toEqual(["?all=false"]);
    const toggle = screen.getByRole("checkbox", { name: "Benannte Volumes einbeziehen" });
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(screen.getByRole("checkbox", { name: /anonymous/ }).getAttribute("aria-checked")).toBe("true");
    expect(screen.queryByRole("checkbox", { name: /^named/ }) === null).toBe(true);
    if (includeNamed) {
      await userEvent.click(toggle);
      const named = await screen.findByRole("checkbox", { name: /^named/ });
      expect(requests).toEqual(["?all=false", "?all=true"]);
      expect(named.getAttribute("aria-checked")).toBe("true");
      await userEvent.click(named);
    }
    await userEvent.click(screen.getByRole("button", { name: "1 Volume löschen" }));
    await screen.findByRole("region", { name: "Entfernt" });
    expect(submitted).toEqual({ all: includeNamed, candidates: [{ name: "anonymous", createdAt: "2026-10-22T12:00:00Z" }] });
  });
}
