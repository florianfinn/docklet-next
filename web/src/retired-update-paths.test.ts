// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original UP13 structural guard over actual production clients.

import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { it, expect } from "vitest";
function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => e.isDirectory() ? e.name === "generated" ? [] : sources(join(dir, e.name)) : /\.tsx?$/.test(e.name) && !/\.(test|test-support)\./.test(e.name) ? [join(dir, e.name)] : []);
}
it("UP13 production clients contain no retired update routes or settings keys", () => {
  const retired = /policies\/project\/|updater\/schedules|getUpdateSchedules|saveUpdateSchedule|["'](?:autoUpdateInterval|pollingInterval|pollingEnabled|autoUpdateEnabled|vulnerabilityScanEnabled)["']/;
  for (const path of sources("src")) expect(retired.test(readFileSync(path, "utf8")), path).toBe(false);
});
