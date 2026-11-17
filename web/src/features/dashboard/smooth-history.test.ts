// Source: docklet-next/issues/509
// Revision: fb1306767563a1e0d7bd028aaef362cca38166b6
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original smoothing proofs for host card history.

import { describe, expect, it } from "vitest";

import { smoothHistory } from "./smooth-history";

describe("host history smoothing", () => {
  it("damps a single-sample spike", () => {
    const smoothed = smoothHistory([10, 10, 10, 100, 10, 10]);
    expect(Math.max(...smoothed)).toBeLessThan(50);
    expect(smoothed[3]).toBeGreaterThan(10);
  });

  it("follows a sustained step within a few samples", () => {
    const smoothed = smoothHistory([10, 10, 80, 80, 80, 80, 80]);
    expect(smoothed.at(-1)).toBeGreaterThan(73);
    expect(smoothed.at(-1)).toBeLessThanOrEqual(80);
  });

  it("keeps length, a constant series and the input unchanged", () => {
    const input = [25, 25, 25];
    expect(smoothHistory(input)).toEqual([25, 25, 25]);
    expect(input).toEqual([25, 25, 25]);
    expect(smoothHistory([])).toEqual([]);
  });
});
