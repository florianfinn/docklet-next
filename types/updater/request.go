// Source: docklet-next/issues/343
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original owned service-set semantics and container policy wire intent.

package updater

import "slices"

func (r LifecycleRequest) Clone() LifecycleRequest {
	r.Services = slices.Clone(r.Services)
	slices.Sort(r.Services)
	r.Services = slices.Compact(r.Services)
	return r
}

func (r LifecycleRequest) Equal(other LifecycleRequest) bool {
	r, other = r.Clone(), other.Clone()
	return r.PointID == other.PointID && r.ExpectedRevision == other.ExpectedRevision && r.ReplaceConfiguration == other.ReplaceConfiguration && slices.Equal(r.Services, other.Services)
}

// ContainerImageRequest carries optional hub policy attribution. The resolved
// RuntimeTarget freezes the canonical container ID before any executor work.
type ContainerImageRequest struct {
	Policy *PolicyBinding `json:"policy,omitempty"`
}
