// Source: docklet-next/issues/341
// Revision: 593699500ef01a82fef720cc7217ed19eaaae7c0
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original job run localisation test from the hub job activity producer shape.

import { expect, it, vi } from "vitest";

import { renderRun } from "../message-code";
import { mapActivityDetail } from "./map-dto";

type Detail = Parameters<typeof mapActivityDetail>[0];

// Shape of a running hub update check: the English job name is the resource name and
// raw scheduler states are the step names (activity/service_projection.go jobActivityInternal).
const job = (resourceId: string, resourceName: string) => ({
  activity: { id: "run", type: "job_run", environmentId: "0", resourceType: "job", resourceId, resourceName, status: "running", step: "running", startedAt: "2026-11-05T12:00:00Z", createdAt: "2026-11-05T12:00:00Z" },
  stepTrace: [
    { name: "queued", code: "job.queued", status: "success", startedAt: "2026-11-05T12:00:00Z", endedAt: "2026-11-05T12:00:01Z" },
    { name: "running", code: "job.running", status: "running", startedAt: "2026-11-05T12:00:01Z" },
  ],
  messages: [], output: [], outputTruncated: false,
}) as unknown as Detail;

it.each([
  ["de", "Update-Prüfung", ["In Warteschlange", "Läuft"]],
  ["en-US", "Update check", ["Queued", "Running"]],
] as const)("localises the hub update check run in %s", (locale, target, steps) => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    const run = renderRun(mapActivityDetail(job("update-check", "Update check")), locale);
    expect([run.kind, run.title, run.target, run.steps.map((step) => step.label)]).toEqual(["check-updates", target, target, steps]);
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});

it("keeps the name of a job without a catalogued kind", () => {
  expect(renderRun(mapActivityDetail(job("example-job", "Example job")), "de").target).toBe("Example job");
});
