import { createStore } from "@/shared/lib/store";

/** Entrance slots that already played in this page load; remounts and route changes never replay them. */
export const playedSlots = new Set<string>();
/** Incremented by replayBrandEntrance; every mounted animated logo plays again once. */
export const replayStore = createStore(0);

export function replayBrandEntrance(): void {
  replayStore.set((n) => n + 1);
}

/** Test hook: forget which slots played. */
export function resetBrandEntrances(): void {
  playedSlots.clear();
}
