// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original client-side pattern folding, UI and regression evidence for M47.

import type { LogLevel, LogLine } from "@/domain/model";
import { classifyLevel } from "./level";

export const PATTERN_LINES = 100_000;
export const PATTERN_BYTES = 20 * 1024 * 1024;
export const PATTERN_LIMIT = 5_000;
const encoder = new TextEncoder();
// ANSI control sequences are removed before continuation detection and keying.
// eslint-disable-next-line no-control-regex -- ANSI escape and bell bytes delimit terminal control sequences.
export const stripAnsi = (text: string) => text.replace(/\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))/g, "");
export const continuation = (text: string) => /^(?:\s|at |Caused by|Traceback|File ")/.test(stripAnsi(text));
export function patternKey(line: Pick<LogLine, "message">): string {
  const text = stripAnsi(line.message);
  if (continuation(text)) return "";
  return text
    .replace(/\b\d{4}[-/]\d{2}[-/]\d{2}(?:[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)?\b/g, "<time>")
    .replace(/\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi, "<uuid>")
    .replace(/(?:\b[0-9a-f]{1,4})?(?::[0-9a-f]{0,4}){2,}(?:%[\w-]+)?/gi, "<ip>")
    .replace(/\b\d{1,3}(?:\.\d{1,3}){3}\b/g, "<ip>")
    .replace(/\b(?:0x)?[0-9a-f]{8,}\b/gi, "<hex>")
    .replace(/"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'/g, "<quoted>")
    .replace(/\b\d+(?:\.\d+)?\b/g, "<number>");
}
export interface LogPattern {
  key: string;
  other: boolean;
  level: LogLevel;
  count: number;
  first: string;
  last: string;
  bins: number[];
  samples: string[];
}
interface Accumulator extends Omit<LogPattern, "bins"> { times: number[] }
export class PatternFold {
  private patterns = new Map<string, Accumulator>();
  private heads = new Map<string, Accumulator>();
  private other?: Accumulator;
  lines = 0;
  bytes = 0;
  first = Infinity;
  last = -Infinity;
  reason?: "cap-lines" | "cap-bytes";
  add(line: LogLine): boolean {
    if (this.reason) return false;
    const bytes = encoder.encode(line.message).length;
    if (this.bytes + bytes > PATTERN_BYTES) { this.reason = "cap-bytes"; return false; }
    this.lines++; this.bytes += bytes;
    const time = Date.parse(line.ts);
    if (Number.isFinite(time)) { this.first = Math.min(this.first, time); this.last = Math.max(this.last, time); }
    const key = patternKey(line);
    let row = key ? this.patterns.get(key) : this.heads.get(line.service);
    if (!row && key) {
      const fresh = (): Accumulator => ({ key, other: false, level: classifyLevel(stripAnsi(line.message)), count: 0, first: line.ts, last: line.ts, samples: [], times: [] });
      if (this.patterns.size < PATTERN_LIMIT) { row = fresh(); this.patterns.set(key, row); }
      else { row = this.other ??= { ...fresh(), key: "", other: true }; }
    }
    // An orphan continuation is counted in the overflow row, never as its own shape.
    if (!row) row = this.other ??= { key: "", other: true, level: "info", count: 0, first: line.ts, last: line.ts, samples: [], times: [] };
    if (key) this.heads.set(line.service, row);
    row.count++;
    if (line.ts < row.first) row.first = line.ts;
    if (line.ts > row.last) row.last = line.ts;
    if (Number.isFinite(time)) row.times.push(time);
    if (key && row.samples.length < 3 && !row.samples.includes(stripAnsi(line.message))) row.samples.push(stripAnsi(line.message));
    if (this.lines >= PATTERN_LINES) this.reason = "cap-lines";
    else if (this.bytes >= PATTERN_BYTES) this.reason = "cap-bytes";
    return true;
  }
  snapshot(): LogPattern[] {
    return [...this.patterns.values(), ...(this.other ? [this.other] : [])].map(({ times, ...row }) => {
      const bins = Array<number>(30).fill(0);
      for (const time of times) bins[Math.min(29, Math.floor((time - this.first) / Math.max(1, this.last - this.first) * 30))]!++;
      return { ...row, samples: [...row.samples], bins };
    }).sort((a, b) => b.count - a.count);
  }
}
