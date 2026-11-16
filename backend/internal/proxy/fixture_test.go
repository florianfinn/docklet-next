// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original protocol regression and deterministic recovery proofs.

package proxy

import (
	"context"
	"encoding/json/jsontext"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/modules"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const initialEpoch = "2026-11-15T01:00:00Z"
const nextEpoch = "2026-11-15T02:00:00Z"

type fakeRuntime struct {
	mu                         sync.Mutex
	files                      map[string]Bytes
	epoch                      string
	running                    bool
	checkErr                   error
	rebindErr                  error
	rebound                    bool
	epochErr                   error
	beforePublish              func()
	publications, restorations int
}

func (f *fakeRuntime) Check(context.Context, dto.Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checkErr
}
func (f *fakeRuntime) Rebind(_ context.Context, i dto.Instance) (dto.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.checkErr == nil {
		return i, nil
	}
	if f.rebindErr != nil {
		return i, f.rebindErr
	}
	f.rebound = true
	return i, nil
}
func (f *fakeRuntime) Target(_ context.Context, _ dto.Instance, p string) (Bytes, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.files[p]; ok {
		return b, nil
	}
	return Snapshot(nil, false), nil
}
func (f *fakeRuntime) Graph(context.Context, dto.Instance) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := map[string]string{}
	for p, b := range f.files {
		if !strings.Contains(p, ".docklet-") && !strings.Contains(p, "docklet-check-") && p != "" {
			if b.Present {
				g[p] = b.Hash
			}
		}
	}
	return g, nil
}
func (f *fakeRuntime) Stage(_ context.Context, i Intent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i.New.Present {
		f.files[i.TempPath] = i.New
	}
	return nil
}
func (f *fakeRuntime) Publish(_ context.Context, i Intent) error {
	if f.beforePublish != nil {
		f.beforePublish()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.publications++
	if i.New.Present {
		f.files[i.Request.Target] = f.files[i.TempPath]
		delete(f.files, i.TempPath)
	} else {
		f.files[i.TempPath] = f.files[i.Request.Target]
		delete(f.files, i.Request.Target)
	}
	return nil
}
func (f *fakeRuntime) Restore(_ context.Context, i Intent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restorations++
	if i.Old.Present {
		f.files[i.Request.Target] = i.Old
	} else {
		delete(f.files, i.Request.Target)
	}
	return nil
}
func (f *fakeRuntime) Cleanup(_ context.Context, i Intent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, i.TempPath)
	delete(f.files, i.MarkerPath)
	return nil
}
func (f *fakeRuntime) Epoch(context.Context, dto.Instance) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Like DockerRuntime.Epoch, an inspect failure reports a stopped container.
	if f.epochErr != nil {
		return false, "", f.epochErr
	}
	return f.running, f.epoch, nil
}

type fakeEngine struct {
	runtime                                           *fakeRuntime
	new                                               Bytes
	reload, restart, marker, validation, observations int
	adminOff                                          bool
	observe                                           func() Observation
	reloadErr                                         error
	restartErr                                        error
	validate                                          func() error
	prepareErr                                        error
	state                                             jsontext.Value
	seen                                              []Intent
}

func (e *fakeEngine) Inventory(context.Context, dto.Instance) (dto.Inventory, error) {
	return dto.Inventory{Eligibility: dto.Eligibility{Writable: true}}, nil
}
func (e *fakeEngine) Read(context.Context, dto.Instance) (dto.ReadResult, error) {
	return dto.ReadResult{}, nil
}
func (e *fakeEngine) Prepare(ctx context.Context, q dto.ApplyRequest) (Intent, error) {
	if e.prepareErr != nil {
		return Intent{}, e.prepareErr
	}
	b, _ := e.runtime.Target(ctx, q.Instance, q.Target)
	g, _ := e.runtime.Graph(ctx, q.Instance)
	return Intent{Old: b, New: e.new, GraphHashes: g, Mode: 0644, Reload: q.Instance.Kind == dto.Caddy && !e.adminOff, Restart: e.adminOff, Owned: true, Engine: e.state}, nil
}
func (e *fakeEngine) Validate(context.Context, Intent) error {
	e.validation++
	if e.validate != nil {
		return e.validate()
	}
	return nil
}
func (e *fakeEngine) Reload(context.Context, Intent) error { e.reload++; return e.reloadErr }
func (e *fakeEngine) Restart(context.Context, Intent) error {
	e.restart++
	if e.restartErr != nil {
		return e.restartErr
	}
	e.runtime.mu.Lock()
	e.runtime.running = true
	e.runtime.epoch = nextEpoch
	e.runtime.mu.Unlock()
	return nil
}
func (e *fakeEngine) Observe(_ context.Context, i Intent, _ bool) (Observation, error) {
	e.observations++
	e.seen = append(e.seen, i)
	if e.observe != nil {
		return e.observe(), nil
	}
	return Observation{State: dto.Verified, Evidence: dto.Routing}, nil
}
func (e *fakeEngine) Mark(_ context.Context, i Intent) error {
	e.marker++
	e.runtime.mu.Lock()
	e.runtime.files[i.MarkerPath] = Snapshot([]byte("marker"), true)
	e.runtime.mu.Unlock()
	return nil
}
func (e *fakeEngine) Cleanup(context.Context, Intent) error { return nil }

type fixture struct {
	db       *database.DB
	a        *Agent
	f        *fakeRuntime
	e        *fakeEngine
	request  dto.ApplyRequest
	mods     *modules.Service
	settings *settings.SettingsService
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "proxy.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	f := &fakeRuntime{files: map[string]Bytes{"route.yml": Snapshot([]byte("old"), true)}, epoch: initialEpoch, running: true}
	e := &fakeEngine{runtime: f, new: Snapshot([]byte("new"), true)}
	a, err := NewAgent(db.DB, f, Engines{dto.Traefik: e, dto.Caddy: e})
	require.NoError(t, err)
	s, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Stop(context.WithoutCancel(t.Context()))) })
	require.NoError(t, s.EnsureDefaultSettings(t.Context()))
	require.NoError(t, s.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
	mods := modules.New(s)
	g, _ := f.Graph(t.Context(), dto.Instance{})
	q := dto.ApplyRequest{ID: "apply-one", Instance: dto.Instance{ID: "instance-one", EnvironmentID: "example-host", Kind: dto.Traefik, Identity: dto.Identity{ContainerID: "example-container", ContainerName: "example-proxy", StartedAt: initialEpoch}, Source: dto.Source{Kind: dto.ProjectSource, Canonical: "/example/projects/proxy", MountTarget: "/etc/proxy"}, Eligibility: dto.Eligibility{Writable: true}, Registration: "example-registration", Generation: a.Generation()}, Generation: a.Generation(), Registration: "example-registration", Target: "route.yml", Expected: f.files["route.yml"].FileState, GraphHashes: g, Change: dto.Change{Kind: "edit-route"}}
	return &fixture{db, a, f, e, q, mods, s}
}
func (x *fixture) query() dto.Query {
	return dto.Query{ID: x.request.ID, Instance: x.request.Instance, Generation: x.a.Generation(), Registration: x.request.Registration}
}
func (x *fixture) persist(t *testing.T, i Intent, o dto.Operation) {
	t.Helper()
	ib, err := marshal(i)
	require.NoError(t, err)
	ob, err := marshal(o)
	require.NoError(t, err)
	require.NoError(t, x.db.Create(&Journal{ID: o.ID, Source: "host\x00" + x.request.Instance.Source.Canonical, Generation: o.Generation, Digest: o.Digest, Registration: o.Registration, State: o.State, Phase: o.Phase, Intent: ib, Result: ob, CreatedAt: o.CreatedAt}).Error)
}
func cloneRequest(q dto.ApplyRequest) dto.ApplyRequest {
	q.GraphHashes = maps.Clone(q.GraphHashes)
	return q
}
