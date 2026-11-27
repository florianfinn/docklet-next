// Source: docklet-next/docs/design/image-prune-progress.md
// Revision: b637b8c19497e52b7a62987a497ca4ee05b1b5af
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Image cleanup activity mapping and bulk validation coverage for issue #661.

import { expect, it } from "vitest";
import { formatBulkPruneCounts } from "@/features/activity/bulk-check-counts";
import { mapActivitySummary } from "./map-dto";

const at = "2026-11-27T10:00:00Z";
const bulk = { unit: "target", planned: 4, plannedKnown: true, completed: 2, succeeded: 1, failed: 1, skipped: 0, updates: 0 };
const summary = (value: unknown) => mapActivitySummary({ id: "prune", environmentId: "edge", type: "image_prune", resourceType: "image", status: "running", startedAt: at, createdAt: at, bulk: value } as never);

it("maps the producer's image_prune activity to a cleanup run with its counts", () => {
  expect(summary(bulk)).toMatchObject({ kind: "prune", sourceOperation: "image_prune", hostId: "edge", title: { code: "runMessages.activityTitles.imagePrune" }, bulk });
});

it.each([
  ["fractional count", { ...bulk, completed: 1.5 }],
  ["negative count", { ...bulk, failed: -1 }],
  ["missing count", { ...bulk, skipped: undefined }],
  ["text count", { ...bulk, planned: "4" }],
  ["unknown unit", { ...bulk, unit: "image" }],
  ["results beyond completed", { ...bulk, succeeded: 3 }],
  ["completed beyond plan", { ...bulk, completed: 5, succeeded: 4 }],
])("drops %s so progress stays indeterminate", (_, value) => {
  expect(summary(value).bulk).toBeUndefined();
});

it("formats cleanup counts as processed while running and removed at the end, leaving out zero parts", () => {
  const t = (key: string, values?: { count: number; planned?: number }) => key === "separator" ? " · " : `${key}:${values?.count}${values?.planned === undefined ? "" : `/${values.planned}`}`;
  expect(formatBulkPruneCounts({ ...bulk, unit: "target" }, true, t)).toBe("processed:2/4 · failed:1");
  expect(formatBulkPruneCounts({ ...bulk, unit: "target", completed: 4, succeeded: 3, skipped: 0 }, false, t)).toBe("removed:3 · failed:1");
});
