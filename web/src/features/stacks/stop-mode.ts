// Source: docklet-next/issues/317
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original stop-mode resolution that fails closed when the hub setting is unreadable.

import { useConnectedSettings } from "@/data/queries";

export type ExplicitStop = "down" | "stop-containers";

/**
 * Stop follows the hub setting. When it cannot be read (for example without
 * settings:read), the mode-dependent Stop is disabled and both explicit
 * actions are offered instead, so no action guesses the mode.
 */
export function useStackStopMode(): { unavailable: boolean; alternates: ExplicitStop[] } {
  const settings = useConnectedSettings();
  if (settings.isError) return { unavailable: true, alternates: ["down", "stop-containers"] };
  const mode = settings.data?.general.stackStopMode ?? "down";
  return { unavailable: false, alternates: [mode === "down" ? "stop-containers" : "down"] };
}
