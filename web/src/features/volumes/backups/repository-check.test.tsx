// Source: docklet-next/issues/571
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original repository check run kind, stage labels, validated result and consumer presentation proof.

import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale, type AppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapActivityDetail } from "@/data/http/map-dto";
import produced from "@/data/http/testdata/repository-check-activity.json";
import storedCheck from "@/data/http/testdata/repository-check-stored.json";
import { renderRun, type CodedActionRun } from "@/data/message-code";
import { RunFacts, RunStatusLine } from "@/features/activity/RunFocus";
import { renderWithApp } from "@/test/render";
import { BackupOperation } from "./BackupOperation";

const at = "2026-11-22T10:00:00Z";
type Step = { id: string; name: string; status: "success" | "running" | "failed"; startedAt: string; endedAt?: string };
const steps: Step[] = ["connect", "save", "open", "check"].map((name, index) => ({ id: `s${index}`, name, status: "success", startedAt: at, endedAt: at }));
afterEach(() => setAppLocale("de"));

function freeze<T>(value: T): T {
  if (value && typeof value === "object") { Object.values(value).forEach(freeze); Object.freeze(value); }
  return value;
}
function check(overrides: Record<string, unknown> = {}, activity: Record<string, unknown> = {}, trace = steps) {
  const metadata = { action: "check_volume_backup_repository", targetId: "local", backupTargetId: "local", backupTargetCheck: { hostId: "0", setVersion: 1, configVersion: 0, activityId: "check-run", ok: true, snapshotCount: 3, at, ...overrides } };
  const detail = { activity: { id: "check-run", type: "resource_action", environmentId: "0", status: "success", resourceType: "backup_target", resourceId: "local", resourceName: "local", startedAt: at, createdAt: at, metadata, ...activity }, messages: [], output: [], stepTrace: trace, outputTruncated: false };
  return mapActivityDetail(freeze(detail) as never);
}
/** Producer bytes re-labelled under another registration: the reader must not accept a foreign host's result. */
function underEnvironment(shape: { activity: object }, environmentId: string) {
  const copy = structuredClone(shape) as { activity: Record<string, unknown> };
  Object.assign(copy.activity, { environmentId, sourceEnvironmentId: environmentId });
  return mapActivityDetail(freeze(copy) as never);
}

it("names a repository check by its own kind and stage labels in both locales", () => {
  for (const locale of ["en-US", "de"] as const) {
    setAppLocale(locale);
    const run = renderRun(check(), locale);
    expect(run.kind).toBe("repository-check");
    expect(run.title).toBe(catalogs[locale].activity.runKind["repository-check"]);
    expect(run.steps.map((s) => s.label)).toEqual(["connect", "save", "open", "check"].map((name) => catalogs[locale].volumes.backups.repositoryCheck.steps[name as "connect"]));
  }
});

it("claims a snapshot count only for this run's successful check of its own target", () => {
  expect(check().repositoryCheck).toEqual({ snapshotCount: 3 });
  expect(check({ snapshotCount: 0 }).repositoryCheck).toEqual({ snapshotCount: 0 });
  for (const [overrides, activity] of [
    [{ activityId: "other-run" }, {}],
    [{ ok: false, code: "volume.backup.repository-check-failed" }, {}],
    [{ snapshotCount: -1 }, {}],
    [{ snapshotCount: 1.5 }, {}],
    [{ snapshotCount: Number.MAX_SAFE_INTEGER + 2 }, {}],
    [{ snapshotCount: "3" }, {}],
    [{}, { status: "failed" }],
    [{}, { resourceId: "other-target" }],
  ] as const) {
    expect(check(overrides, activity).repositoryCheck).toBeUndefined();
  }
});

it("binds the result to the run's host and a valid timestamp", () => {
  // Producer bytes: a hub-local check, and a stored-target check run by an agent worker after hub delivery,
  // collected and relayed by the hub over Direct and Edge (repository_remote_test.go).
  expect(mapActivityDetail(freeze(structuredClone(produced.hub)) as never).repositoryCheck).toEqual({ snapshotCount: 2 });
  expect(storedCheck.activity.environmentId).toBe("env-7");
  expect(mapActivityDetail(freeze(structuredClone(storedCheck)) as never).repositoryCheck).toEqual({ snapshotCount: 2 });
  expect(underEnvironment(produced.agent, "env-7").repositoryCheck).toEqual({ snapshotCount: 2 });
  // An explicitly different registration never receives the count, also for a stored target.
  expect(underEnvironment(storedCheck, "env-8").repositoryCheck).toBeUndefined();
  expect(underEnvironment(produced.agent, "env-8").repositoryCheck).toBeUndefined();
  // Before any target set arrived an agent records "0"; that fallback is valid only for its local target.
  expect(check({}, { environmentId: "env-7" }).repositoryCheck).toEqual({ snapshotCount: 3 });
  const stored = { resourceId: "archive", environmentId: "env-7", metadata: { action: "check_volume_backup_repository", targetId: "archive", backupTargetId: "archive", backupTargetCheck: { hostId: "0", activityId: "check-run", ok: true, snapshotCount: 3, at } } };
  expect(check({}, stored).repositoryCheck).toBeUndefined();
  expect(check({ hostId: "env-8" }, { environmentId: "env-7" }).repositoryCheck).toBeUndefined();
  for (const value of [undefined, "", "yesterday", "2026-13-45T99:00:00Z", 1763805600000]) {
    expect(check({ at: value }).repositoryCheck).toBeUndefined();
  }
  expect(produced.agent.activity.environmentId).toBe("0");
});

it("keeps ordinary backup runs on the backup kind", () => {
  const run = check({}, { metadata: { action: "create_volume_backup" } });
  expect(run.kind).toBe("backup");
  expect(run.repositoryCheck).toBeUndefined();
});

const running = (reached: number) => steps.slice(0, reached).map((step, index) => ({ ...step, status: index === reached - 1 ? "running" as const : "success" as const, endedAt: index === reached - 1 ? undefined : at }));
const failing = (reached: number) => steps.slice(0, reached).map((step, index) => ({ ...step, status: index === reached - 1 ? "failed" as const : "success" as const }));
function focus(run: CodedActionRun, locale: AppLocale) {
  setAppLocale(locale);
  const rendered = renderRun(run, locale);
  const view = render(<IntlProvider locale={locale} messages={catalogs[locale]}><MemoryRouter><RunStatusLine run={rendered} /><RunFacts run={rendered} now={Date.parse(at)} /></MemoryRouter></IntlProvider>);
  const result = { status: view.getByRole("status").textContent, progress: view.container.querySelector("[data-fact=progress] dd")?.textContent };
  view.unmount();
  return result;
}

it("shows the stage, failing stage or result in the run focus without a stage count", () => {
  for (const locale of ["en-US", "de"] as const) {
    const labels = catalogs[locale].volumes.backups.repositoryCheck.steps;
    const failedAt = (label: string) => catalogs[locale].activity.failedAt.replace("{label}", label);
    const result = locale === "de" ? "Repository geprüft · 3 Snapshots" : "Repository checked · 3 snapshots";
    expect(focus(check({}, { status: "running", metadata: { action: "check_volume_backup_repository" } }, running(2)), locale)).toEqual({ status: labels.save, progress: labels.save });
    expect(focus(check(), locale)).toEqual({ status: result, progress: result });
    expect(focus(check({}, { status: "failed", error: "volume.backup.repository-check-failed" }, failing(4)), locale)).toEqual({ status: failedAt(labels.check), progress: failedAt(labels.check) });
    // A success without a validated result claims nothing beyond the generic completion.
    expect(focus(check({ hostId: "env-8" }), locale)).toEqual({ status: catalogs[locale].activity.dialog.result.succeeded, progress: "–" });
  }
});

it("gives the operation line a stage, a failing stage or a terminal state instead of a percentage", async () => {
  setAppLocale("de");
  const labels = catalogs.de.volumes.backups.repositoryCheck.steps;
  for (const [run, expected] of [
    [check({}, { status: "running", metadata: { action: "check_volume_backup_repository" } }, running(3)), labels.open],
    [check(), "Repository geprüft · 3 Snapshots"],
    [check({}, { status: "failed" }, failing(2)), catalogs.de.activity.failedAt.replace("{label}", labels.save)],
    [check({}, { status: "failed" }, []), catalogs.de.activity.status.failed],
    [check({}, { status: "cancelled" }, []), catalogs.de.activity.status.cancelled],
  ] as const) {
    const adapter = { ...createDemoAdapter(), observeActionDetail: vi.fn((_host: string, _id: string, listener: (state: never) => void) => { listener({ status: "ready", run } as never); return () => {}; }) };
    const view = renderWithApp(<BackupOperation hostId="0" activityId="check-run" />, adapter);
    await waitFor(() => expect(screen.getByRole("status").querySelector("span")?.textContent).toBe(`${catalogs.de.volumes.backups.operation}: ${expected}`));
    view.unmount();
  }
});
