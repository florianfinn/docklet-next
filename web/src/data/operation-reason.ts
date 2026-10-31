// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shared presentation keys for M14 operation refusals.

export function operationReasonKey(code: string) {
  switch (code) {
    case "container.origin-locked": return "originLocked";
    case "container.recreate-recovered": return "recreateRecovered";
    case "container.recreate-ambiguous": return "recreateAmbiguous";
    case "runtime.idempotency-mismatch": return "idempotencyMismatch";
    case "agent.unavailable": return "hostUnavailable";
    case "agent.invalid-token": return "credentialRejected";
    case "agent.update-required": return "updateRequired";
    default: return undefined;
  }
}
