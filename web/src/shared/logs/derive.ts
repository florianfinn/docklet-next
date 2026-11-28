// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import type { LogLine } from "@/domain/model";
import { logGap } from "@/data/log-buffer";

export function collapseLogs(lines: readonly LogLine[]) {
  const groups: LogLine[][] = [];
  for (const line of lines) {
    const previous = groups.at(-1);
    if (logGap(line) === undefined && previous && logGap(previous[0]!) === undefined && previous[0]!.service === line.service && previous[0]!.message === line.message) previous.push(line);
    else groups.push([line]);
  }
  return groups;
}
export function densityBins(lines: readonly LogLine[]) {
  const times = lines.map((l) => Date.parse(l.ts)).filter(Number.isFinite);
  const first = Math.min(...times), last = Math.max(...times);
  const width = Math.max(1, last - first) / 60;
  const bins = Array.from({ length: 60 }, (_, i) => ({ start: first + i * width, end: first + (i + 1) * width, errors: 0, warnings: 0, firstId: undefined as string | undefined }));
  for (const line of lines) {
    if (logGap(line) !== undefined) continue;
    const index = Math.min(59, Math.floor((Date.parse(line.ts) - first) / width));
    const bin = bins[index];
    if (!bin) continue;
    bin.firstId ??= line.id;
    if (line.level === "error") bin.errors++;
    if (line.level === "warn") bin.warnings++;
  }
  return bins;
}
