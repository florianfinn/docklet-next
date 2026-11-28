// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original client-side pattern folding, UI and regression evidence for M47.

import { useEffect, useState, useSyncExternalStore } from "react";
import { useAdapter } from "@/data/use-adapter";
import { logLifetime } from "@/data/log-buffer";
import { identityLifetime } from "@/data/identity-lifetime";
import type { LogReadResult } from "@/data/api";
import { PATTERN_BYTES, PATTERN_LINES, type LogPattern } from "./patterns";
import { PatternQueue } from "./pattern-queue";

export interface PatternView { rows: LogPattern[]; lines: number; bytes: number; first?: string; last?: string; result?: LogReadResult }
export function usePatternRead(kind: "stack" | "container", id: string, hours: number, retry: number) {
  const adapter = useAdapter();
  const lifetime = logLifetime(adapter.subscribeLogs);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const key = JSON.stringify([kind, id, hours, retry, generation]);
  const [state, setState] = useState<{ key: string; adapter: typeof adapter; view: PatternView }>();
  useEffect(() => {
    const controller = new AbortController(), queue = new PatternQueue();
    let active = true, frame = 0, result: LogReadResult | undefined;
    const clear = () => { active = false; controller.abort(); cancelAnimationFrame(frame); queue.clear(); };
    const reset = () => { clear(); setState(undefined); };
    const off = lifetime.listen(reset);
    const offIdentity = identityLifetime(adapter).listen(reset);
    const publish = () => {
      frame = 0;
      if (!active) return;
      queue.slice();
      const fold = queue.fold;
      setState({ key, adapter, view: { rows: fold.snapshot(), lines: fold.lines, bytes: fold.bytes,
        first: Number.isFinite(fold.first) ? new Date(fold.first).toISOString() : undefined,
        last: Number.isFinite(fold.last) ? new Date(fold.last).toISOString() : undefined,
        result: queue.pending ? undefined : result } });
      if (queue.pending) frame = requestAnimationFrame(publish);
    };
    const schedule = () => { if (active && !frame) frame = requestAnimationFrame(publish); };
    if (!identityLifetime(adapter).active()) return () => { off(); offIdentity(); clear(); };
    void adapter.readLogs({ kind, id }, { since: new Date(Date.now() - hours * 3600_000).toISOString(), tail: PATTERN_LINES, maxLines: PATTERN_LINES, maxBytes: PATTERN_BYTES }, (lines) => {
      if (active) { queue.enqueue(lines); schedule(); }
    }, controller.signal).then((value) => {
      if (!active) return;
      if (value.reason === "aborted") { reset(); return; }
      result = value; schedule();
    });
    return () => { off(); offIdentity(); clear(); };
  }, [adapter, key, kind, id, hours, lifetime]);
  return state?.key === key && state.adapter === adapter ? state.view : undefined;
}
