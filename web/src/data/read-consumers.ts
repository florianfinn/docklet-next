// Source: docklet-next/issues/211
// Revision: 31a3237938f4b560d550c9f0a34ebc547fddde0d
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original observer-demand bridge keyed by the stable adapter subscription.

import type { DockletAdapter, Topic } from "./adapter";

type Subscribe = DockletAdapter["subscribeChanges"];
const consumers = new WeakMap<Subscribe, Map<() => boolean, Topic>>();

/** A cached query is not demand; the getter checks current enabled observers. */
export function registerReadConsumer(subscribe: Subscribe, topic: Topic, active: () => boolean) {
  const entries = consumers.get(subscribe) ?? new Map();
  consumers.set(subscribe, entries); entries.set(active, topic);
  return () => { entries.delete(active); if (!entries.size) consumers.delete(subscribe); };
}
export function hasReadConsumer(subscribe: Subscribe, topic: Topic): boolean {
  return [...(consumers.get(subscribe) ?? [])].some(([active, key]) => key === topic && active());
}
