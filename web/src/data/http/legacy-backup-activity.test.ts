// Source: docklet-next/issues/340
// Revision: 4155c8fe73dc2c562ddd41075b20e0304f879f5b
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original regression test for a retained backup job row without backup context (M17 acceptance M17-A1).

import { expect, it } from "vitest";

import { renderRun } from "../message-code";
import { mapActivitySummary } from "./map-dto";

type Summary = Parameters<typeof mapActivitySummary>[0];

// Shape of a retained scheduled volume backup job row as the hub lists it.
const legacy = (error: string) => ({
  id: "run-legacy", type: "job_run", resourceType: "job", resourceId: "volume-backup:policy",
  environmentId: "0", status: "failed", error, messageCode: "job.failed",
  startedAt: "2026-10-30T08:01:00Z", endedAt: "2026-10-30T08:01:05Z", createdAt: "2026-10-30T08:01:00Z",
}) as unknown as Summary;

it.each([
  ["de", "Das Backup wurde unterbrochen."],
  ["en-US", "The backup was interrupted."],
] as const)("translates a known backup code on a job row without backup context in %s", (locale, text) => {
  expect(renderRun(mapActivitySummary(legacy("volume.backup.interrupted")), locale).error).toBe(text);
});

it("keeps unknown job errors out of the backup catalogue", () => {
  expect(mapActivitySummary(legacy("volume.backup.not-a-code")).error?.code.startsWith("volumes.backupCodes.")).toBe(false);
});
