// Source: docklet-next/issues/583
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original activity-addressed candidate transport read proofs.

import { expect, test } from "vitest";
import { isCancelledError } from "@tanstack/react-query";
import { readCandidateRun } from "./vulnerabilities";
import { vulnerabilityTransport } from "./vulnerability-transport";

const run = { id: "run", activityId: "old-activity", trigger: "automatic", status: "partial", startedAt: "2026-11-23T10:00:00Z", completedAt: "2026-11-23T10:01:00Z", targets: 8, scanned: 2, cached: 1, failed: 1, deferred: 2 };

test("reads exactly the selected host and retained activity, including cached and shared-proof counts", async () => {
  const paths: string[] = [];
  const request = vulnerabilityTransport("/api", { token: async () => "example", fetch: async (input) => { paths.push(String(input)); return Response.json({ success: true, data: run }); } }, () => 0);
  const result = await readCandidateRun(request, "example/host", "old-activity");
  expect(paths).toEqual(["/api/environments/example%2Fhost/vulnerabilities/candidates/runs/old-activity"]);
  expect(result).toMatchObject({ activityId: "old-activity", targets: 8, scanned: 2, cached: 1, failed: 1, deferred: 2 });
  expect(result).not.toHaveProperty("done");
});
test("retention 404 is unavailable without another run's counts", async () => {
  const request = vulnerabilityTransport("/api", { token: async () => "example", fetch: async () => Response.json({ code: "vulnerability.run-not-found", status: 404 }, { status: 404 }) }, () => 0);
  await expect(readCandidateRun(request, "example", "old-activity")).resolves.toBeUndefined();
});
test.each([{ activityId: "newer-activity" }, { targets: -1 }, { scanned: null }, { status: "future" }])("rejects unrelated or malformed stored counters %#", async (change) => {
  const request = vulnerabilityTransport("/api", { token: async () => "example", fetch: async () => Response.json({ success: true, data: { ...run, ...change } }) }, () => 0);
  await expect(readCandidateRun(request, "example", "old-activity")).rejects.toThrow();
});
test("identity replacement fences a held response", async () => {
  let identity = 0, finish!: (r: Response) => void;
  const request = vulnerabilityTransport("/api", { token: async () => "example", fetch: async () => new Promise((resolve) => { finish = resolve; }) }, () => identity);
  const pending = readCandidateRun(request, "example", "old-activity").catch((e: unknown) => e);
  ++identity; finish(Response.json({ success: true, data: run }));
  expect(isCancelledError(await pending)).toBe(true);
});
