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
/** A preset reaches back from the moment the read starts; a custom range is fixed. */
export type PatternRange = { hours: number } | { since: string; until: string };
/** Selectable line limits of a pattern read (K-8); the largest is the agreed cap. */
export const PATTERN_LIMITS = [10_000, 50_000, PATTERN_LINES] as const;
// Completed reads per window stay available while the app runs (K-8). The key carries the log and
// identity generations, so a new session or viewer never sees another's result; only folded
// patterns are kept, never raw lines.
const CACHE_ENTRIES = 8;
const caches = new WeakMap<object, Map<string, { retry: number; view: PatternView }>>();
const cacheOf = (owner: object) => { let cache = caches.get(owner); if (!cache) caches.set(owner, (cache = new Map())); return cache; };
export function usePatternRead(kind: "stack" | "container", id: string, range: PatternRange, retry: number, limit: number = PATTERN_LINES) {
  const adapter = useAdapter();
  const lifetime = logLifetime(adapter.subscribeLogs);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const identity = identityLifetime(adapter).version();
  const windowKey = JSON.stringify([kind, id, range, limit, generation, identity]);
  const key = JSON.stringify([windowKey, retry]);
  const cached = cacheOf(adapter.subscribeLogs).get(windowKey);
  const hit = cached?.retry === retry ? cached.view : undefined;
  const [state, setState] = useState<{ key: string; adapter: typeof adapter; view: PatternView }>();
  useEffect(() => {
    if (hit) return;
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
      const view = { rows: fold.snapshot(), lines: fold.lines, bytes: fold.bytes,
        first: Number.isFinite(fold.first) ? new Date(fold.first).toISOString() : undefined,
        last: Number.isFinite(fold.last) ? new Date(fold.last).toISOString() : undefined,
        result: queue.pending ? undefined : result };
      setState({ key, adapter, view });
      // Only complete or capped reads are remembered; an interrupted or failed read is read again.
      if (view.result && (view.result.reason === "end" || view.result.reason === "cap-lines" || view.result.reason === "cap-bytes")) {
        const cache = cacheOf(adapter.subscribeLogs);
        cache.delete(windowKey); cache.set(windowKey, { retry, view });
        for (const old of cache.keys()) if (cache.size > CACHE_ENTRIES) cache.delete(old);
      }
      if (queue.pending) frame = requestAnimationFrame(publish);
    };
    const schedule = () => { if (active && !frame) frame = requestAnimationFrame(publish); };
    if (!identityLifetime(adapter).active()) return () => { off(); offIdentity(); clear(); };
    const bounds = "hours" in range ? { since: new Date(Date.now() - range.hours * 3600_000).toISOString() } : { since: range.since, until: range.until };
    void adapter.readLogs({ kind, id }, { ...bounds, tail: limit, maxLines: limit, maxBytes: PATTERN_BYTES }, (lines) => {
      if (active) { queue.enqueue(lines); schedule(); }
    }, controller.signal).then((value) => {
      if (!active) return;
      if (value.reason === "aborted") { reset(); return; }
      result = value; schedule();
    });
    return () => { off(); offIdentity(); clear(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps -- `key` carries the range, limit and retry by value.
  }, [adapter, key, kind, id, lifetime, Boolean(hit)]);
  return hit ?? (state?.key === key && state.adapter === adapter ? state.view : undefined);
}
