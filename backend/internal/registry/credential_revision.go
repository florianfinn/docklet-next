// Source: docklet-next/issues/445
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original registry credential revision for candidate observations.

package registry

// CredentialRevision invalidates raw observations after local or delivered config changes.
func (s *ContainerRegistryService) CredentialRevision() uint64 { return s.credentialRevision.Load() }

func (s *ContainerRegistryService) advanceCredentialRevision(err *error) {
	if *err == nil {
		s.credentialRevision.Add(1)
	}
}
