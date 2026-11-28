// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original client-side pattern folding, UI and regression evidence for M47.

import type { LogLine } from "@/domain/model";
import { PatternFold, PATTERN_BYTES, PATTERN_LINES } from "./patterns";

/** The admission caps include already folded lines, so queued messages cannot exceed the read budget. */
export class PatternQueue {
  readonly fold = new PatternFold();
  private queue: (LogLine | undefined)[] = [];
  private cursor = 0;
  private bytes = 0;
  private lines = 0;
  private stopped = false;
  enqueue(lines: LogLine[]) {
    if (this.stopped) return;
    for (const line of lines) {
      const size = new TextEncoder().encode(line.message).length;
      if (this.lines >= PATTERN_LINES || this.bytes + size > PATTERN_BYTES) break;
      this.queue.push(line); this.lines++; this.bytes += size;
    }
  }
  get pending() { return this.queue.length - this.cursor; }
  slice(now = () => performance.now()) {
    const start = now();
    while (!this.stopped && this.cursor < this.queue.length && now() - start < 8) {
      const line = this.queue[this.cursor]!;
      this.queue[this.cursor++] = undefined;
      this.fold.add(line);
    }
    if (!this.pending) { this.queue = []; this.cursor = 0; }
  }
  clear() { this.stopped = true; this.queue = []; this.cursor = 0; }
}
