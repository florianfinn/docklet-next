import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { errorCode } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { planVolumePrune } from "@/domain/volume-prune";
import type { Volume } from "@/domain/volumes";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderRoutes } from "@/test/render";

import VolumesPage from "./VolumesPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 3 }));
});
afterEach(() => {
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const ALL = ["atlas", "boreal", "delta", "cedar"];
const OLD_PLEX = "vol-atlas-plex_old_config";
const ids = (vs: { id: string }[]) => vs.map((v) => v.id).toSorted();

const volume = (p: Partial<Volume>): Volume => ({ id: "v", name: "v", hostId: "atlas", kind: "volume", source: "/x", sizeBytes: 1, createdAt: "", consumers: [], labels: {}, ...p });

describe("prune rule", () => {
  it("keeps bind mounts and volumes of stopped containers and never reads disconnected hosts", () => {
    const stopped = { containerId: "c", containerName: "c", target: "/d", readOnly: false, running: false };
    const plan = planVolumePrune(
      [volume({ id: "free" }), volume({ id: "stopped", consumers: [stopped] }), volume({ id: "bind", kind: "bind" }), volume({ id: "far", hostId: "cedar" })],
      [
        { id: "atlas", status: "online" },
        { id: "cedar", status: "offline" },
      ],
      ["atlas", "cedar"],
    );
    expect([ids(plan.candidates), plan.kept.map((k) => `${k.volume.id}:${k.reason}`), plan.skippedHosts]).toEqual([["free"], ["stopped:in-use", "bind:bind"], ["cedar"]]);
  });
});

describe("demo prune contract", () => {
  it("previews and removes only unused volumes of connected hosts in scope", async () => {
    const adapter = createDemoAdapter();
    const plan = await adapter.planVolumePrune({ hostIds: ALL });
    expect(plan.candidates.map((v) => v.name).toSorted()).toEqual(["4f1c2e9a7b3d8f60a5e2c1b9d7f3a6e8c0b4d2f9e7a1c5b3d8f6e2a9c7b1d3f5", "nextcloud_previews_old", "paperless_export_tmp", "plex_old_config"]);
    expect(plan.skippedHosts).toEqual(["cedar"]);
    expect(plan.kept.some((k) => k.reason === "bind") && plan.kept.some((k) => k.reason === "in-use")).toBe(true);
    expect(plan.candidates.every((v) => v.sizeBytes > 0)).toBe(true);

    const result = await adapter.pruneVolumes({ hostIds: ["atlas"], volumeIds: [OLD_PLEX, "vol-boreal-nextcloud_previews_old", "vol-cedar-wiki_uploads_old", "vol-atlas-media_media_config", "bind-atlas-mnt-user-media"] });
    expect(result.removed.map((r) => r.id)).toEqual([OLD_PLEX]);
    expect(result.skipped).toEqual([
      { id: "vol-boreal-nextcloud_previews_old", reason: "not-found" },
      { id: "vol-cedar-wiki_uploads_old", reason: "not-found" },
      { id: "vol-atlas-media_media_config", reason: "in-use" },
      { id: "bind-atlas-mnt-user-media", reason: "bind" },
    ]);
    const after = await adapter.listVolumes();
    expect([after.some((v) => v.id === OLD_PLEX), after.some((v) => v.id === "vol-cedar-wiki_uploads_old")]).toEqual([false, true]);
    expect((await adapter.pruneVolumes({ hostIds: ALL, volumeIds: ["vol-cedar-wiki_uploads_old"] })).skipped).toEqual([{ id: "vol-cedar-wiki_uploads_old", reason: "host-offline" }]);
    expect(await adapter.deleteVolume("vol-cedar-wiki_uploads_old").catch((e: unknown) => errorCode(e))).toBe("host-offline");
  });

  it("keeps a previewed volume that a container mounts before the prune runs", async () => {
    const adapter = createDemoAdapter();
    const created = await adapter.createVolume({ name: "werkstatt_data", hostId: "boreal", labels: {} });
    const plan = await adapter.planVolumePrune({ hostIds: ["boreal"] });
    expect(plan.candidates.some((v) => v.id === created.id)).toBe(true);

    await adapter.deployStack({ stackId: null, hostId: "boreal", name: "werkstatt", compose: "services:\n  app:\n    image: nginx:1.27\n    volumes:\n      - data:/data\nvolumes:\n  data:\n", env: "", labels: [] });
    await waitFor(async () => expect((await adapter.listVolumes()).find((v) => v.id === created.id)?.consumers.length).toBe(1), { timeout: 8000 });

    const result = await adapter.pruneVolumes({ hostIds: ["boreal"], volumeIds: plan.candidates.map((v) => v.id) });
    expect(result.skipped).toEqual([{ id: created.id, reason: "in-use" }]);
    expect(await adapter.deleteVolume(created.id).catch((e: unknown) => errorCode(e))).toBe("in-use");
  }, 12_000);
});

describe("volumes page actions", () => {
  it("reviews candidates with sizes, prunes the chosen ones and updates the list", async () => {
    const adapter = createDemoAdapter();
    preferencesStore.set((p) => ({ ...p, hostSelection: ["atlas", "cedar"] }));
    renderRoutes([{ path: "/volumes", element: <VolumesPage /> }], ["/volumes"], adapter);
    await screen.findByRole("link", { name: "plex_old_config öffnen" });
    await userEvent.click(screen.getByRole("button", { name: "Aufräumen" }));
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText("2 von 2 Volumes ausgewählt · 1,5 GB");
    expect(within(dialog).getByText(/Nicht verbunden und deshalb nicht geprüft: Cedar/) !== null).toBe(true);
    expect(within(dialog).queryByText("wiki_uploads_old") === null).toBe(true);

    await userEvent.click(within(dialog).getByRole("checkbox", { name: /paperless_export_tmp/ }));
    await userEvent.click(within(dialog).getByRole("button", { name: "1 Volume löschen" }));
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    await waitFor(() => expect(screen.queryByRole("link", { name: "plex_old_config öffnen" }) === null).toBe(true));
    expect(screen.getByRole("link", { name: "paperless_export_tmp öffnen" }) !== null).toBe(true);
    expect((await adapter.listVolumes()).some((v) => v.name === "plex_old_config")).toBe(false);
  });

  it("offers the row actions by right click and by the menu button, with delete only for unused volumes", async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/volumes", element: <VolumesPage /> }], ["/volumes"], adapter);
    const used = await screen.findByRole("link", { name: "media_config öffnen" });
    fireEvent.contextMenu(used.closest("tr")!);
    const menu = await screen.findByRole("menu", { name: "media_config" });
    expect(within(menu).getByRole("menuitem", { name: "Löschen" }).getAttribute("aria-disabled")).toBe("true");
    await userEvent.keyboard("{Escape}");

    await userEvent.click(screen.getByRole("button", { name: "Aktionen für plex_old_config" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Löschen" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Löschen" }));
    await waitFor(() => expect(screen.queryByRole("link", { name: "plex_old_config öffnen" }) === null).toBe(true));
  });
});
