// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original client-side pattern folding, UI and regression evidence for M47.

import { expect, it } from "vitest";
import { PatternQueue } from "./pattern-queue";
import { patternFixture } from "@/data/demo/pattern-fixtures";

it("measures 100000 synthetic lines through the production slice scheduler", () => {
  const queue = new PatternQueue(), fixture = patternFixture(false, "example");
  queue.enqueue(Array.from({ length: 100000 }, (_, i) => fixture[i % fixture.length]!));
  let longest = 0, slices = 0;
  const start = performance.now();
  while (queue.pending) {
    const before = performance.now(); queue.slice();
    longest = Math.max(longest, performance.now() - before); slices++;
  }
  const total = performance.now() - start;
  process.stdout.write(JSON.stringify({ environment: "vitest-node-desktop", lines: queue.fold.lines, totalMs: total, longestSliceMs: longest, slices }) + "\n");
  expect(queue.fold.lines).toBe(100000);
});
