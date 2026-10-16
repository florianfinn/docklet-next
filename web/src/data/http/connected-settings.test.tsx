// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original connected settings persistence and in-flight draft tests.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useConnectedSettings } from "@/data/queries";
import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { renderRoutes } from "@/test/render";
import { GeneralSection } from "@/features/settings/sections/GeneralSection";
import SettingsPage from "@/features/settings/SettingsPage";

import { createHttpAdapter } from "./http-adapter";

const initial = {
  setupCompleted: "true", hubName: "Docklet", hubAddress: "192.0.2.10:8080", domainUrl: "", linkBase: "auto", timeZone: "Europe/Berlin",
  keepActionOutput: "true", actionPresentation: "dialog",
};

function connectedServer() {
  const settings: Record<string, string> = { ...initial };
  const writes: Record<string, string>[] = [];
  let release: (() => void) | undefined;
  let pending = false;
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path === "/api/auth/mode") return new Response(JSON.stringify({ success: true, data: { mode: "local" } }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
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
  const schedule = useScheduleText();
  const next = schedule.next({ mode: "daily", time: "09:00" }, new Date("2026-10-17T12:00:00Z"), zone);
  return <p data-testid="schedule-zone">{next?.zone}</p>;
}

function GeneralWithSchedule() {
  const settings = useConnectedSettings();
  return <><ScheduleProbe />{settings.data && <GeneralSection settings={settings.data} />}</>;
}

afterEach(() => { localStorage.clear(); });

describe("connected hub settings", () => {
  it("reads settings back through the HTTP adapter and updates schedule time zone", async () => {
    const server = connectedServer();
    renderRoutes([{ path: "/", element: <GeneralWithSchedule /> }], ["/"], server.adapter);
    expect((await screen.findByTestId("schedule-zone")).textContent?.includes("Europe/Berlin")).toBe(true);
    (await screen.findByRole("combobox", { name: "Zeitzone" })).focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.click(await screen.findByRole("option", { name: "UTC" }));
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(screen.getByTestId("schedule-zone").textContent?.includes("UTC")).toBe(true));
    expect(server.writes.at(-1)?.timeZone).toBe("UTC");
    expect((await server.adapter.getConnectedSettings()).general.timeZone).toBe("UTC");
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
});
