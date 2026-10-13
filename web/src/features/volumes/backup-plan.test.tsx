import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { addDays, nextOccurrence, parseDate, toIsoDate, weekdayOf, zonedParts } from "@/domain/schedule";
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

/** Same day next month, clamped like the calendar's Page Down. */
function nextMonth(date: string): string {
  const p = parseDate(date)!;
  const last = new Date(Date.UTC(p.y, p.m + 1, 0)).getUTCDate();
  const t = new Date(Date.UTC(p.y, p.m, 1));
  return toIsoDate(t.getUTCFullYear(), t.getUTCMonth() + 1, Math.min(p.d, last));
}

describe("backup plan schedule and retention", () => {
  it("edits a weekly plan with weekday, time and date pickers and independent retention limits", async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/volumes/:volumeId", element: <VolumePage /> }], [`/volumes/${HA}?tab=backups`], adapter);
    const recurrence = await screen.findByRole("radiogroup", { name: "Wiederholung" });
    // No free-text time or window field anywhere in the plan.
    expect(screen.queryByRole("textbox", { name: /Uhrzeit|Zeitfenster/ }) === null && document.querySelector('input[type="time"]') === null).toBe(true);

    await userEvent.click(within(recurrence).getByRole("radio", { name: "Wöchentlich" }));
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Wochentag" })).getByRole("radio", { name: "Mittwoch" }));

    await userEvent.click(screen.getByRole("button", { name: "Uhrzeit: 02:30" }));
    await userEvent.click(within(await screen.findByRole("radiogroup", { name: "Stunde" })).getByRole("radio", { name: "04" }));
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Minute" })).getByRole("radio", { name: "15" }));
    await waitFor(() => expect(screen.queryByRole("radiogroup", { name: "Minute" })).toBe(null));
    expect(screen.getByRole("button", { name: "Uhrzeit: 04:15" }) instanceof HTMLButtonElement).toBe(true);

    // The calendar opens on the next Wednesday; Page Down + Enter picks the same day a month later as first date.
    const first = zonedParts(nextOccurrence({ mode: "weekly", day: "wed", time: "04:15" }, new Date(), TZ)!, TZ).date;
    await userEvent.click(screen.getByRole("button", { name: /^Erster Termin: / }));
    await screen.findByRole("grid");
    await waitFor(() => expect((document.activeElement as HTMLElement | null)?.dataset.date).toBe(first));
    await userEvent.keyboard("{PageDown}{Enter}");
    const picked = nextMonth(first);
    const day = weekdayOf(picked)!;
    await waitFor(() => expect(screen.queryByRole("grid")).toBe(null));
    // Choosing a date sets the weekday to that date's; the recurrence stays weekly.
    expect(within(recurrence).getByRole("radio", { name: "Wöchentlich" }).getAttribute("aria-checked")).toBe("true");
    const dayNames = { mon: "Montag", tue: "Dienstag", wed: "Mittwoch", thu: "Donnerstag", fri: "Freitag", sat: "Samstag", sun: "Sonntag" };
    expect(within(screen.getByRole("radiogroup", { name: "Wochentag" })).getByRole("radio", { name: dayNames[day] }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByText(/^Nächste Ausführung: .* · in \d+ Tagen · Zeitzone Europe\/Berlin \((MEZ|MESZ)\)$/) !== null).toBe(true);

    // Retention: size limit on, invalid value blocks saving, count limit off.
    await userEvent.click(screen.getByRole("switch", { name: "Gesamtgröße begrenzen" }));
    const size = screen.getByRole("spinbutton", { name: "Höchste Gesamtgröße in GB als Zahl" });
    await userEvent.clear(size);
    await userEvent.type(size, "0");
    expect(screen.getByText("Ganze Zahl von 1 bis 100000 GB angeben.") !== null && (screen.getByRole("button", { name: "Plan speichern" }) as HTMLButtonElement).disabled).toBe(true);
    await userEvent.clear(size);
    await userEvent.type(size, "25");
    await userEvent.click(screen.getByRole("switch", { name: "Anzahl der Archive begrenzen" }));
    expect(screen.getByText("Neueste Archive bis zusammen 25 GB bleiben erhalten, unabhängig von ihrer Anzahl.") !== null).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: "Plan speichern" }));
    await waitFor(async () => expect((await adapter.getVolume(HA)).backup?.retention).toEqual({ maxCount: null, maxSizeGb: 25 }));
    expect((await adapter.getVolume(HA)).backup?.schedule).toEqual({ mode: "weekly", day, time: "04:15", from: picked });
  });

  it("rejects an invalid plan in the adapter as well", async () => {
    const adapter = createDemoAdapter();
    const v = await adapter.getVolume(HA);
    const err = await adapter.setBackupPolicy(HA, { ...v.backup!, schedule: { mode: "once", date: addDays(zonedParts(new Date(), TZ).date, -2), time: "03:00" } }).catch((e: { code?: string }) => e.code);
    expect(err).toBe("invalid-plan");
    expect(await adapter.setBackupPolicy(HA, { ...v.backup!, retention: { maxCount: 0, maxSizeGb: null } }).catch((e: { code?: string }) => e.code)).toBe("invalid-plan");
  });

  it("saves default retention in the hub settings and reads it back", async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/backups"], adapter);
    await screen.findByRole("switch", { name: "Gesamtgröße begrenzen" });
    await userEvent.click(screen.getByRole("switch", { name: "Gesamtgröße begrenzen" }));
    const size = screen.getByRole("spinbutton", { name: "Höchste Gesamtgröße in GB als Zahl" });
    await userEvent.clear(size);
    await userEvent.type(size, "30");
    expect(screen.getByText(/^Höchstens 7 Archive und höchstens 30 GB/) !== null).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(async () => expect((await adapter.getSettings()).backups.defaultRetention).toEqual({ maxCount: 7, maxSizeGb: 30 }));

    // Both off is an explicit "unlimited", not a hidden archive number.
    await userEvent.click(screen.getByRole("switch", { name: "Gesamtgröße begrenzen" }));
    await userEvent.click(screen.getByRole("switch", { name: "Anzahl der Archive begrenzen" }));
    expect(screen.getByText(/^Unbegrenzt: Docklet löscht keine Archive/) !== null && screen.queryByRole("slider") === null).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(async () => expect((await adapter.getSettings()).backups.defaultRetention).toEqual({ maxCount: null, maxSizeGb: null }));
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
