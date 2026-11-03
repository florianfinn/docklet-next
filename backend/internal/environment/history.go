// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original history invalidation seam at host lifecycle boundaries.

package environment

func (s *EnvironmentService) SetHistoryInvalidator(invalidate func(string)) {
	s.historyInvalidator = invalidate
}
func (s *EnvironmentService) invalidateHistory(id string) {
	if s.historyInvalidator != nil {
		s.historyInvalidator(id)
	}
}

func hostRevisionChangedForHistory(before, after Environment) bool {
	return before.Enabled != after.Enabled || before.ApiUrl != after.ApiUrl || before.CredentialRevision != after.CredentialRevision || before.IsEdge != after.IsEdge
}
