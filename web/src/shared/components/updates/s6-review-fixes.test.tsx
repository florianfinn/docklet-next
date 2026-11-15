// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original regression coverage for observed update UI review follow-ups.

import { screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { updateFixture, point } from "@/data/http/stack-updates.test-support";
import { response, runtimeDetail, snapshot } from "@/data/http/runtime-commands.test-support";
import { renderWithApp } from "@/test/render";
import { ConnectedUpdatesSection } from "@/features/settings/sections/ConnectedUpdatesSection";
import { DeployRecovery } from "./deploy-recovery";
import { JobUpdateOutcomes } from "./job-update-outcomes";
import { renderRun } from "@/data/message-code";
import { mapActivityDetail } from "@/data/http/map-dto";
import recoveryFixture from "@/data/http/testdata/update-recovery.json";
import targetFixture from "@/data/http/testdata/update-target-outcomes.json";

vi.mock("@/shared/hooks/use-fit-view", () => ({ useFitView: () => ({ view: "table", ref: { current: null } }) }));
afterEach(() => setAppLocale("de"));
const at = point.createdAt;
const jobRun = (jobId: string) => renderRun(mapActivityDetail({ activity: { id: "example", type: "job_run", environmentId: "0", status: "failed", startedAt: at, createdAt: at, metadata: { jobId, runId: "example-job" } }, messages: [], output: [], stepTrace: [], outputTruncated: false }), "en-US");

it("a failed job target whose child proved no workload effect omits the volume warning", async () => {
  setAppLocale("en-US"); const f = updateFixture(); const original = f.fetcher.getMockImplementation()!;
  const target = targetFixture.find((t) => t.code === "update.unstable")!;
  const recovery = recoveryFixture.find((row) => row.recovery.outcome === "not-needed");
  f.fetcher.mockImplementation(async (input, init = {}) => String(input).includes("/jobs/update-window%3Adirect/runs/example-job") ? response({ id: "example-job", jobId: "update-window:direct", environmentId: "0", outcome: { status: "failed", targets: [target] } }) : original(input, init));
  f.state.hold = (path) => path.includes("/activities/example-run") || path.includes("/runtime-runs/example-run") ? Promise.resolve(response({ success: true, data: runtimeDetail(snapshot("direct", { id: "example-run", status: "failed", imageLifecycle: recovery as never })) })) : undefined;
  await f.adapter.getCurrentUser();
  renderWithApp(<JobUpdateOutcomes run={jobRun("update-window:direct")} />, f.adapter);
  await waitFor(() => expect(document.querySelector(`[data-job-target="${target.id}"] [data-update-recovery="not-needed"]`) !== null).toBe(true));
  const box = document.querySelector(`[data-job-target="${target.id}"]`)!;
  expect(box.textContent?.includes(catalogs["en-US"].common.stackUpdates.volumeNotRestored)).toBe(false);
  // The box names the target, not the qualified merge key.
  expect(box.textContent?.startsWith(`${target.targetId} · `)).toBe(true);
});

it("only update-window job runs read job targets", () => {
  expect(jobRun("update-window:direct").jobRef !== undefined).toBe(true);
  expect(jobRun("update-check").jobRef === undefined).toBe(true);
});

it("pending restore points do not claim that none were recorded", async () => {
  setAppLocale("en-US"); const f = updateFixture();
  let release!: () => void; const gate = new Promise<void>((resolve) => { release = resolve; });
  const original = f.fetcher.getMockImplementation()!;
  let requested = false;
  f.fetcher.mockImplementation(async (input, init = {}) => { if (String(input).includes("restore-points")) { requested = true; await gate; } return original(input, init); });
  await f.adapter.getCurrentUser();
  const run = renderRun(mapActivityDetail({ activity: { id: "example-run", type: "project_deploy", resourceType: "project", resourceId: "project", environmentId: "direct", status: "failed", startedAt: at, createdAt: at, imageLifecycle: { runId: "example-run", restorePointId: point.id, verified: false, volumeDataIncluded: false } }, messages: [], output: [], stepTrace: [], outputTruncated: false }), "en-US");
  renderWithApp(<DeployRecovery run={run} />, f.adapter);
  const button = await screen.findByRole("button", { name: catalogs["en-US"].common.stackUpdates.previousDeployment });
  await waitFor(() => expect(requested).toBe(true));
  expect(document.body.textContent?.includes(catalogs["en-US"].common.stackUpdates.noPoints)).toBe(false);
  expect(button.hasAttribute("disabled")).toBe(true);
  release();
});

it("a host without policy access shows the refusal, not a read failure", async () => {
  setAppLocale("en-US"); const f = updateFixture(); f.state.denied.add("listUpdatePolicies"); await f.adapter.getCurrentUser();
  renderWithApp(<ConnectedUpdatesSection />, f.adapter);
  await screen.findAllByText(catalogs["en-US"].common.query.unavailableTitle);
  expect(document.body.textContent?.includes(catalogs["en-US"].common.query.errorTitle)).toBe(false);
});
