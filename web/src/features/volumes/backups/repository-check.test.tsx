// Source: docklet-next/issues/571
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original repository check run kind, stage labels and validated result proof.

import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { mapActivityDetail } from "@/data/http/map-dto";
import { renderRun } from "@/data/message-code";

const at = "2026-11-22T10:00:00Z";
const steps = ["connect", "save", "open", "check"].map((name, index) => ({ id: `s${index}`, name, status: "success" as const, startedAt: at, endedAt: at }));

function check(overrides: Record<string, unknown> = {}, activity: Record<string, unknown> = {}) {
  const metadata = { action: "check_volume_backup_repository", targetId: "local", backupTargetId: "local", backupTargetCheck: { hostId: "0", setVersion: 1, configVersion: 0, activityId: "check-run", ok: true, snapshotCount: 3, at, ...overrides } };
  const detail = { activity: { id: "check-run", type: "resource_action", environmentId: "0", status: "success", resourceType: "backup_target", resourceId: "local", resourceName: "local", startedAt: at, createdAt: at, metadata, ...activity }, messages: [], output: [], stepTrace: steps, outputTruncated: false };
  return mapActivityDetail(Object.freeze(detail) as never);
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

it("keeps ordinary backup runs on the backup kind", () => {
  const run = check({}, { metadata: { action: "create_volume_backup" } });
  expect(run.kind).toBe("backup");
  expect(run.repositoryCheck).toBeUndefined();
});
