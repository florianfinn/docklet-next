// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Mutation lifecycle regression tests for issue #181.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { useLocation } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { AdapterError } from "@/data/api";
import type { Volume } from "@/domain/volumes";
import { renderRoutes } from "@/test/render";
import { CreateVolumeDialog } from "./CreateVolumeDialog";
import { RenameVolumeDialog } from "./RenameVolumeDialog";
import VolumePage from "./VolumePage";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); });
const volume: Volume = { id: "vol-atlas-old", hostId: "atlas", name: "old", kind: "volume", driver: "local", source: "", sizeBytes: 0, createdAt: "", consumers: [], labels: {} };
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; }
function Location() { return <p data-testid="location">{useLocation().pathname}</p>; }

it("renames a volume and navigates to its returned identity", async () => {
  const adapter = createDemoAdapter();
  adapter.renameVolume = vi.fn(async () => ({ ...volume, id: "atlas~new", name: "new" }));
  function Page() { const [open, setOpen] = useState(true); return <><Location />{open && <RenameVolumeDialog volume={volume} onClose={() => setOpen(false)} />}</>; }
  renderRoutes([{ path: "*", element: <Page /> }], ["/volumes/old"], adapter);
  await userEvent.clear(screen.getByRole("textbox", { name: "Neuer Name" }));
  await userEvent.type(screen.getByRole("textbox", { name: "Neuer Name" }), "new");
  await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
  await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/volumes/atlas~new"));
  expect(adapter.renameVolume).toHaveBeenCalledWith(volume.id, "new");
});

it("ignores a rename response after the dialog closes", async () => {
  const adapter = createDemoAdapter(); const done = deferred<Volume>();
  adapter.renameVolume = vi.fn(() => done.promise);
  function Page() { const [open, setOpen] = useState(true); return <><Location />{open && <RenameVolumeDialog volume={volume} onClose={() => setOpen(false)} />}</>; }
  renderRoutes([{ path: "*", element: <Page /> }], ["/volumes/old"], adapter);
  await userEvent.type(screen.getByRole("textbox", { name: "Neuer Name" }), "-new");
  await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
  await waitFor(() => expect(adapter.renameVolume).toHaveBeenCalledOnce());
  await userEvent.click(screen.getByRole("button", { name: "Abbrechen" }));
  await act(async () => { done.resolve({ ...volume, id: "atlas~new" }); await done.promise; });
  expect(screen.getByTestId("location").textContent).toBe("/volumes/old");
});

it("ignores create completion after closing, and creates normally on a fresh dialog", async () => {
  const adapter = createDemoAdapter(); const hosts = await adapter.listHosts(); const done = deferred<Volume>();
  adapter.createVolume = vi.fn(() => done.promise);
  function Page() { const [open, setOpen] = useState(true); return <><Location /><button onClick={() => setOpen(true)}>Reopen</button><CreateVolumeDialog open={open} onOpenChange={setOpen} hosts={hosts} volumes={[]} defaultHostId="atlas" /></>; }
  renderRoutes([{ path: "*", element: <Page /> }], ["/volumes"], adapter);
  await userEvent.type(screen.getByRole("textbox", { name: "Name" }), "created");
  await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
  await waitFor(() => expect(adapter.createVolume).toHaveBeenCalledOnce());
  await userEvent.keyboard("{Escape}");
  await act(async () => { done.resolve({ ...volume, id: "atlas~created" }); await done.promise; });
  expect(screen.getByTestId("location").textContent).toBe("/volumes");
  adapter.createVolume = vi.fn(async () => ({ ...volume, id: "atlas~created" }));
  await userEvent.click(screen.getByRole("button", { name: "Reopen" }));
  await userEvent.type(screen.getByRole("textbox", { name: "Name" }), "created");
  await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
  await waitFor(() => expect(screen.getByTestId("location").textContent).toBe("/volumes/atlas~created"));
});

it("keeps an in-use delete visible and navigates only after a successful retry", async () => {
  const adapter = createDemoAdapter();
  adapter.getVolume = vi.fn(async () => ({ ...volume, backups: [] }));
  adapter.deleteVolume = vi.fn().mockRejectedValueOnce(new AdapterError("in-use")).mockResolvedValue(undefined);
  renderRoutes([{ path: "/volumes/:volumeId", element: <VolumePage /> }, { path: "/volumes", element: <Location /> }], ["/volumes/vol-atlas-old"], adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Volume löschen" }));
  const dialog = await screen.findByRole("alertdialog");
  await userEvent.click(within(dialog).getByRole("button", { name: "Löschen" }));
  await waitFor(() => expect(adapter.deleteVolume).toHaveBeenCalledOnce());
  await waitFor(() => expect(within(dialog).getByRole("button", { name: "Löschen" }).getAttribute("aria-busy")).toBe(null));
  expect(screen.queryByRole("alertdialog") !== null).toBe(true);
  await userEvent.click(within(dialog).getByRole("button", { name: "Löschen" }));
  await screen.findByTestId("location");
});

it("does not navigate after leaving while a rename is pending", async () => {
  const adapter = createDemoAdapter(); const done = deferred<Volume>();
  adapter.renameVolume = vi.fn(() => done.promise);
  const view = renderRoutes([{ path: "/volumes/old", element: <RenameVolumeDialog volume={volume} onClose={() => {}} /> }, { path: "/elsewhere", element: <Location /> }], ["/volumes/old"], adapter);
  await userEvent.type(screen.getByRole("textbox", { name: "Neuer Name" }), "-new");
  await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
  await waitFor(() => expect(adapter.renameVolume).toHaveBeenCalledOnce());
  await act(() => view.router.navigate("/elsewhere"));
  await act(async () => { done.resolve({ ...volume, id: "atlas~new" }); await done.promise; });
  expect(view.router.state.location.pathname).toBe("/elsewhere");
});

it("discards the old volume's pending rename when the detail identity changes", async () => {
  const adapter = createDemoAdapter(); const done = deferred<Volume>();
  adapter.renameVolume = vi.fn(() => done.promise);
  adapter.getVolume = vi.fn(async (id: string) => ({ ...volume, id, name: id === "old" ? "old" : "other", backups: [] }));
  const view = renderRoutes([{ path: "/volumes/:volumeId", element: <VolumePage /> }], ["/volumes/old"], adapter);
  const rename = await screen.findByRole("button", { name: "Volume umbenennen" });
  await waitFor(() => expect(rename.hasAttribute("disabled")).toBe(false));
  await userEvent.click(rename);
  await userEvent.type(screen.getByRole("textbox", { name: "Neuer Name" }), "-new");
  await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
  await waitFor(() => expect(adapter.renameVolume).toHaveBeenCalledOnce());
  await act(() => view.router.navigate("/volumes/other"));
  await screen.findByRole("heading", { name: "other" });
  await act(async () => { done.resolve({ ...volume, id: "atlas~renamed" }); await done.promise; });
  expect(view.router.state.location.pathname).toBe("/volumes/other");
  expect(screen.queryByRole("dialog") === null).toBe(true);
});
