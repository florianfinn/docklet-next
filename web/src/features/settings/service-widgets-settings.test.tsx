// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original Settings › Service widgets gesture tests for issues #332 and #333 (SW7, SW8, SW15) and draft retention across failed refreshes.

import { act, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { queryKeys } from "@/data/query-keys";
import type { ServiceKind, WidgetConfigInput } from "@/domain/service-widgets";
import { renderRoutes } from "@/test/render";

import SettingsPage from "./SettingsPage";

const card = async (kind: ServiceKind) => (await waitFor(() => document.querySelector<HTMLElement>(`[data-widget-settings="${kind}"]`) ?? Promise.reject(new Error(`${kind} missing`)))).closest<HTMLElement>('[data-slot="card"]')!;
const certificate = { hostPort: "plex.example.com:32400", sha256: "ab".repeat(32) };

function adapter() {
  const base = createDemoAdapter();
  const saved: WidgetConfigInput[] = [];
  return {
    saved,
    adapter: {
      ...base,
      testServiceWidget: vi.fn(async (kind: ServiceKind, input: WidgetConfigInput) => (input.address.startsWith("https://") ? { kind, stale: false, ageSeconds: 0, error: { code: "widget.tls-failed" as const, certificate } } : base.testServiceWidget(kind, input))),
      saveServiceWidgetConfig: vi.fn(async (kind: ServiceKind, input: WidgetConfigInput) => { saved.push(input); return base.saveServiceWidgetConfig(kind, input); }),
    },
  };
}

describe("Settings › Service widgets", () => {
  it("tests a draft without saving, trusts the offered certificate and saves it with the kept credential", async () => {
    const f = adapter();
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/serviceWidgets"], f.adapter);
    const plex = await card("plex");
    const address = within(plex).getByLabelText("Adresse");
    await userEvent.clear(address);
    await userEvent.type(address, "http://fail.example.com");
    await userEvent.click(within(plex).getByRole("button", { name: "Verbindung testen" }));
    expect(await within(plex).findByText("Der Dienst hat nicht geantwortet.") !== null).toBe(true);
    await userEvent.clear(address);
    await userEvent.type(address, "https://plex.example.com:32400");
    await userEvent.click(within(plex).getByRole("button", { name: "Verbindung testen" }));
    expect((await within(plex).findByText(certificate.sha256)).hasAttribute("data-fingerprint")).toBe(true);
    expect(f.saved).toEqual([]);
    await userEvent.click(within(plex).getByRole("button", { name: "Diesem Zertifikat vertrauen" }));
    expect(within(plex).getByText(/Vertrautes Zertifikat für plex\.example\.com:32400/) !== null).toBe(true);
    await userEvent.click(within(plex).getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(f.saved).toHaveLength(1));
    expect(f.saved[0]).toEqual({ enabled: true, address: "https://plex.example.com:32400", openUrl: "", tlsPin: certificate, credentialAction: "keep" });
  });

  it("replaces a credential write-only and clears a stored one on request", async () => {
    const f = adapter();
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/serviceWidgets"], f.adapter);
    const overseerr = await card("overseerr");
    const key = within(overseerr).getByLabelText("API-Schlüssel") as HTMLInputElement;
    expect([key.type, key.value]).toEqual(["password", ""]);
    await userEvent.type(key, "example-key");
    await userEvent.click(within(overseerr).getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(f.saved).toHaveLength(1));
    expect([f.saved[0]!.credentialAction, f.saved[0]!.credential]).toEqual(["replace", "example-key"]);
    await waitFor(() => expect((within(overseerr).getByLabelText("API-Schlüssel") as HTMLInputElement).value).toBe(""));
    await userEvent.click(within(overseerr).getByRole("button", { name: "Gespeicherten Wert entfernen" }));
    await userEvent.click(within(overseerr).getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(f.saved).toHaveLength(2));
    expect([f.saved[1]!.credentialAction, "credential" in f.saved[1]!]).toEqual(["clear", false]);
    expect(within(await card("minecraft")).queryByLabelText(/Token|API-Schlüssel/)).toBe(null);
  });

  it("drops a late test answer once the tested draft was discarded or edited, so trust is never offered for another address", async () => {
    const f = adapter();
    const answers: ((value: Awaited<ReturnType<typeof f.adapter.testServiceWidget>>) => void)[] = [];
    f.adapter.testServiceWidget = vi.fn(() => new Promise((resolve) => answers.push(resolve)));
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/serviceWidgets"], f.adapter);
    const plex = await card("plex");
    const address = within(plex).getByLabelText("Adresse");
    const offer = { kind: "plex" as const, stale: false, ageSeconds: 0, error: { code: "widget.tls-failed" as const, certificate } };
    await userEvent.clear(address);
    await userEvent.type(address, "https://plex.example.com:32400");
    await userEvent.click(within(plex).getByRole("button", { name: "Verbindung testen" }));
    await userEvent.click(within(plex).getByRole("button", { name: "Verwerfen" }));
    await act(async () => answers[0]!(offer));
    expect(within(plex).queryByText(certificate.sha256)).toBe(null);
    await userEvent.clear(address);
    await userEvent.type(address, "https://plex.example.com:32400");
    await userEvent.click(within(plex).getByRole("button", { name: "Verbindung testen" }));
    await userEvent.type(address, "0");
    await act(async () => answers[1]!(offer));
    expect(within(plex).queryByRole("button", { name: "Diesem Zertifikat vertrauen" })).toBe(null);
    await userEvent.type(address, "{Backspace}");
    expect(within(plex).getByRole("button", { name: "Diesem Zertifikat vertrauen" }) !== null).toBe(true);
    expect(f.saved).toEqual([]);
  });

  it("does not show a test answer that arrives after the tested draft was saved", async () => {
    const f = adapter();
    const answers: ((value: Awaited<ReturnType<typeof f.adapter.testServiceWidget>>) => void)[] = [];
    f.adapter.testServiceWidget = vi.fn(() => new Promise((resolve) => answers.push(resolve)));
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/serviceWidgets"], f.adapter);
    const minecraft = await card("minecraft");
    const address = within(minecraft).getByLabelText("Adresse");
    await userEvent.clear(address);
    await userEvent.type(address, "192.0.2.30:25565");
    await userEvent.click(within(minecraft).getByRole("button", { name: "Verbindung testen" }));
    await userEvent.click(within(minecraft).getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(f.saved).toHaveLength(1));
    await act(async () => answers[0]!({ kind: "minecraft", stale: false, ageSeconds: 0, minecraft: { online: 0, max: 10, motd: "", version: "1.21", players: [] } }));
    expect(within(minecraft).queryByText(/Die Verbindung funktioniert/)).toBe(null);
  });

  it("keeps an unsaved draft and credential through a failed refresh of its configuration", async () => {
    const f = adapter();
    let fail = false;
    const getServiceWidgetConfig = vi.fn(async (kind: ServiceKind) => {
      if (kind === "overseerr" && fail) throw new Error("config read failed");
      return f.adapter.getServiceWidgetConfig(kind);
    });
    let client!: QueryClient;
    function Capture() {
      client = useQueryClient();
      return null;
    }
    renderRoutes([{ path: "/settings/:section", element: <><Capture /><SettingsPage /></> }], ["/settings/serviceWidgets"], { ...f.adapter, getServiceWidgetConfig });
    const overseerr = await card("overseerr");
    const address = within(overseerr).getByLabelText("Adresse") as HTMLInputElement;
    await userEvent.clear(address);
    await userEvent.type(address, "http://draft.example.com:5055");
    await userEvent.type(within(overseerr).getByLabelText("API-Schlüssel"), "example-key");
    fail = true;
    // Saving another kind refetches every widget configuration; this refetch of Overseerr fails.
    const minecraft = await card("minecraft");
    await userEvent.clear(within(minecraft).getByLabelText("Adresse"));
    await userEvent.type(within(minecraft).getByLabelText("Adresse"), "192.0.2.30:25565");
    await userEvent.click(within(minecraft).getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(client.getQueryState(queryKeys.serviceWidgetConfig("overseerr"))?.status).toBe("error"));
    expect(!!within(document.body).queryByRole("alert")).toBe(true);
    const form = () => (document.querySelector('[data-widget-settings="overseerr"]') as HTMLElement | null);
    expect(form() === null).toBe(false);
    fail = false;
    await act(async () => { await client.invalidateQueries({ queryKey: queryKeys.serviceWidgetConfig("overseerr") }); });
    await waitFor(() => expect(client.getQueryState(queryKeys.serviceWidgetConfig("overseerr"))?.status).toBe("success"));
    expect((within(form()!).getByLabelText("Adresse") as HTMLInputElement).value).toBe("http://draft.example.com:5055");
    expect((within(form()!).getByLabelText("API-Schlüssel") as HTMLInputElement).value).toBe("example-key");
  });
});
