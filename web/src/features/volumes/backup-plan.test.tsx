import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { addDays, zonedParts } from "@/domain/schedule";
import SettingsPage from "@/features/settings/SettingsPage";
import { renderRoutes } from "@/test/render";

import VolumePage from "./VolumePage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const TZ = "Europe/Berlin";
const HA = "vol-atlas-home_ha_config";

describe("backup plan schedule and retention", () => {
  it("edits weekly time and scheduled snapshot count", async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/volumes/:volumeId", element: <VolumePage /> }], [`/volumes/${HA}?tab=backups`], adapter);
    const recurrence = await screen.findByRole("radiogroup", { name: "Wiederholung" });
    expect(within(recurrence).queryByRole("radio", { name: "Einmalig" }) === null).toBe(true);
    await userEvent.click(within(recurrence).getByRole("radio", { name: "Wöchentlich" }));
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Wochentag" })).getByRole("radio", { name: "Mittwoch" }));
    await userEvent.click(screen.getByRole("button", { name: "Uhrzeit: 02:30" }));
    await userEvent.click(within(await screen.findByRole("radiogroup", { name: "Stunde" })).getByRole("radio", { name: "04" }));
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Minute" })).getByRole("radio", { name: "15" }));
    const count = screen.getByRole("spinbutton", { name: "Aufbewahrung" });
    await userEvent.clear(count); await userEvent.type(count, "0");
    expect(screen.queryByRole("switch", { name: "Gesamtgröße begrenzen" }) === null).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Plan speichern" }));
    await waitFor(async () => expect((await adapter.getVolume(HA)).backup?.retentionCount).toBe(0));
    expect((await adapter.getVolume(HA)).backup?.schedule).toEqual({ mode: "weekly", day: "wed", time: "04:15" });
  });

  it("rejects invalid count retention in the adapter", async () => {
    const adapter = createDemoAdapter(); const v = await adapter.getVolume(HA);
    await expect(adapter.setBackupPolicy(HA, { ...v.backup!, retentionCount: -1 })).rejects.toMatchObject({ problem: { code: "volume.backup.policy-invalid" } });
  });

  it("edits scheduled update installs through date and time pickers, never free text", async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/updates"], adapter);
    const trigger = await screen.findByRole("button", { name: /^Zeitplan ändern: Paperless, Wöchentlich mittwochs um 03:30$/ });
    expect(screen.queryByRole("textbox", { name: /Zeitfenster/ })).toBe(null);
    await userEvent.click(trigger);
    const dialog = await screen.findByRole("dialog", { name: "Zeitplan ändern: Paperless" });
    await userEvent.click(within(dialog).getByRole("radio", { name: "Einmalig" }));
    // A one-time run starts without a date: the prompt shows and applying is blocked.
    expect(within(dialog).getByRole("button", { name: "Datum: Datum wählen" }).getAttribute("aria-invalid")).toBe("true");
    expect((within(dialog).getByRole("button", { name: "Übernehmen" }) as HTMLButtonElement).disabled).toBe(true);
    await userEvent.click(within(dialog).getByRole("button", { name: "Datum: Datum wählen" }));
    await screen.findByRole("grid");
    await userEvent.keyboard("{ArrowRight}{ArrowRight}{Enter}");
    const date = addDays(zonedParts(new Date(), TZ).date, 2);
    await userEvent.click(within(dialog).getByRole("button", { name: "Übernehmen" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBe(null));
    expect(screen.getByRole("button", { name: /^Zeitplan ändern: Paperless, Einmalig am / }) instanceof HTMLButtonElement).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(async () => expect((await adapter.listStacks()).find((s) => s.id === "paperless")?.autoUpdate).toEqual({ mode: "scheduled", schedule: { mode: "once", date, time: "03:30" } }));
  });
});
