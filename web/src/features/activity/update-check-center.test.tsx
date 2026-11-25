// Source: docklet-next/issues/626
// Revision: 505d1715fdf1dc1668655e9affef78b67620f7d9
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original bulk check presentation and producer-shaped regression coverage.

import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { updateFixture } from "@/data/http/stack-updates.test-support";
import { renderWithApp } from "@/test/render";
import { ActivityCenter } from "./ActivityCenter";
import { activityStore } from "./activity-store";

const at = "2026-11-25T12:00:00Z";
afterEach(() => { cleanup(); activityStore.reset(); setAppLocale("de"); });
it("shows all-host counts and target rows in the expanded Activity Centre entry", async () => {
  setAppLocale("en-US"); const f = updateFixture(), original = f.fetcher.getMockImplementation()!;
  const activity = { id: "example-check", type: "job_run", environmentId: "0", resourceType: "job", resourceId: "update-check", status: "success", startedAt: at, createdAt: at, endedAt: at, metadata: { jobId: "update-check", runId: "example-job", jobStatus: "succeeded" }, bulk: { unit: "target", planned: 2, plannedKnown: true, completed: 2, succeeded: 1, skipped: 1, failed: 0, updates: 1 } };
  const steps = ["running", "succeeded"].map((name) => ({ id: `step-${name}`, name, status: "success", startedAt: at, endedAt: at }));
  f.fetcher.mockImplementation(async (input, init) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (path.endsWith("/0/activities")) return Response.json({ success: true, data: [{ ...activity, stepTrace: steps }], pagination: { totalItems: 1 } });
    if (path.endsWith("/activities/example-check")) return Response.json({ success: true, data: { activity, stepTrace: steps, output: [], messages: [], outputTruncated: false } });
    if (path.endsWith("/jobs/update-check/runs/example-job")) return Response.json({ id: "example-job", environmentId: "0", jobId: "update-check", outcome: { targets: [{ id: "direct~stack~project", name: "Example stack", environmentId: "direct", targetKind: "stack", targetId: "project", status: "skipped", code: "update.image-not-pulled" }] } });
    return original(input, init);
  });
  f.adapter.listStacks = async () => []; f.adapter.listContainers = async () => [];
  await f.adapter.getCurrentUser(); await f.adapter.listActions();
  activityStore.set((s) => ({ ...s, open: true, tab: "actions", focusRunId: activity.id }));
  renderWithApp(<ActivityCenter />, f.adapter);
  await screen.findByText("Example stack");
  await waitFor(() => expect(document.querySelector('[data-slot="bulk-check-result"]')?.textContent).toBe("1 of 2 checked · 1 with updates · 1 skipped"));
  expect(document.querySelector('[data-job-target] [data-tone="warn"]') !== null).toBe(true);
  expect(document.querySelector('[data-layer="activity"]')?.textContent).not.toContain("Error at");
});
