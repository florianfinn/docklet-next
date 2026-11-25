// Source: docklet-next/issues/626
// Revision: 505d1715fdf1dc1668655e9affef78b67620f7d9
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original bulk check presentation and producer-shaped regression coverage.

import { cleanup } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";
import { renderWithApp } from "@/test/render";
import { ActionToastView } from "./ActionToastView";
import { runPhase } from "./run-phase";
import { RunFacts, RunStatusLine } from "./RunFocus";

const at = "2026-11-25T12:00:00Z";
const counts = { unit: "target" as const, planned: 4, plannedKnown: true, completed: 2, succeeded: 1, failed: 1, skipped: 0, updates: 1 };
function fixture(state = "running", steps = ["running"], bulk: typeof counts | null = counts) {
  return mapActivityDetail({ activity: { id: "example-run", type: "job_run", environmentId: "0", resourceType: "job", resourceId: "update-check-host", status: state === "partial" ? "failed" : state === "succeeded" ? "success" : state as "running" | "failed", startedAt: at, createdAt: at, bulk: bulk ?? undefined, metadata: { jobStatus: state } }, stepTrace: steps.map((name) => ({ id: `step-${name}`, name, status: name === "running" && state === "running" ? "running" : name === "partial" || name === "failed" ? "failed" : "success", startedAt: at })), messages: [], output: [], outputTruncated: false });
}
afterEach(() => { cleanup(); setAppLocale("de"); });
it.each(["en-US", "de"] as const)("formats known, unknown and terminal bulk counts across toast and facts (%s)", (locale) => {
  setAppLocale(locale);
  const cases = [
    // The checked part counts successes while running too, so it never drops at settlement.
    ["running", counts, locale === "de" ? "1 von 4 geprüft · 1 fehlgeschlagen" : "1 of 4 checked · 1 failed"],
    ["running", { ...counts, plannedKnown: false }, locale === "de" ? "1 bisher geprüft · 1 fehlgeschlagen" : "1 checked so far · 1 failed"],
    ["running", { ...counts, completed: 3, skipped: 1 }, locale === "de" ? "1 von 4 geprüft · 1 fehlgeschlagen · 1 übersprungen" : "1 of 4 checked · 1 failed · 1 skipped"],
    ["partial", { ...counts, completed: 4, succeeded: 2, skipped: 1 }, locale === "de" ? "2 von 4 geprüft · 1 mit Updates · 1 fehlgeschlagen · 1 übersprungen" : "2 of 4 checked · 1 with updates · 1 failed · 1 skipped"],
    ["succeeded", { ...counts, completed: 4, succeeded: 4, failed: 0, updates: 0 }, locale === "de" ? "4 von 4 geprüft" : "4 of 4 checked"],
  ] as const;
  for (const [state, bulk, text] of cases) {
    const run = renderRun(fixture(state, ["running", ...(state === "running" ? [] : [state])], bulk), locale);
    const view = renderWithApp(<><RunStatusLine run={run} /><RunFacts run={run} now={0} />{(["bar", "compact", "edge"] as const).map((design) => <ActionToastView key={design} run={run} design={design} detail="full" now={0} major={false} onClose={() => {}} onOpen={() => {}} />)}</>);
    expect(view.container.querySelector('[data-fact="progress"] dd')?.textContent).toBe(text);
    expect(view.container.querySelector('[data-fact="services"]') === null).toBe(true);
    for (const toast of view.container.querySelectorAll('[data-slot="action-toast"]')) { expect(toast.textContent).toContain(text); expect(toast.textContent).not.toContain(locale === "de" ? "Fehler bei" : "Error at"); }
    view.unmount();
  }
});
it.each([["running"], ["queued", "running"]])("counts only observed phases for %j", (...steps) => {
  setAppLocale("en-US");
  const run = renderRun(fixture("running", steps, null), "en-US");
  expect(runPhase(run).phases.map((step) => step.schedulerState)).toEqual(steps);
  const view = renderWithApp(<RunStatusLine run={run} />);
  expect(view.container.textContent).toBe(`Step ${steps.length} of ${steps.length}: Running`);
});
it("never calls a terminal outcome a failing phase but retains real phase failure", () => {
  setAppLocale("en-US");
  const run = renderRun(fixture("partial", ["queued", "running", "partial"]), "en-US");
  const view = renderWithApp(<RunStatusLine run={run} />);
  expect(view.container.textContent).toBe("1 of 4 checked · 1 with updates · 1 failed");
  view.unmount();
  const real = { ...run, sourceOperation: "project_deploy", bulk: undefined, steps: [{ key: "pull" as const, label: "Pull", state: "error" as const }] };
  const fail = renderWithApp(<RunStatusLine run={real} />);
  expect(fail.container.textContent).toBe("Error at Pull");
});

it.each(["step identity", "all-host reference"])("preserves scheduler %s", (check) => {
  const detail = { activity: { id: "example", environmentId: "0", type: "job_run", status: "running" as const, startedAt: at, createdAt: at, metadata: { jobId: "update-check", runId: "example-job-run" } }, stepTrace: [{ id: "step-running", name: "running", status: "running" as const, startedAt: at }], messages: [], output: [], outputTruncated: false };
  const run = renderRun(mapActivityDetail(detail), "en-US");
  if (check === "step identity") expect(run.steps[0]).toHaveProperty("schedulerState", "running");
  else expect(run.jobRef).toEqual({ jobId: "update-check", runId: "example-job-run" });
});

it.each(["succeeded", "partial", "failed", "skipped", "canceled", "needs_attention"])("separates the %s outcome from the stored legacy phases", (state) => {
  const run = renderRun(fixture("failed", ["queued", "running", state], null), "en-US");
  expect(runPhase(run).phases.map((step) => step.schedulerState)).toEqual(["queued", "running"]);
  expect(runPhase(run).outcome).toBe(state);
  const view = renderWithApp(<ActionToastView run={run} design="bar" detail="full" now={0} major={false} onClose={() => {}} onOpen={() => {}} />);
  expect(view.container.querySelectorAll('[data-slot="step-track"] > li').length).toBe(2);
  expect(view.container.textContent).not.toContain("Error at");
});
