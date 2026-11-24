// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original proxy module API and lifecycle wiring.

package proxy

import (
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerinternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/modules"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"log/slog"
	"sync"
	"time"
)

type Module struct {
	Agent       *Agent
	Hub         *Hub
	Runtime     *DockerRuntime
	Settings    *settings.SettingsService
	Modules     *modules.Service
	Transport   *HostTransport
	AgentMode   bool
	unsubscribe func()
	life        context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	passes      sync.Map
	// lifeMu orders Stop against every new unit of reconciler work, so a callback
	// already captured by the online notifier cannot start a pass after Stop.
	lifeMu  sync.Mutex
	stopped bool
	// every overrides reconcileInterval and passHook observes pass start and end in tests.
	every    time.Duration
	passHook func(key string, finished bool)
}

// reconcileInterval bounds how long an unfinished record waits without an agent-online event.
const reconcileInterval = 30 * time.Second

func NewModule(db *database.DB, cfg *config.Config, mods *modules.Service, s *settings.SettingsService, d *dockerinternal.DockerClientService, v *volume.VolumeService, hosts *environment.EnvironmentService) (*Module, error) {
	engines := Engines{}
	runtime := &DockerRuntime{Docker: d, Settings: s, Volumes: v, DB: db, Engines: engines}
	a, err := NewAgent(db.DB, runtime, engines)
	if err != nil {
		return nil, err
	}
	a.Gate = mods
	mods.SetAgentMode(cfg.AgentMode || cfg.EdgeAgent)
	m := &Module{Agent: a, Runtime: runtime, Settings: s, Modules: mods, AgentMode: cfg.AgentMode || cfg.EdgeAgent}
	m.Transport = &HostTransport{Hosts: hosts, Local: m}
	m.Hub = &Hub{DB: db.DB, Modules: mods, Transport: m.Transport}
	err = mods.Register(features.Proxy, modules.Hooks{Unfinished: func(ctx context.Context) (string, error) {
		id, err := unfinished(ctx, db.DB, "proxy_journal")
		if err != nil || id != "" {
			return id, err
		}
		return m.Hub.Unfinished(ctx)
	}, DeleteData: func(ctx context.Context) error {
		g, err := clearData(ctx, db.DB)
		if err == nil {
			a.generationID.Store(g)
		}
		return err
	}})
	return m, err
}

// RegisterEngine is startup-only, before Start or any request. S3/S4 construct
// engines with Runtime and register their kind; runtime and agent share this map.
func (m *Module) RegisterEngine(kind dto.Kind, e Engine) { m.Agent.Engines[kind] = e }
func (m *Module) Start(ctx context.Context) error {
	// Leftover probe containers are best-effort cleanup; a hub without a
	// reachable Docker daemon must still start.
	if m.Runtime != nil && m.Runtime.Docker != nil {
		if err := m.Runtime.SweepProbes(ctx); err != nil {
			slog.WarnContext(ctx, "Proxy probe cleanup deferred", "error", err)
		}
	}
	if err := m.Agent.Settle(ctx); err != nil {
		return err
	}
	if !m.AgentMode {
		var subscribe func(func(context.Context, string)) func()
		if m.Transport != nil && m.Transport.Hosts != nil {
			subscribe = m.Transport.Hosts.SubscribeAgentOnline
		}
		interval := m.every
		if interval == 0 {
			interval = reconcileInterval
		}
		m.startReconciler(subscribe, interval)
	}
	return nil
}

// startReconciler follows unfinished hub records at startup, on agent-online and
// every interval. None of these triggers waits for a pass; Stop cancels and waits.
func (m *Module) startReconciler(subscribe func(func(context.Context, string)) func(), interval time.Duration) {
	m.life, m.cancel = context.WithCancel(context.Background())
	if subscribe != nil {
		m.unsubscribe = subscribe(func(_ context.Context, id string) { m.reconcileAsync(id) })
	}
	m.reconcileAsync("")
	if !m.admit() {
		return
	}
	go func() {
		defer m.wg.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-m.life.Done():
				return
			case <-t.C:
				m.reconcileAsync("")
			}
		}
	}()
}

// reconcileAsync starts a pass unless one with the same scope is running. Each
// environment is followed by at most one pass at a time, including the all-hosts pass.
func (m *Module) reconcileAsync(environmentID string) {
	life := m.life
	if life == nil {
		life = context.Background()
	}
	key := reconcileKey(environmentID)
	if _, running := m.passes.LoadOrStore(key, true); running {
		return
	}
	if !m.admit() {
		m.passes.Delete(key)
		return
	}
	go func() {
		defer m.wg.Done()
		defer m.passes.Delete(key)
		if m.passHook != nil {
			m.passHook(key, false)
			defer m.passHook(key, true)
		}
		m.reconcile(life, environmentID)
	}()
}

// admit counts one unit of reconciler work unless Stop has begun.
func (m *Module) admit() bool {
	m.lifeMu.Lock()
	defer m.lifeMu.Unlock()
	if m.stopped {
		return false
	}
	m.wg.Add(1)
	return true
}
func reconcileKey(environmentID string) string {
	if environmentID == "" {
		return "all"
	}
	return "environment:" + environmentID
}
func (m *Module) Inventory(ctx context.Context, i dto.Instance) (dto.Inventory, error) {
	if err := m.Agent.Check(ctx, i); err != nil {
		return dto.Inventory{}, err
	}
	e, err := m.Agent.Engines.For(i.Kind)
	if err != nil {
		return dto.Inventory{}, err
	}
	r, err := e.Inventory(ctx, i)
	if err != nil {
		return r, err
	}
	// The current epoch of the same container: an operation fences it, not the registration's.
	if _, r.Identity.StartedAt, err = m.Agent.Runtime.Epoch(ctx, i); err != nil {
		return dto.Inventory{}, err
	}
	r.Identity.ContainerID, r.Identity.ContainerName = i.Identity.ContainerID, i.Identity.ContainerName
	r.Generation = m.Agent.Generation()
	if i.Source.ReadOnly || i.Source.Kind == dto.SingleFileSource || i.Source.Kind == dto.UnknownSourceKind {
		r.Eligibility = dto.Eligibility{Reason: i.Source.Reason}
	}
	return r, err
}
func (m *Module) Read(ctx context.Context, i dto.Instance) (dto.ReadResult, error) {
	if err := m.Agent.Check(ctx, i); err != nil {
		return dto.ReadResult{}, err
	}
	e, err := m.Agent.Engines.For(i.Kind)
	if err != nil {
		return dto.ReadResult{}, err
	}
	r, err := e.Read(ctx, i)
	if _, ok := errors.AsType[*IncompleteRead](err); ok {
		err = nil // shown as incomplete inventory
	}
	for n := range r.Files {
		b, owned, er := m.Runtime.LastVerified(ctx, i, r.Files[n].Path)
		if er != nil {
			return r, er
		}
		r.Files[n].Owned = owned
		if b.Present {
			r.Files[n].LastVerifiedHash = b.Hash
		}
	}
	r.Generation = m.Agent.Generation()
	r.Inventory.Generation = m.Agent.Generation()
	if i.Source.ReadOnly || i.Source.Kind == dto.SingleFileSource || i.Source.Kind == dto.UnknownSourceKind {
		r.Eligibility = dto.Eligibility{Reason: i.Source.Reason}
	}
	return r, err
}
func (m *Module) Instance(ctx context.Context, id string) (dto.Instance, error) {
	var r instanceRow
	err := m.Hub.DB.WithContext(ctx).First(&r, "id = ?", id).Error
	var i dto.Instance
	if err == nil {
		err = json.Unmarshal(r.Data, &i)
	}
	return i, err
}
func (m *Module) Instances(ctx context.Context) ([]dto.Instance, error) {
	var rows []instanceRow
	if err := m.Hub.DB.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []dto.Instance{}
	for _, r := range rows {
		var i dto.Instance
		if err := json.Unmarshal(r.Data, &i); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}
func (m *Module) CreateInstance(ctx context.Context, q dto.CreateInstanceRequest) (dto.Instance, error) {
	i := dto.Instance{EnvironmentID: q.EnvironmentID, Kind: q.Kind, Identity: dto.Identity{ContainerID: q.ContainerID}, Source: dto.Source{MountTarget: q.MountTarget}, Name: q.Name, RootFile: q.RootFile, RouteDirectory: q.RouteDirectory}
	existing, err := m.Instances(ctx)
	if err != nil {
		return i, err
	}
	for _, registered := range existing {
		if registered.EnvironmentID == i.EnvironmentID && registered.Identity.ContainerID == i.Identity.ContainerID {
			return i, dto.Fail(dto.OperationConflict, 409)
		}
	}
	list, err := m.Transport.Discover(ctx, i.EnvironmentID)
	if err != nil {
		return i, err
	}
	found := false
	for _, d := range list {
		if d.Identity.ContainerID != i.Identity.ContainerID || d.Kind != i.Kind {
			continue
		}
		for _, s := range d.Sources {
			if s.MountTarget == i.Source.MountTarget {
				i.Source = s
				i.Identity = d.Identity
				found = true
			}
		}
	}
	if !found {
		return i, dto.Fail(dto.ContainerChanged, 409)
	}
	if err := revalidateSource(i.Source, i.RouteDirectory); err != nil {
		return i, err
	}
	i.ID = uuid.NewString()
	i.Registration, err = m.Transport.Registration(ctx, i.EnvironmentID)
	if err != nil {
		return i, err
	}
	if i.Kind == dto.Caddy {
		i.Authority = dto.CaddyfileAuthority
	} else {
		i.Authority = dto.FileAuthority
	}
	inventory, err := m.Transport.Inventory(ctx, i)
	if err != nil {
		return i, err
	}
	i.Generation = inventory.Generation
	i.Eligibility = inventory.Eligibility
	m.Hub.mu.Lock()
	defer m.Hub.mu.Unlock()
	err = m.Modules.Admit(ctx, features.Proxy, func(ctx context.Context) error {
		return m.Hub.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			owner, er := instanceOwner(tx, i)
			if er != nil {
				return er
			}
			if owner != "" {
				return dto.Fail(dto.OperationConflict, 409)
			}
			data, er := marshal(i)
			if er != nil {
				return er
			}
			return tx.Create(&instanceRow{ID: i.ID, EnvironmentID: i.EnvironmentID, Data: data}).Error
		})
	})
	return i, err
}
func (m *Module) saveInstance(ctx context.Context, i dto.Instance) error {
	b, err := marshal(i)
	if err != nil {
		return err
	}
	return m.Hub.DB.WithContext(ctx).Save(&instanceRow{ID: i.ID, EnvironmentID: i.EnvironmentID, Data: b}).Error
}
func (m *Module) UpdateInstance(ctx context.Context, id string, s dto.Settings) (dto.Instance, error) {
	i, original, err := m.instanceSnapshot(ctx, id)
	if err != nil {
		return i, err
	}
	i.Name = s.Name
	i.RootFile = s.RootFile
	i.RouteDirectory = s.RouteDirectory
	var held reservation
	err = m.Hub.DB.WithContext(ctx).First(&held, "instance_id = ?", id).Error
	if err == nil {
		return i, dto.Fail(dto.ModuleBusy, 409)
	}
	if !missing(err) {
		return i, err
	}
	if err := m.revalidateSettings(ctx, i); err != nil {
		if reason(err, "") != dto.ContainerChanged {
			return i, err
		}
		adopted, er := m.Adopt(ctx, id)
		if er != nil {
			return i, er
		}
		if adopted.Outcome == dto.AdoptionRefused {
			return i, dto.Fail(dto.ContainerChanged, 409)
		}
		i, original, err = m.instanceSnapshot(ctx, id)
		if err != nil {
			return i, err
		}
		i.Name, i.RootFile, i.RouteDirectory = s.Name, s.RootFile, s.RouteDirectory
		if er = m.revalidateSettings(ctx, i); er != nil {
			return i, er
		}
	}
	inv, err := m.Transport.Inventory(ctx, i)
	if err != nil {
		return i, err
	}
	i.Eligibility = inv.Eligibility
	i.Generation = inv.Generation
	i.Registration, err = m.Transport.Registration(ctx, i.EnvironmentID)
	if err != nil {
		return i, err
	}
	m.Hub.mu.Lock()
	defer m.Hub.mu.Unlock()
	return i, m.Modules.Admit(ctx, features.Proxy, func(ctx context.Context) error {
		return m.Hub.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			open, er := instanceOpen(tx, id)
			if er != nil {
				return er
			}
			if open {
				return dto.Fail(dto.ModuleBusy, 409)
			}
			return casInstance(tx, original, i)
		})
	})
}
func (m *Module) RemoveInstance(ctx context.Context, id string) error {
	m.Hub.mu.Lock()
	defer m.Hub.mu.Unlock()
	return m.Modules.Admit(ctx, features.Proxy, func(ctx context.Context) error {
		return m.Hub.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			open, er := instanceOpen(tx, id)
			if er != nil {
				return er
			}
			if open {
				return dto.Fail(dto.ModuleBusy, 409)
			}
			return tx.Delete(&instanceRow{}, "id = ?", id).Error
		})
	})
}
func (m *Module) Operations(ctx context.Context, id string) ([]dto.Operation, error) {
	if err := prune(ctx, m.Hub.DB, nowUTC()); err != nil {
		return nil, err
	}
	var rows []Record
	if err := m.Hub.DB.WithContext(ctx).Where("instance_id = ?", id).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []dto.Operation{}
	for _, r := range rows {
		_, o, err := decodeRecord(r)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// Settlement follows admitted work while the module is off as well. Reconnects
// query existing IDs only; they never dispatch a new Apply.
func (m *Module) reconcile(ctx context.Context, environmentID string) {
	var rows []Record
	if err := m.Hub.DB.WithContext(ctx).Where("finished_at IS NULL").Find(&rows).Error; err != nil {
		return
	}
	byEnvironment := map[string][]string{}
	var order []string
	for _, row := range rows {
		request, _, err := decodeRecord(row)
		if err != nil {
			slog.WarnContext(ctx, "Proxy operation record cannot be decoded", "operationId", row.ID)
			continue
		}
		e := request.Instance.EnvironmentID
		if environmentID != "" && e != environmentID {
			continue
		}
		if _, seen := byEnvironment[e]; !seen {
			order = append(order, e)
		}
		byEnvironment[e] = append(byEnvironment[e], row.ID)
	}
	for _, e := range order {
		// The all-hosts pass skips an environment that its own pass is following.
		if environmentID == "" {
			if _, running := m.passes.LoadOrStore(reconcileKey(e), true); running {
				continue
			}
		}
		for _, id := range byEnvironment[e] {
			if ctx.Err() != nil {
				break
			}
			if _, err := m.Hub.Query(ctx, id); err != nil {
				slog.WarnContext(ctx, "Proxy operation settlement postponed", "operationId", id, "code", dto.AgentUnavailable)
			}
		}
		if environmentID == "" {
			m.passes.Delete(reconcileKey(e))
		}
		if ctx.Err() != nil {
			return
		}
	}
}
func (m *Module) Stop(context.Context) error {
	m.lifeMu.Lock()
	m.stopped = true
	m.lifeMu.Unlock()
	if m.unsubscribe != nil {
		m.unsubscribe()
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	return nil
}

// Discovery combines the host-owned projection with hub-owned registrations.
func (m *Module) Discover(ctx context.Context, environmentID string) ([]dto.Discovery, error) {
	rows, err := m.Transport.Discover(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	instances, err := m.Instances(ctx)
	if err != nil {
		return nil, err
	}
	for n := range rows {
		for _, i := range instances {
			if i.EnvironmentID == environmentID && i.Identity.ContainerID == rows[n].Identity.ContainerID {
				rows[n].InstanceID = i.ID
			}
		}
	}
	return rows, nil
}

// A new operation uses a fresh authenticated executor binding. Historical
// records keep their original binding, including after credential rotation.
func (m *Module) CreateOperation(ctx context.Context, id string, input dto.OperationRequest) (dto.Operation, error) {
	instance, err := m.writeAdmission(ctx, id, true)
	if err != nil {
		return dto.Operation{}, err
	}
	request := dto.ApplyRequest{ID: uuid.NewString(), Instance: instance, Generation: instance.Generation, Registration: instance.Registration, Target: input.Target, Expected: input.Expected, GraphHashes: input.GraphHashes, Change: input.Change, Acknowledgements: input.Acknowledgements}
	return m.Hub.Apply(ctx, request)
}

// writeAdmission refreshes the instance for a write. A recreated container is
// adopted first (AUTO-1), and the refreshed identity (a restart's StartedAt),
// generation and registration are stored before the hub's admission compares
// them. A concurrent change of the stored container identity is
// container-changed; any other concurrent change of the row is retried once,
// and a second one is an operation conflict.
func (m *Module) writeAdmission(ctx context.Context, id string, eligible bool) (dto.Instance, error) {
	for attempt := 0; ; attempt++ {
		instance, original, err := m.instanceSnapshot(ctx, id)
		if err != nil {
			return dto.Instance{}, err
		}
		inventory, err := m.Transport.Inventory(ctx, instance)
		if reason(err, "") == dto.ContainerChanged {
			adoption, er := m.Adopt(ctx, id)
			if er != nil {
				return dto.Instance{}, er
			}
			if adoption.Outcome == dto.AdoptionRefused {
				return dto.Instance{}, dto.Fail(dto.ContainerChanged, 409)
			}
			if instance, original, err = m.instanceSnapshot(ctx, id); err != nil {
				return dto.Instance{}, err
			}
			inventory, err = m.Transport.Inventory(ctx, instance)
		}
		if err != nil {
			return dto.Instance{}, err
		}
		if inventory.Identity.ContainerID != instance.Identity.ContainerID {
			return dto.Instance{}, dto.Fail(dto.ContainerChanged, 409)
		}
		registration, err := m.Transport.Registration(ctx, instance.EnvironmentID)
		if err != nil {
			return dto.Instance{}, err
		}
		instance.Generation = inventory.Generation
		instance.Registration = registration
		if inventory.Identity.StartedAt != "" {
			instance.Identity.StartedAt = inventory.Identity.StartedAt // a restart keeps container and source
		}
		instance.Eligibility = inventory.Eligibility
		if eligible {
			if err = Eligibility(instance); err != nil {
				return dto.Instance{}, err
			}
		}
		m.Hub.mu.Lock()
		err = m.Modules.Admit(ctx, features.Proxy, func(ctx context.Context) error {
			return m.Hub.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return casInstance(tx, original, instance) })
		})
		m.Hub.mu.Unlock()
		if reason(err, "") == dto.ContainerChanged {
			current, _, er := m.instanceSnapshot(ctx, id)
			if er != nil {
				return dto.Instance{}, er
			}
			if current.Identity.ContainerID != instance.Identity.ContainerID || current.Identity.ContainerName != instance.Identity.ContainerName {
				return dto.Instance{}, err
			}
			if attempt == 0 {
				continue
			}
			return dto.Instance{}, dto.Fail(dto.OperationConflict, 409)
		}
		return instance, err
	}
}
