// Source: docklet-next/issues/640
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original guided tour session state.

import { createStore } from "@/shared/lib/store";

import { TOUR_STEPS } from "./tour-steps";

/** Session state only: the offer follows a finished setup, a restart comes from Settings › Explanations. */
export interface TourState {
  status: "idle" | "offer" | "running";
  index: number;
}

export const tourStore = createStore<TourState>({ status: "idle", index: 0 });

export const offerTour = () => tourStore.set({ status: "offer", index: 0 });
export const startTour = () => tourStore.set({ status: "running", index: 0 });
export const endTour = () => tourStore.set({ status: "idle", index: 0 });

export function goToStep(index: number): void {
  if (index < 0 || index >= TOUR_STEPS.length) return;
  tourStore.set({ status: "running", index });
}
