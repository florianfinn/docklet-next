// Source: docklet-next/issues/389
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proofs for skipped-host retention and maintenance decoding in the backup adapter.

import { expect, it } from "vitest";

import { backupFixture } from "./backups.test-support";
import { skippedResponse } from "./dispatch-gate";

const deepFreeze = <T>(value: T): T => {
  if (value && typeof value === "object" && !Object.isFrozen(value)) { Object.freeze(value); for (const child of Object.values(value)) deepFreeze(child); }
  return value;
};

it("R2-k a skipped host keeps its previous overview rows as unavailable in new objects", async () => {
  const f = backupFixture();
  const before = deepFreeze(await f.api.listBackupOverviews());
  f.intercept(async (url) => url.pathname.includes("/second/") && url.pathname.endsWith("/overview") ? skippedResponse() : undefined);
  const after = await f.api.listBackupOverviews(undefined, "poll");
  const second = after.find((row) => row.hostId === "second");
  expect(second?.unavailable).toBe(true);
  expect(second?.volumes).toHaveLength(1);
  expect(after.find((row) => row.hostId === "first")?.unavailable).toBeFalsy();
  expect(before.find((row) => row.hostId === "second")?.unavailable).toBeFalsy();
  expect(second === before.find((row) => row.hostId === "second")).toBe(false);
});

it("R2-d a backup write answered by the maintenance gate carries backup.maintenance", async () => {
  const f = backupFixture();
  f.intercept(async (_url, init) => init?.method === "POST" ? new Response(JSON.stringify({ title: "Service Unavailable", status: 503, detail: "backup.maintenance" }), { status: 503, headers: { "Content-Type": "application/problem+json" } }) : undefined);
  await expect(f.api.runBackup("first~data")).rejects.toMatchObject({ outcome: { code: "backup.maintenance", status: 503 } });
});
