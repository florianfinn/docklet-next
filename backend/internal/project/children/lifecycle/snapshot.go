// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Preserve Compose runtime fields omitted by its public JSON representation.

package lifecycle

import (
	"encoding/json"
	compose "github.com/compose-spec/compose-go/v2/types"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

type snapshotAlias Snapshot
type modelRuntime struct {
	WorkingDir        string
	ComposeFiles      []string
	Environment       compose.Mapping
	DisabledServices  compose.Services
	DisabledJobs      compose.Jobs
	Profiles          []string
	Extensions        compose.Extensions
	ServiceExtensions map[string]compose.Extensions
}
type snapshotWire struct {
	snapshotAlias
	ModelRuntime *modelRuntime
}

func (s Snapshot) MarshalJSON() ([]byte, error) {
	wire := snapshotWire{snapshotAlias: snapshotAlias(s)}
	if s.Model != nil {
		m := s.Model
		runtime := &modelRuntime{WorkingDir: m.WorkingDir, ComposeFiles: m.ComposeFiles, Environment: m.Environment, DisabledServices: m.DisabledServices, DisabledJobs: m.DisabledJobs, Profiles: m.Profiles, Extensions: m.Extensions, ServiceExtensions: map[string]compose.Extensions{}}
		for key, svc := range m.Services {
			runtime.ServiceExtensions[key] = svc.Extensions
		}
		wire.ModelRuntime = runtime
	}
	return json.Marshal(wire)
}
func (s *Snapshot) UnmarshalJSON(body []byte) error {
	var wire snapshotWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	*s = Snapshot(wire.snapshotAlias)
	if s.Model == nil {
		return nil
	}
	m := s.Model
	if r := wire.ModelRuntime; r != nil {
		m.WorkingDir = r.WorkingDir
		m.ComposeFiles = r.ComposeFiles
		m.Environment = r.Environment
		m.DisabledServices = r.DisabledServices
		m.DisabledJobs = r.DisabledJobs
		m.Profiles = r.Profiles
		m.Extensions = r.Extensions
		for key, svc := range m.Services {
			svc.Extensions = r.ServiceExtensions[key]
			m.Services[key] = svc
		}
	}
	for key, svc := range m.Services {
		svc.Name = key
		m.Services[key] = svc
	}
	for key, svc := range m.DisabledServices {
		svc.Name = key
		m.DisabledServices[key] = svc
	}
	return nil
}

// A point is usable only when its complete all-service restore fits admission.
func fitsRestoreIntent(snapshot *Snapshot) bool {
	if snapshot == nil {
		return false
	}
	placeholder := Hash(nil)
	intent := dto.LifecycleIntent{Request: dto.LifecycleRequest{PointID: "00000000-0000-4000-8000-000000000000", ExpectedRevision: snapshot.SavedRevision, ReplaceConfiguration: true}, TargetIdentity: snapshot.TargetIdentity, SavedRevision: snapshot.SavedRevision, ObservedRevision: placeholder, PointRevision: placeholder, Selections: []dto.ImageSelection{}}
	for name, img := range snapshot.Services {
		if !img.Present {
			continue
		}
		intent.Selections = append(intent.Selections, dto.ImageSelection{Service: name, SourceReference: img.Reference, Reference: img.Reference, Digest: img.Digest, ImageID: img.ImageID, Platform: img.Platform, Kind: "restore"})
	}
	if len(intent.Selections) > dto.MaxLifecycleServices {
		return false
	}
	body, err := json.Marshal(intent)
	return err == nil && len(body) <= dto.MaxLifecycleIntentBytes
}
