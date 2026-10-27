// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

import { CancelledError } from "@tanstack/react-query";
import { AdapterOutcomeError } from "../capability";

export const activityTimeout = () => new AdapterOutcomeError({ kind: "error", code: "activity.read-timeout" });

/** Bound headers, body consumption and safe retries with the same owned signal. */
export async function activityDeadline<T>(read: (signal: AbortSignal) => Promise<T>, parent?: AbortSignal, milliseconds = 40_000): Promise<T> {
  const controller = new AbortController();
  let reject!: (error: unknown) => void;
  const stopped = new Promise<never>((_, fail) => { reject = fail; });
  const abort = () => { const error = parent?.reason instanceof AdapterOutcomeError ? parent.reason : new CancelledError(); controller.abort(error); reject(error); };
  const timer = setTimeout(() => { const error = activityTimeout(); controller.abort(error); reject(error); }, milliseconds);
  parent?.addEventListener("abort", abort, { once: true });
  if (parent?.aborted) abort();
  try { return await Promise.race([read(controller.signal), stopped]); }
  finally { clearTimeout(timer); parent?.removeEventListener("abort", abort); }
}
