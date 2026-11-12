// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original connected settings persistence and in-flight draft tests. M19 web consumer integration. #410 simple-user hub zone.

import { useQuery } from "@tanstack/react-query";
import { useFormatter } from "use-intl";

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useConnectedSettings } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { setAppLocale } from "@/app/i18n/locale-store";
import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { renderRoutes } from "@/test/render";
import { GeneralSection } from "@/features/settings/sections/GeneralSection";
import SettingsPage from "@/features/settings/SettingsPage";

import { createHttpAdapter } from "./http-adapter";

const initial = {
  setupCompleted: "true", hubName: "Docklet", hubAddress: "192.0.2.10:8080", domainUrl: "", linkBase: "auto", effectiveTimeZone: "Europe/Berlin",
  defaultHostConnection: "direct", agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/projects/docklet-agent", defaultPuid: "", defaultPgid: "", detectSocketGroup: "true",
  keepActionOutput: "true", actionPresentation: "dialog", activityEventLimit: "500", activityRunLimit: "100",
};

function connectedServer(zone = "Europe/Berlin") {
  const settings: Record<string, string> = { ...initial, effectiveTimeZone: zone };
  const writes: Record<string, string>[] = [];
  let release: (() => void) | undefined;
  let pending = false;
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode: "local" } }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path === "/api/capabilities") return new Response(JSON.stringify({ success: true, data: [{ operationId: "saveSettings", available: true, resourceRef: { kind: "hub", id: "0" } }] }));
    if (path === "/api/environments/0/settings") {
      if (init?.method === "PUT") {
        expect(new Headers(init.headers).get("X-CSRF-Token")).toBe("example".padEnd(43, "x"));
        const values = JSON.parse(String(init.body)) as Record<string, string>;
        writes.push(values);
        if (pending) await new Promise<void>((resolve) => { release = resolve; });
        Object.assign(settings, values);
        return new Response(JSON.stringify({ success: true, data: [] }));
      }
      return new Response(JSON.stringify(Object.entries(settings).map(([key, value]) => ({ key, value, type: "text" }))));
    }
    throw new Error(`unexpected ${path}`);
  });
  return {
    adapter: createHttpAdapter({ fetch: fetcher as typeof fetch }),
    settings, writes, fetcher,
    hold: () => { pending = true; },
    release: () => { pending = false; release?.(); },
  };
}

function ScheduleProbe() {
  const zone = useHubTimeZone();
  const formatter = useFormatter();
  const schedule = useScheduleText();
  const next = schedule.next({ mode: "daily", time: "09:00" }, new Date("2026-10-17T12:00:00Z"), zone);
  return <><p data-testid="schedule-zone">{next?.zone}</p><p data-testid="schedule-instant">{next?.at.toISOString()}</p><p data-testid="formatted-date">{formatter.dateTime(new Date("2026-10-17T12:00:00Z"), { hour: "2-digit", minute: "2-digit", hour12: false })}</p></>;
}

function GeneralWithSchedule() {
  const settings = useConnectedSettings();
  return <><ScheduleProbe />{settings.data && <GeneralSection settings={settings.data} />}</>;
}

afterEach(() => { localStorage.clear(); });

describe("connected hub settings", () => {
  it.each([["Europe/Berlin", "2026-10-18T07:00:00.000Z", "14:00"], ["UTC", "2026-10-18T09:00:00.000Z", "12:00"], ["America/New_York", "2026-10-17T13:00:00.000Z", "08:00"]])("reads process zone %s for schedules and IntlProvider without writing the effective zone", async (zone, instant, time) => {
    const server = connectedServer(zone);
    renderRoutes([{ path: "/", element: <GeneralWithSchedule /> }], ["/"], server.adapter);
    await waitFor(() => expect(screen.getByTestId("schedule-zone").textContent?.includes(zone)).toBe(true));
    expect((await screen.findByLabelText("Zeitzone") as HTMLInputElement).placeholder).toBe(zone);
    expect(screen.queryByRole("combobox", { name: "Zeitzone" }) !== null).toBe(true);
    expect((await server.adapter.getConnectedSettings()).general.timeZone).toBe(zone);
    await server.adapter.saveSettings("general", { ...(await server.adapter.getConnectedSettings()).general, configuredTimeZone: "UTC" });
    expect(screen.getByTestId("schedule-instant").textContent).toBe(instant);
    expect(screen.getByTestId("formatted-date").textContent).toBe(time);
    expect(server.writes.at(-1)?.timeZone).toBe("UTC");
    expect(server.writes.at(-1)?.effectiveTimeZone).toBeUndefined();
  });

  it("keeps General input typed while an HTTP save is in flight", async () => {
    const server = connectedServer();
    server.hold();
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/general"], server.adapter);
    const name = await screen.findByLabelText("Name des Hubs") as HTMLInputElement;
    await userEvent.clear(name);
    await userEvent.type(name, "First");
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(server.writes).toHaveLength(1));
    await userEvent.type(name, "Later");
    server.release();
    await waitFor(() => expect(screen.getByRole("status").textContent).toBe("Ungespeicherte Änderungen"));
    expect(name.value).toBe("FirstLater");
    expect(server.settings.hubName).toBe("First");
  });

  it("keeps Activity input changed while an HTTP save is in flight", async () => {
    const server = connectedServer();
    server.hold();
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/activity"], server.adapter);
    const output = await screen.findByRole("switch", { name: /Ausgaben von Aktionen speichern/ });
    await userEvent.click(output);
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(server.writes).toHaveLength(1));
    await userEvent.click(output);
    server.release();
    await waitFor(() => expect(screen.getByRole("status").textContent).toBe("Ungespeicherte Änderungen"));
    expect((output as HTMLButtonElement).getAttribute("data-state")).toBe("checked");
    expect(server.settings.keepActionOutput).toBe("false");
  });

  it("writes load alert settings as hub keys and blocks a value outside the hub range", async () => {
    const server = connectedServer();
    Object.assign(server.settings, { loadAlertsEnabled: "true", loadAlertCpuPercent: "85", loadAlertMemoryPercent: "90", loadAlertMinutes: "10" });
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/activity"], server.adapter);
    const minutes = await screen.findByLabelText("Minuten über dem Schwellwert");
    expect((screen.getByLabelText("CPU-Schwellwert (%)") as HTMLInputElement).value).toBe("85");
    await userEvent.clear(minutes);
    await userEvent.type(minutes, "4");
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    expect(await screen.findByText("Bitte eine Zahl von 5 bis 60 eingeben.")).toBeTruthy();
    expect(server.writes).toHaveLength(0);
    await userEvent.type(minutes, "5");
    await userEvent.click(screen.getByRole("switch", { name: /Warnungen bei anhaltender Last/ }));
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(server.writes).toHaveLength(1));
    expect(server.writes[0]).toMatchObject({ loadAlertsEnabled: "false", loadAlertCpuPercent: "85", loadAlertMemoryPercent: "90", loadAlertMinutes: "45" });
  });
});


it("goal 8: zone picker saves the configured zone and resets to TZ", async () => {
  const server = connectedServer("America/New_York");
  renderRoutes([{ path: "/", element: <GeneralWithSchedule /> }], ["/"], server.adapter);
  const zone = await screen.findByRole("combobox", { name: "Zeitzone" });
  await userEvent.type(zone, "UTC"); await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(server.writes).toHaveLength(1));
  expect(server.writes[0]!.timeZone).toBe("UTC");
  await userEvent.click(screen.getByRole("button", { name: /TZ verwenden/ }));
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(server.writes).toHaveLength(2));
  expect(server.writes[1]!.timeZone).toBe("");
  expect(document.querySelector('datalist option[value="UTC"]') !== null).toBe(true);
  expect(document.querySelector('datalist option[value="America/New_York"]') !== null).toBe(true);
});


it("the shared General picker also persists an explicit zone during first-run setup", async () => {
  const server = connectedServer();
  const settings = await server.adapter.getConnectedSettings();
  await server.adapter.completeSetup({ general: { ...settings.general, configuredTimeZone: "UTC" }, hostDefaults: settings.hostDefaults, auth: { mode: "none", local: { username: "", passwordSet: false }, proxy: { userHeader: "", emailHeader: "", groupsHeader: "", trustedProxies: "", logoutUrl: "" } } });
  expect(server.writes.at(-1)?.timeZone).toBe("UTC");
});

it("reads and writes configured retention counts through the real settings HTTP mapper", async () => {
  const server = connectedServer();
  Object.assign(server.settings, { activityEventLimit: "123", activityRunLimit: "27" });
  const settings = await server.adapter.getConnectedSettings();
  expect(settings.activity).toMatchObject({ maxEvents: 123, maxFinishedActions: 27 });
  await server.adapter.saveSettings("activity", { ...settings.activity, maxEvents: 321, maxFinishedActions: 42 });
  expect(server.writes.at(-1)).toMatchObject({ activityEventLimit: "321", activityRunLimit: "42" });
  expect((await server.adapter.getConnectedSettings()).activity).toMatchObject({ maxEvents: 321, maxFinishedActions: 42 });
});

function UserSchedule() {
  // AuthGate owns the principal read in the app; the probe performs it here.
  const adapter = useAdapter();
  const user = useQuery({ queryKey: queryKeys.currentUser, queryFn: () => adapter.getCurrentUser() });
  return user.data ? <ScheduleProbe /> : null;
}

it.each(["en-US", "de"] as const)("#410: a simple user sees schedules in the hub zone without reading connected settings in %s", async (locale) => {
  setAppLocale(locale);
  const server = connectedServer("Asia/Tokyo");
  const adapter = { ...server.adapter, getCurrentUser: vi.fn(async () => ({ id: "member", username: "member", isAdmin: false, assignments: [], requiresPasswordChange: false })) };
  const connectedRead = vi.spyOn(adapter, "getConnectedSettings");
  renderRoutes([{ path: "/", element: <UserSchedule /> }], ["/"], adapter as typeof server.adapter);
  await waitFor(() => expect(screen.getByTestId("schedule-zone").textContent?.startsWith("Asia/Tokyo")).toBe(true));
  expect(screen.getByTestId("schedule-instant").textContent).toBe("2026-10-18T00:00:00.000Z");
  expect(screen.getByTestId("formatted-date").textContent).toBe("21:00");
  expect(connectedRead).not.toHaveBeenCalled();
});
