// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import type { ActionRun } from "@/domain/model";

/** Consumer-only detail evidence; no additional activity or wire field is required. */
export interface RuntimePresentation {
  unknownReason?: string;
  resultCode?: string;
  released?: boolean;
  releaseReason?: string;
  lateResult?: string;
  canCancel?: boolean;
  canRelease?: boolean;
  currentIntent?: boolean;
  /** False for a targeted publication before any aggregate source observation. */
  sourceObserved?: boolean;
}
export type RuntimeView<T> = T & { runtime?: RuntimePresentation };
export const runtimePresentation = (run: ActionRun): RuntimePresentation | undefined => (run as RuntimeView<ActionRun>).runtime;

export function runtimeState(run: ActionRun): "unconfirmed" | "unknown" | "released" | "interrupted" | "cancelling" | undefined {
  const evidence = runtimePresentation(run);
  if (evidence?.released) return "released";
  if (run.outcomeState === "unknown") return evidence?.unknownReason ? "unknown" : "unconfirmed";
  if (evidence?.resultCode === "runtime.interrupted") return "interrupted";
  if (run.cancelRequested && (run.status === "queued" || run.status === "running")) return "cancelling";
  return undefined;
}
