import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { errorCode } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import ContainersPage from "@/features/containers/ContainersPage";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderRoutes } from "@/test/render";

import VolumePage from "./VolumePage";
import VolumesPage from "./VolumesPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 3 }));
});

afterEach(() => {
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const MEDIA_CONFIG = "vol-atlas-media_media_config";

describe("volumes and their consumers", () => {
  it("derives volumes from compose mounts and links consumers both ways", async () => {
    renderRoutes(
      [
        { path: "/containers", element: <ContainersPage /> },
        { path: "/volumes/:volumeId", element: <VolumePage /> },
      ],
      [`/volumes/${MEDIA_CONFIG}`],
    );
    expect((await screen.findByRole("heading", { level: 1 })).textContent).toBe("media_config");
    const stackLink = await screen.findByRole("link", { name: "Stack öffnen" });
    expect(stackLink.getAttribute("href")).toBe("/stacks/media");
    expect(screen.getByRole("link", { name: "Container öffnen" }).getAttribute("href")).toBe("/containers?c=c-media-jellyfin");

    await userEvent.click(screen.getByRole("link", { name: "Container öffnen" }));
    const mount = await screen.findByRole("link", { name: "Dateien von media_config:/config öffnen" });
    expect(mount.getAttribute("href")).toBe(`/volumes/${MEDIA_CONFIG}?tab=files`);
  });

  it("follows the global host filter on the overview", async () => {
    preferencesStore.set((p) => ({ ...p, hostSelection: ["boreal"] }));
    renderRoutes([{ path: "/volumes", element: <VolumesPage /> }], ["/volumes"]);
    await screen.findByRole("link", { name: "cloud_data öffnen" });
    expect(screen.queryByRole("link", { name: "media_config öffnen" }) === null).toBe(true);
  });

  it("creates, renames and deletes files in memory with confirmation", async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/volumes/:volumeId", element: <VolumePage /> }], [`/volumes/${MEDIA_CONFIG}?tab=files`], adapter);
    await screen.findByRole("button", { name: "system.xml öffnen" });

    await userEvent.click(screen.getByRole("button", { name: "Neue Datei" }));
    await userEvent.type(screen.getByLabelText("Name"), "system.xml");
    await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
    expect(screen.getByText("Existiert bereits in diesem Ordner") !== null).toBe(true);
    await userEvent.clear(screen.getByLabelText("Name"));
    await userEvent.type(screen.getByLabelText("Name"), "notiz.txt");
    await userEvent.click(screen.getByRole("button", { name: "Anlegen" }));
    // A new file opens as an editor tab; the folder tab still lists it.
    await waitFor(() => expect(screen.getByRole("tab", { name: /^notiz\.txt/ }).getAttribute("aria-selected")).toBe("true"));
    await userEvent.click(screen.getByRole("tab", { name: /^Wurzel/ }));
    await screen.findByRole("button", { name: "notiz.txt öffnen" });

    await userEvent.click(screen.getByRole("button", { name: "Aktionen für notiz.txt" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Löschen" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Abbrechen" }));
    expect(screen.getByRole("button", { name: "notiz.txt öffnen" }) !== null).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: "Aktionen für notiz.txt" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Löschen" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Löschen" }));
    // The dialog stays open (and the page behind it hidden) until the deletion has finished.
    await waitFor(() => expect(screen.queryByRole("alertdialog") === null).toBe(true));
    await waitFor(() => expect(screen.queryByRole("button", { name: "notiz.txt öffnen" }) === null).toBe(true));
    expect(screen.queryByRole("tab", { name: /^notiz\.txt/ }) === null).toBe(true);
    expect((await adapter.listFiles(MEDIA_CONFIG, "/")).entries.some((e) => e.name === "notiz.txt")).toBe(false);
  });
});

describe("demo storage contract", () => {
  it("rejects deleting a volume in use and allows deleting a fresh one", async () => {
    const adapter = createDemoAdapter();
    const inUse = await adapter.deleteVolume(MEDIA_CONFIG).catch((e: unknown) => errorCode(e));
    expect(inUse).toBe("in-use");
    const created = await adapter.createVolume({ name: "werkstatt_daten", hostId: "boreal", labels: {} });
    expect((await adapter.listVolumes()).some((v) => v.id === created.id && v.consumers.length === 0)).toBe(true);
    await adapter.deleteVolume(created.id);
    expect((await adapter.listVolumes()).some((v) => v.id === created.id)).toBe(false);
    expect(await adapter.createVolume({ name: "x", hostId: "cedar", labels: {} }).catch((e: unknown) => errorCode(e))).toBe("host-offline");
  });

  it("runs a manual backup, records it and keeps only the configured number of archives", async () => {
    const adapter = createDemoAdapter();
    const id = "vol-atlas-home_ha_config";
    const before = await adapter.getVolume(id);
    await adapter.setBackupPolicy(id, { ...before.backup!, retention: { maxCount: 2, maxSizeGb: null } });
    await adapter.runBackup(id);
    await waitFor(async () => expect((await adapter.getVolume(id)).backups[0]?.status).toBe("succeeded"), { timeout: 6000 });
    const after = await adapter.getVolume(id);
    expect(after.backups.filter((r) => r.status === "succeeded").length).toBe(2);
    expect(after.backups[0]?.trigger).toBe("manual");
  });
});
