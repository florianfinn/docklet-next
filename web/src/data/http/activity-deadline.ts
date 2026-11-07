// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223). M20 #360: shell list reads and target-specific terminal invalidation.

import { AdapterOutcomeError } from "../capability";
import { hostReadDeadline } from "./host-read-deadline";

export const activityTimeout = () => new AdapterOutcomeError({ kind: "error", code: "activity.read-timeout" });

export function activityDeadline<T>(read: (signal: AbortSignal) => Promise<T>, parent?: AbortSignal, milliseconds = 40_000): Promise<T> {
  return hostReadDeadline(milliseconds, "activity.read-timeout")(read, parent);
}
