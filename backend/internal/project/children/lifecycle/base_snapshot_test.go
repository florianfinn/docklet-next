// Source: docklet-next/backend/internal/project/children/lifecycle/{store,snapshot}.go
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Base Snapshot and MarshalJSON renamed solely to create real old sealed payloads in regression tests; M34 #548.

package lifecycle

import (
	"encoding/json"
	compose "github.com/compose-spec/compose-go/v2/types"
)

type baseSnapshot struct {
	AutoExcluded                                                   bool `json:"-"`
	TargetID, TargetIdentity, RegistrationID, RunID, SavedRevision string
	Files                                                          []File
	Model                                                          *compose.Project
	Services                                                       map[string]ServiceImage
	Containers                                                     map[string]ServiceImage
}
type baseSnapshotAlias baseSnapshot
type baseModelRuntime struct {
	WorkingDir        string
	ComposeFiles      []string
	Environment       compose.Mapping
	DisabledServices  compose.Services
	DisabledJobs      compose.Jobs
	Profiles          []string
	Extensions        compose.Extensions
	ServiceExtensions map[string]compose.Extensions
}
type baseSnapshotWire struct {
	baseSnapshotAlias
	ModelRuntime *baseModelRuntime
}

func (s baseSnapshot) MarshalJSON() ([]byte, error) {
	wire := baseSnapshotWire{baseSnapshotAlias: baseSnapshotAlias(s)}
	if s.Model != nil {
		m := s.Model
		runtime := &baseModelRuntime{WorkingDir: m.WorkingDir, ComposeFiles: m.ComposeFiles, Environment: m.Environment, DisabledServices: m.DisabledServices, DisabledJobs: m.DisabledJobs, Profiles: m.Profiles, Extensions: m.Extensions, ServiceExtensions: map[string]compose.Extensions{}}
		for key, svc := range m.Services {
			runtime.ServiceExtensions[key] = svc.Extensions
		}
		wire.ModelRuntime = runtime
	}
	return json.Marshal(wire)
}
