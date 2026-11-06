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

// A settled job without an outcome message carries "<job name>: <state>" as error and latest message.
const settled = (state: string, message = `Update check: ${state}`) => {
  const detail = job("update-check", "Update check") as unknown as { activity: Record<string, unknown> };
  Object.assign(detail.activity, { status: "failed", endedAt: "2026-11-05T12:01:00Z", messageCode: `job.${state}`, latestMessage: message, error: message });
  return detail as unknown as Detail;
};

it.each([
  ["partial", "Teilweise abgeschlossen"],
  ["failed", "Fehlgeschlagen"],
  ["needs_attention", "Braucht Aufmerksamkeit"],
])("localises the %s summary of a settled hub update check", (state, text) => {
  const run = renderRun(mapActivityDetail(settled(state)), "de");
  expect([run.error, run.log]).toEqual([text, [text]]);
});

it("keeps a real job outcome message", () => {
  expect(renderRun(mapActivityDetail(settled("failed", "Registry example.com refused the request")), "de").error).toBe("Registry example.com refused the request");
});

it("localises the persisted message history of a settled hub update check", () => {
  const detail = settled("partial") as unknown as { messages: unknown[] };
  detail.messages = [{ id: "example-message", activityId: "run", level: "info", message: "Update check: partial", code: "job.partial", step: "partial", createdAt: "2026-11-05T12:01:00Z" }];
  const run = renderRun(mapActivityDetail(detail as unknown as Detail), "de");
  expect([run.error, run.log]).toEqual(["Teilweise abgeschlossen", ["Teilweise abgeschlossen"]]);
});

// Producer texts of carrier codes stay data in run logs (#357 S0R1-1): outcome, activity.message and event detail.
it("keeps a real job outcome in the latest message and the persisted history", () => {
  const outcome = "Registry example.com refused the request";
  const detail = settled("failed", outcome) as unknown as { messages: unknown[] };
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    expect(renderRun(mapActivityDetail(detail as unknown as Detail), "de").log).toEqual([outcome]);
    detail.messages = [{ id: "example-message", activityId: "run", level: "error", message: outcome, code: "job.failed", step: "failed", createdAt: "2026-11-05T12:01:00Z" }];
    expect(renderRun(mapActivityDetail(detail as unknown as Detail), "de").log).toEqual([outcome]);
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});

it.each(["project_update", "project_restore"])("keeps activity.message and event detail text in a %s log", (type) => {
  const at = "2026-11-05T12:00:00Z";
  const line = (id: string, code: string, message: string) => ({ id, activityId: "run", level: "info", message, code, step: "pull", createdAt: at });
  const detail = {
    activity: { id: "run", type, environmentId: "example", resourceType: "project", resourceId: "app", resourceName: "app", status: "running", startedAt: at, createdAt: at },
    stepTrace: [], output: [], outputTruncated: false,
    messages: [line("m1", "activity.message", "Pulling example/app:1.2.3"), line("m2", "deploy.pull", "Pulled example/app:1.2.3"), line("m3", "container.start", "Started app-web-1"), line("m4", "run.started", "Started")],
  } as unknown as Detail;
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    expect(renderRun(mapActivityDetail(detail), "de").log).toEqual(["Pulling example/app:1.2.3", "Pulled example/app:1.2.3", "Started app-web-1", "Läuft"]);
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});
