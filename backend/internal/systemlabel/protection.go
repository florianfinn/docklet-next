// Source: docklet-next/issues/369
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original shared workload protection predicate.

package systemlabel

// SelfProtected wraps the adopted Arcane current-process verdict. Remote callers
// retain the executing host's verdict; Docklet roles protect workloads on all hosts.
func SelfProtected(values map[string]string, arcaneSelf bool) bool {
	return values["io.docklet.system"] == "agent" || values["io.docklet.system"] == "hub" || arcaneSelf
}
