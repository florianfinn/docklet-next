// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed update UI and regression coverage.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { updateFixture, point } from "@/data/http/stack-updates.test-support";
import { response } from "@/data/http/runtime-commands.test-support";
import { renderWithApp } from "@/test/render";
import { ConnectedUpdatesSection } from "@/features/settings/sections/ConnectedUpdatesSection";
import { presentationStore } from "@/features/activity/run-presentation";
import { ContainerUpdates } from "@/features/containers/ContainerUpdates";
import { DeployRecovery } from "./deploy-recovery";
import { renderRun } from "@/data/message-code";
import { mapActivityDetail } from "@/data/http/map-dto";
import recoveryFixture from "@/data/http/testdata/update-recovery.json";
import { runtimeDetail, snapshot } from "@/data/http/runtime-commands.test-support";
import targetFixture from "@/data/http/testdata/update-target-outcomes.json";
import { JobUpdateOutcomes } from "./job-update-outcomes";
import type { ContainerInfo } from "@/domain/model";

vi.mock("@/shared/hooks/use-fit-view", () => ({ useFitView: () => ({ view: "table", ref: { current: null } }) }));
afterEach(() => { setAppLocale("de"); presentationStore.reset(); });
for (const locale of ["en-US", "de"] as const) {
  it(`F5 ${locale} one policy table across hosts has separate columns, check-only dash and per-row segmented save`, async () => {
    setAppLocale(locale); const f = updateFixture(); await f.adapter.getCurrentUser();
    renderWithApp(<ConnectedUpdatesSection />, f.adapter);
    const t = catalogs[locale].settings.updates;
    const table = await screen.findByRole("table", { name: t.table });
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
    expect(screen.getAllByRole("table", { name: t.table })).toHaveLength(1);
    expect(within(table).getByRole("columnheader", { name: t.mode }) !== null).toBe(true);
    expect(within(table).getByRole("columnheader", { name: t.window }) !== null).toBe(true);
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows[0]!.textContent).toContain("direct"); expect(rows[1]!.textContent).toContain("other");
    for (const row of rows) expect(within(row).getByText("–") !== null).toBe(true);
    const schedule = within(rows[1]!).getByRole("radio", { name: t.scheduled });
    await waitFor(() => expect(schedule.hasAttribute("disabled")).toBe(false));
    await userEvent.click(schedule);
    await waitFor(() => expect(f.writes().filter((c) => c.init.method === "PUT")).toHaveLength(1));
    const write = f.writes().find((c) => c.init.method === "PUT")!;
    expect(write.path).toContain("/environments/other/updater/policies/stack/project");
    expect(JSON.parse(String(write.init.body))).toEqual({ mode: "scheduled", expectedRevision: 1, window: { kind: "weekly", day: 0, time: "04:00" } });
    const help = within(table).getByRole("button", { name: t.window });
    help.focus(); await userEvent.keyboard("{Tab}{Shift>}{Tab}{/Shift}");
    await waitFor(() => expect(document.body.textContent).toContain(t.windowHelp));
  });
  it(`F5 ${locale} renders the backend next check in the non-UTC hub zone and binds Check all now to the action toast`, async () => {
    setAppLocale(locale); const f = updateFixture(); const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input, init = {}) => String(input).endsWith("/environments/0/settings") ? response(Object.entries({ effectiveTimeZone: "America/New_York", timeZone: "America/New_York", updateCheckInterval: "12h", updateNotify: "false" }).map(([key, value]) => ({ key, value }))) : original(input, init));
    await f.adapter.getCurrentUser();
    renderWithApp(<ConnectedUpdatesSection />, f.adapter);
    const t = catalogs[locale].settings.updates;
    const check = await screen.findByRole("button", { name: t.checkNow });
    const expected = new Intl.DateTimeFormat(locale, { timeZone: "America/New_York", dateStyle: "medium", timeStyle: "short" }).format(new Date(f.state.job.nextRun));
    expect(document.body.textContent).toContain(expected);
    expect(document.body.textContent).toContain("America/New_York");
    await waitFor(() => expect(check.hasAttribute("disabled")).toBe(false)); await userEvent.click(check);
    await waitFor(() => expect(presentationStore.get().byRun[f.run.id]).toBe("toast"));
  });
  it(`F5 ${locale} disabled deployment recovery states why it is held`, async () => {
    setAppLocale(locale); const f = updateFixture(); f.state.points = [{ ...point, holdState: "active" }]; await f.adapter.getCurrentUser();
    const at = point.createdAt;
    const run = renderRun(mapActivityDetail({ activity: { id: "example-run", type: "project_deploy", resourceType: "project", resourceId: "project", environmentId: "direct", status: "failed", startedAt: at, createdAt: at, imageLifecycle: { runId: "example-run", restorePointId: point.id, verified: false, volumeDataIncluded: false } }, messages: [], output: [], stepTrace: [], outputTruncated: false }), locale);
    renderWithApp(<DeployRecovery run={run} />, f.adapter);
    const button = await screen.findByRole("button", { name: catalogs[locale].common.stackUpdates.previousDeployment });
    await screen.findByText(catalogs[locale].common.stackUpdates.recoveryHeld);
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(button.title).toBe(catalogs[locale].common.stackUpdates.recoveryHeld);
  });
}
it("F5 no empty container box before any gesture or actual control is available", async () => {
  const f = updateFixture(); f.adapter.getContainerUpdatePolicy = () => new Promise(() => {}); f.adapter.getContainerPreviousImage = () => new Promise(() => {}); await f.adapter.getCurrentUser();
  const c = { id: "direct~example", hostId: "direct", name: "example", origin: "standalone", image: { repository: "example", tag: "1" }, update: { state: "unchecked" } } as ContainerInfo;
  const view = renderWithApp(<ContainerUpdates c={c} select={() => {}} />, f.adapter);
  await act(async () => { await Promise.resolve(); });
  expect(view.container.querySelector("section") === null).toBe(true);
});
for (const locale of ["en-US", "de"] as const) it.each(targetFixture.filter((target) => target.code !== "update.unstable"))(`W-1 ${locale} actual job detail renders Go-produced target $code from the job endpoint`, async (target) => {
  const code = target.code;
  setAppLocale(locale); const f = updateFixture(); const original = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init = {}) => String(input).includes("/jobs/update-window%3Adirect/runs/example-job") ? response({ id: "example-job", jobId: "update-window:direct", environmentId: "0", outcome: { status: "failed", targets: [target] } }) : original(input, init));
  await f.adapter.getCurrentUser();
  const recovery = recoveryFixture.find((row) => row.recovery.outcome === ({ "update.auto-restored": "restored", "update.auto-restore-failed": "restore-failed", "update.auto-restore-unavailable": "unavailable", "update.interrupted": "interrupted" } as Record<string, string>)[code]);
  f.state.hold = (path) => path.includes("/activities/example-run") || path.includes("/runtime-runs/example-run") ? Promise.resolve(response({ success: true, data: runtimeDetail(snapshot("direct", { id: "example-run", status: "failed", imageLifecycle: recovery as never })) })) : undefined;
  const at = point.createdAt;
  const run = renderRun(mapActivityDetail({ activity: { id: "example", type: "job_run", environmentId: "0", status: "failed", startedAt: at, createdAt: at, metadata: { jobId: "update-window:direct", runId: "example-job" } }, messages: [], output: [], stepTrace: [], outputTruncated: false }), locale);
  renderWithApp(<JobUpdateOutcomes run={run} />, f.adapter);
  const key = code.replace(".", "_") as keyof typeof catalogs.de.images.errors;
  await screen.findByText(catalogs[locale].images.errors[key], { exact: false });
  if (code === "update.candidate-failed") expect(document.body.textContent).toContain(catalogs[locale].activity.jobStep.skipped);
  else {
    expect(document.body.textContent).toContain(catalogs[locale].common.stackUpdates.volumeNotRestored);
    await waitFor(() => expect(document.querySelector(`[data-job-target="${target.id}"] [data-update-recovery="${recovery!.recovery.outcome}"]`) !== null).toBe(true));
    if (code === "update.auto-restore-unavailable") expect(document.body.textContent).toContain(catalogs[locale].images.errors["restore_files-changed"]);
  }
});

it.each([
  ["en-US", "Scheduled updates and “Update with check”", "If an update fails, nothing is rolled back automatically."],
  ["de", "Geplante Updates und „Update mit Prüfung“", "Schlägt ein Update fehl, wird nichts automatisch zurückgesetzt."],
] as const)("F5 %s distinguishes observed recovery from ordinary manual updates", async (locale, observed, obsolete) => {
  setAppLocale(locale); const f = updateFixture(); await f.adapter.getCurrentUser();
  renderWithApp(<ConnectedUpdatesSection />, f.adapter);
  await screen.findByRole("button", { name: catalogs[locale].settings.updates.checkNow });
  expect(document.body.textContent).toContain(observed);
  expect(document.body.textContent).not.toContain(obsolete);
});
