// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original service widget card state tests for issues #332 and #333 (SW9, SW11, SW13).

import { act, screen, waitFor } from "@testing-library/react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { ServiceKind, WidgetResult, WidgetSummary } from "@/domain/service-widgets";
import { queryKeys } from "@/data/query-keys";
import { renderWithApp } from "@/test/render";

import { ServiceWidgetBody } from "./services/ServiceWidgets";

const summary = (kind: ServiceKind, extra: Partial<WidgetSummary> = {}): WidgetSummary => ({ kind, configured: true, enabled: true, openUrl: "", ...extra });
function adapterWith(result: (kind: ServiceKind) => WidgetResult) {
  const adapter = createDemoAdapter();
  return { ...adapter, getServiceWidget: vi.fn(async (kind: ServiceKind) => result(kind)), decideMediaRequest: vi.fn(async () => {}) };
}

describe("service widget card states", () => {
  it("offers administrators the settings link for a widget that is not set up or turned off, and never fetches it", async () => {
    const adapter = adapterWith(() => { throw new Error("no fetch expected"); });
    const { unmount } = renderWithApp(<ServiceWidgetBody kind="plex" summary={summary("plex", { configured: false })} admin />, adapter);
    expect((await screen.findByRole("link", { name: "Einrichten" })).getAttribute("href")).toBe("/settings/serviceWidgets");
    expect(screen.getByText("Nicht eingerichtet") !== null).toBe(true);
    unmount();
    renderWithApp(<ServiceWidgetBody kind="plex" summary={summary("plex", { enabled: false })} admin={false} />, adapter);
    expect(await screen.findByText("Ausgeschaltet") !== null).toBe(true);
    expect(screen.queryByRole("link", { name: "Einrichten" })).toBe(null);
    expect(adapter.getServiceWidget).not.toHaveBeenCalled();
  });

  it("renders a failure from its hub code and stale data with its age and the current error", async () => {
    const adapter = adapterWith((kind) => kind === "plex"
      ? { kind, stale: false, ageSeconds: 0, error: { code: "widget.timeout" } }
      : { kind, stale: true, ageSeconds: 600, error: { code: "widget.unreachable" }, minecraft: { online: 2, max: 10, motd: "Example", version: "1.21", players: ["alpha", "beta"] } });
    const { unmount } = renderWithApp(<ServiceWidgetBody kind="plex" summary={summary("plex")} admin />, adapter);
    expect(await screen.findByText("Der Dienst hat nicht rechtzeitig geantwortet.") !== null).toBe(true);
    unmount();
    renderWithApp(<ServiceWidgetBody kind="minecraft" summary={summary("minecraft")} admin />, adapter);
    const status = await screen.findByRole("status");
    expect(status.textContent).toBe("Daten von vor 10 Minuten· Der Dienst hat nicht geantwortet.");
    expect(screen.getByRole("list", { name: "Spieler" }).children.length).toBe(2);
  });

  it("lets only administrators approve or decline a request, then refreshes the widget", async () => {
    const overseerr: WidgetResult = { kind: "overseerr", stale: false, ageSeconds: 0, overseerr: { pending: 1, approved: 0, available: 0, requests: [{ id: 7, title: "Example Film", mediaType: "movie", requestedBy: "alex", createdAt: new Date(Date.now() - 3 * 3_600_000).toISOString() }] } };
    const adapter = adapterWith(() => overseerr);
    const { unmount } = renderWithApp(<ServiceWidgetBody kind="overseerr" summary={summary("overseerr")} admin={false} />, adapter);
    expect(await screen.findByText("Example Film") !== null).toBe(true);
    expect(screen.queryByRole("button", { name: "Example Film freigeben" })).toBe(null);
    unmount();
    renderWithApp(<ServiceWidgetBody kind="overseerr" summary={summary("overseerr", { openUrl: "https://requests.example.com" })} admin />, adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Example Film freigeben" }));
    await waitFor(() => expect(adapter.decideMediaRequest).toHaveBeenCalledWith(7, "approve"));
    await waitFor(() => expect(adapter.getServiceWidget.mock.calls.length).toBeGreaterThanOrEqual(3));
    const open = screen.getByRole("link", { name: "Overseerr öffnen" });
    expect([open.getAttribute("href"), open.getAttribute("target"), open.getAttribute("rel")]).toEqual(["https://requests.example.com", "_blank", "noopener noreferrer"]);
  });

  it("keeps the last good data with its age and a read failure when a refetch fails, and recovers on the next answer", async () => {
    let fail = false;
    const adapter = adapterWith((kind) => {
      if (fail) throw new Error("hub unreachable");
      return { kind, stale: false, ageSeconds: 0, minecraft: { online: 2, max: 10, motd: "Example server", version: "1.21", players: ["alpha"] } };
    });
    let client: QueryClient | undefined;
    function Probe() { client = useQueryClient(); return <ServiceWidgetBody kind="minecraft" summary={summary("minecraft")} admin />; }
    renderWithApp(<Probe />, adapter);
    expect(await screen.findByText("Example server") !== null).toBe(true);
    expect(screen.queryByRole("status")).toBe(null);
    fail = true;
    await act(() => client!.refetchQueries({ queryKey: queryKeys.serviceWidget("minecraft") }));
    expect((await screen.findByRole("status")).textContent).toMatch(/Der Hub war nicht erreichbar/);
    expect(screen.getByText("Example server") !== null).toBe(true);
    expect(document.querySelector('[data-slot="alert"]')).toBe(null);
    fail = false;
    await act(() => client!.refetchQueries({ queryKey: queryKeys.serviceWidget("minecraft") }));
    await waitFor(() => expect(screen.queryByRole("status")).toBe(null));
  });
});
