// Source: docklet-next/issues/205
// Revision: be813fd8a9f61eaf99161d4141e5c324b5c34094
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original non-network modal rejection coverage.
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef, useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterError } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";
import { RenameVolumeDialog } from "@/features/volumes/RenameVolumeDialog";
import { PruneVolumesDialog } from "@/features/volumes/PruneVolumesDialog";
import { PullDialog } from "@/features/images/ImageDialogs";
import { HostSettingsDialog } from "@/features/hosts/HostSettingsDialog";
import { NameDialog } from "@/features/volumes/files/FileDialogs";
import { useFileCommands } from "@/features/volumes/files/use-file-commands";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); vi.restoreAllMocks(); localStorage.clear(); });
it("keeps a rejected volume rename inline with its draft and retries successfully", async () => {
  const error = vi.spyOn(toast, "error"); const adapter = createDemoAdapter();
  const volume = (await adapter.listVolumes()).find((v) => v.kind === "volume")!;
  adapter.renameVolume = vi.fn().mockRejectedValueOnce(new AdapterError("exists")).mockResolvedValue({ ...volume, name: "retained" });
  function Harness() { const [open, setOpen] = useState(true); return open && <RenameVolumeDialog volume={volume} onClose={() => setOpen(false)} />; }
  renderWithApp(<Harness />, adapter);
  const input = screen.getByRole("textbox", { name: "Neuer Name" });
  await userEvent.clear(input); await userEvent.type(input, "retained");
  await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
  expect((await screen.findByRole("alert")).textContent).toBe(catalogs.de.volumes.errors.exists);
  expect((input as HTMLInputElement).value).toBe("retained"); expect(error).not.toHaveBeenCalled();
  await userEvent.type(input, "_edited"); expect(screen.queryByRole("alert") === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Umbenennen" }));
  await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
});
it("keeps a rejected image pull inline without clearing the image reference", async () => {
  const error = vi.spyOn(toast, "error"); const adapter = createDemoAdapter(); const hosts = await adapter.listHosts();
  adapter.pullImage = vi.fn().mockRejectedValue(new AdapterError("invalid-name"));
  renderWithApp(<PullDialog open onOpenChange={() => {}} hosts={hosts} images={[]} initial={{ hostId: "atlas", reference: "redis:7.4-alpine" }} />, adapter);
  await userEvent.click(screen.getByRole("button", { name: catalogs.de.images.pullDialog.submit }));
  expect((await screen.findByRole("alert")).textContent).toBe(catalogs.de.images.errors["invalid-name"]);
  expect((screen.getByLabelText(catalogs.de.images.pullDialog.reference) as HTMLInputElement).value).toBe("redis:7.4-alpine");
  expect(error).not.toHaveBeenCalled();
});
it("keeps rejected volume prune execution inline beside its unchanged selection", async () => {
  const error = vi.spyOn(toast, "error"); const adapter = createDemoAdapter(); const hosts = await adapter.listHosts();
  const plan = await adapter.planVolumePrune({ hostIds: ["atlas"], all: true });
  adapter.planVolumePrune = vi.fn(async () => plan);
  adapter.pruneVolumes = vi.fn().mockRejectedValue(new AdapterError("invalid-plan"));
  renderWithApp(<PruneVolumesDialog open onOpenChange={() => {}} hosts={hosts} />, adapter);
  const dialog = await screen.findByRole("dialog");
  const submit = await within(dialog).findByRole("button", { name: /löschen/i });
  await waitFor(() => expect((submit as HTMLButtonElement).disabled).toBe(false));
  const selected = within(dialog).getAllByRole("checkbox").map((node) => node.getAttribute("data-state"));
  await userEvent.click(submit);
  expect((await within(dialog).findByRole("alert")).textContent).toBe(catalogs.de.volumes.errors["invalid-plan"]);
  expect(within(dialog).getAllByRole("checkbox").map((node) => node.getAttribute("data-state"))).toEqual(selected);
  expect(error).not.toHaveBeenCalled();
});
it("shows agent restart rejection inline inside host settings", async () => {
  const error = vi.spyOn(toast, "error"); const adapter = createDemoAdapter();
  const hosts = await adapter.listHosts(); const components = await adapter.listSystemComponents();
  adapter.restartAgent = vi.fn().mockRejectedValue(new AdapterError("host-offline"));
  renderWithApp(<HostSettingsDialog host={hosts.find((h) => h.id === "atlas")!} component={components.find((c) => c.hostId === "atlas")} open onOpenChange={() => {}} />, adapter);
  const dialog = await screen.findByRole("dialog");
  await userEvent.click(within(dialog).getByRole("tab", { name: "Agent-Container" }));
  await userEvent.click(within(dialog).getByRole("button", { name: "Agent neu starten" }));
  expect((await within(dialog).findByRole("alert")).textContent).toBeTruthy();
  expect(error).not.toHaveBeenCalled(); expect(adapter.restartAgent).toHaveBeenCalledOnce();
});
function FileName() {
  const lock = useRef(false);
  const cmd = useFileCommands("example-volume", "revision", () => {}, lock);
  return <NameDialog open onOpenChange={() => {}} title="Create file" submitLabel="Create" siblings={[]} onSubmit={(name) => cmd.createEntry("example-volume", "/", name, "file")} />;
}
it("shows a file gesture failure inline in the shared name form", async () => {
  const error = vi.spyOn(toast, "error"); const adapter = createDemoAdapter();
  adapter.createEntry = vi.fn().mockRejectedValue(new AdapterError("exists"));
  renderWithApp(<FileName />, adapter);
  await userEvent.type(screen.getByRole("textbox"), "retained.txt");
  await userEvent.click(screen.getByRole("button", { name: "Create" }));
  expect((await screen.findByRole("alert")).textContent).toBeTruthy();
  expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe("retained.txt"); expect(error).not.toHaveBeenCalled();
});
