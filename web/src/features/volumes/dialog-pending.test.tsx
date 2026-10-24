// Source: docklet-next/issues/205
// Revision: 8c507af62bfe5dd10ba0d5f6ea2f7feedb6b260b
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original pending rename gesture regressions for D1 review correction.
import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { useLocation } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterError } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import type { Volume } from "@/domain/volumes";
import { renderRoutes } from "@/test/render";
import { RenameVolumeDialog } from "./RenameVolumeDialog";

beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); vi.restoreAllMocks(); localStorage.clear(); });
it.each(["success", "failure"] as const)("a pending rename still delivers %s after the user edits the name", async (outcome) => {
  const adapter = createDemoAdapter(), error = vi.spyOn(toast, "error"), success = vi.spyOn(toast, "success");
  const volume = (await adapter.listVolumes()).find((entry) => entry.kind === "volume")!;
  let resolve!: (value: Volume) => void, reject!: (error: unknown) => void;
  const promise = new Promise<Volume>((done, fail) => { resolve = done; reject = fail; });
  adapter.renameVolume = vi.fn(() => promise);
  function Harness() {
    const [open, setOpen] = useState(true); const location = useLocation();
    return <><output data-testid="location">{location.pathname}</output>{open && <RenameVolumeDialog volume={volume} onClose={() => setOpen(false)} />}</>;
  }
  renderRoutes([{ path: "*", element: <Harness /> }], ["/volumes/old"], adapter);
  const input = screen.getByRole("textbox", { name: "Neuer Name" });
  await userEvent.clear(input); await userEvent.type(input, "submitted");
  await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
  expect(adapter.renameVolume).toHaveBeenCalledWith(volume.id, "submitted");
  await userEvent.type(input, "_edited");
  await act(async () => {
    if (outcome === "success") resolve({ ...volume, id: "atlas~submitted", name: "submitted" });
    else reject(new AdapterError("exists"));
    await promise.catch(() => undefined);
  });
  if (outcome === "success") {
    expect(screen.queryByRole("dialog") === null).toBe(true);
    expect(screen.getByTestId("location").textContent).toBe("/volumes/atlas~submitted");
    expect(success).toHaveBeenCalledWith(catalogs.de.volumes.renameDialog.done.replace("{name}", "submitted"));
  } else {
    expect(screen.getByRole("alert").textContent).toBe(catalogs.de.volumes.errors.exists);
    expect((input as HTMLInputElement).value).toBe("submitted_edited");
    expect(success).not.toHaveBeenCalled();
  }
  expect(error).not.toHaveBeenCalled();
});
