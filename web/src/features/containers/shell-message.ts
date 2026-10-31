// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original bounded shell problem and exit catalogue mapping.

export function shellMessage(code: string) {
  switch (code) {
    case "container.shell-invalid": return "invalid";
    case "container.shell-protocol": return "protocol";
    case "container.shell-unavailable": return "unavailable";
    case "container.shell-orphaned": return "orphaned";
    case "container.shell-not-running": return "notRunning";
    case "agent.update-required": return "updateRequired";
    case "agent.invalid-token": return "credentialsRejected";
    case "agent.unavailable": return "hostUnavailable";
    case "authentication.required": return "signIn";
    case "capability.unavailable": case "capability.mapping-unavailable": return "permission";
    default: return "connectionEnded";
  }
}
