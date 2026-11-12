// Source: docklet-next/issues/435
// Revision: 5b0fee810af34673e95c76ffb8af1d49b08c56a7
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original active counts command cancellation on the receiving agent.

package edge

// Active counts are a synchronous read and share the bounded cancellable ingress.
func activeSummaryRequest(method, path string) bool {
	command, ok := ResolveEdgeCommandName(method, path, false).Get()
	return ok && command == "vulnerability.active_summaries"
}
