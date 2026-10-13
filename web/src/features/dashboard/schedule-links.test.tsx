import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { jobHref } from "@/domain/job-links";
import SettingsPage from "@/features/settings/SettingsPage";
import { renderRoutes } from "@/test/render";

import { ScheduleWidget } from "./widgets/SmallWidgets";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const now = Date.parse("2026-10-12T10:00:00Z");
const at = (min: number) => new Date(now + min * 60_000).toISOString();

describe("next jobs", () => {
  it("map each job type to the page that configures it, from typed ids only", () => {
    expect([
      jobHref({ kind: "update-check" }),
      jobHref({ kind: "scan" }),
      jobHref({ kind: "backup", volumeId: "vol-atlas-media_media_config", stackId: "media" }),
      jobHref({ kind: "backup" }),
      jobHref({ kind: "auto-update", stackId: "paperless" }),
      jobHref({ kind: "auto-update" }),
    ]).toEqual([
      "/settings/updates#update-check",
      "/settings/security#scan-schedule",
      "/volumes/vol-atlas-media_media_config?tab=backups",
      "/backups",
      "/stacks/paperless?tab=overview#auto-update",
      "/settings/updates#update-policies",
    ]);
  });

  it("are focusable links with a meaningful name and open the update settings at the check interval", async () => {
    const jobs = [
      { id: "j1", kind: "update-check" as const, title: "Update-Prüfung aller Hosts", nextRunAt: at(38), cadence: "alle 6 Stunden" },
      { id: "j4", kind: "auto-update" as const, title: "Automatische Updates · Paperless", hostId: "atlas", stackId: "paperless", nextRunAt: at(300), cadence: "Mi 03:30" },
    ];
    const { router } = renderRoutes(
      [
        { path: "/", element: <ScheduleWidget jobs={jobs} hosts={new Map()} now={now} /> },
        { path: "/settings/:section", element: <SettingsPage /> },
      ],
      ["/"],
    );
    const links = within(screen.getByRole("list")).getAllByRole("link");
    expect(links.map((l) => [l.getAttribute("href"), l.getAttribute("aria-label")])).toEqual([
      ["/settings/updates#update-check", "Update-Prüfung aller Hosts, in 38 min – Update-Einstellungen öffnen"],
      ["/stacks/paperless?tab=overview#auto-update", "Automatische Updates · Paperless, in 5 h – Update-Bereich des Stacks öffnen"],
    ]);
    await userEvent.tab();
    expect(document.activeElement === links[0]).toBe(true);
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(router.state.location.pathname).toBe("/settings/updates"));
    await waitFor(() => expect(document.activeElement?.id).toBe("update-check"));
  });
});
