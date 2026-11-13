// Source: docklet-next/issues/343
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original owned service-set semantics and container policy wire intent. M21 #438: candidate scans and verified manifest binding. M24 recovery evidence, validation and reason contracts.

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
	return r.AutoRestore == other.AutoRestore && r.PointID == other.PointID && r.ExpectedRevision == other.ExpectedRevision && r.ReplaceConfiguration == other.ReplaceConfiguration && r.Service == other.Service && r.ExpectedManifest == other.ExpectedManifest && slices.Equal(r.Services, other.Services)
}

// ContainerImageRequest carries optional hub policy attribution. The resolved
// RuntimeTarget freezes the canonical container ID before any executor work.
type ContainerImageRequest struct {
	AutoRestore      bool           `json:"autoRestore,omitempty"`
	Policy           *PolicyBinding `json:"policy,omitempty"`
	ExpectedManifest string         `json:"expectedManifest,omitempty" pattern:"^sha256:[a-f0-9]{64}$"`
}

// VerifiedImageRequest is the public update body; policy attribution is trusted runtime input.
type VerifiedImageRequest struct {
	AutoRestore      bool   `json:"autoRestore,omitempty"`
	ExpectedManifest string `json:"expectedManifest,omitempty" pattern:"^sha256:[a-f0-9]{64}$"`
}
