// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import { expect, it } from "vitest";
import { gapLine } from "@/data/log-buffer";
import type { LogLine } from "@/domain/model";
import { collapseLogs, densityBins } from "./derive";
const line = (id: string, service = "web", message = "same", level: LogLine["level"] = "info"): LogLine => Object.freeze({ id, ts: `2026-01-01T00:00:0${id}Z`, service, message, level });
it("derives repeats and sixty bins without mutating a frozen published snapshot", () => {
  const lines = Object.freeze([line("2"), line("1"), line("3", "worker"), line("4", "web", "same", "error"), line("5", "web", "other", "warn")]);
  expect(() => collapseLogs(lines)).not.toThrow();
  expect(collapseLogs(lines).map((g) => g.length)).toEqual([2, 1, 1, 1]);
  const bins = densityBins(lines);
  expect(bins).toHaveLength(60);
  expect(bins.reduce((n, b) => n + b.errors + b.warnings, 0)).toBe(2);
  expect(lines.map((l) => l.id)).toEqual(["2", "1", "3", "4", "5"]);
});
it("never collapses gap lines", () => {
  const gap = gapLine("gap", "2026-01-01T00:00:00Z", 2);
  expect(collapseLogs([gap, gap]).map((g) => g.length)).toEqual([1, 1]);
});
