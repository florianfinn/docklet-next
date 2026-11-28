// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original client-side pattern folding, UI and regression evidence for M47.

import type { LogLine } from "@/domain/model";

function word(i: number): string { return i ? word(Math.floor(i / 26)) + String.fromCharCode(97 + i % 26) : "a"; }
/** Opt-in fixtures keep the default demo fleet and its live subscriptions unchanged. */
export function patternFixture(variable: boolean, service: string): LogLine[] {
  const end = Date.now();
  return Array.from({ length: variable ? 6000 : 1200 }, (_, i) => ({
    id: `pattern-${i}`, ts: new Date(end - (6000 - i) * 1000).toISOString(), service, level: "info",
    message: variable ? `INFO scanned ${word(i)}` : i % 5 === 0 ? "ERROR request failed" : i % 5 === 1 ? "  at worker (example.js:42)" : `INFO served request ${i} from 192.0.2.1`,
  }));
}
