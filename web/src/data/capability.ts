// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original capability outcome model for issue 38.

import type { components } from "@/api/generated/schema";

export type ResourceRef = components["schemas"]["CapabilityResourceRef"];
export type CapabilityDescriptor = components["schemas"]["CapabilityDescriptor"];
export type AdapterOutcome<T> =
  | { kind: "ok"; value: T }
  | { kind: "not-available"; operationId: string; reasonCode: string; resourceRef: ResourceRef }
  | { kind: "conflict"; code: "run.busy" | "run.cancelling"; activeRunId: string }
  | { kind: "conflict"; code: "run.request-mismatch"; activeRunId?: never }
  | { kind: "error"; code: string; status?: number };

export class AdapterOutcomeError extends Error {
  readonly outcome: Exclude<AdapterOutcome<never>, { kind: "ok" }>;
  constructor(outcome: Exclude<AdapterOutcome<never>, { kind: "ok" }>) {
    super(outcome.kind);
    this.name = "AdapterOutcomeError";
    this.outcome = outcome;
  }
}

export const HUB_REF: ResourceRef = { kind: "hub", id: "0" };
export const GENERIC_REASON = "capability.unavailable";

export function unavailable(operationId: string, resourceRef: ResourceRef = HUB_REF, reasonCode = GENERIC_REASON): AdapterOutcomeError {
  return new AdapterOutcomeError({ kind: "not-available", operationId, reasonCode, resourceRef });
}
