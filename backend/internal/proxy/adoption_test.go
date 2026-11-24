// Source: docklet-next/issues/592
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original adoption identity, transactional race, rollback and restart proofs.

package proxy

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type adoptionFixture struct {
	*fixture
	m         *Module
	candidate dto.Discovery
	discover  func()
	inventory func(dto.Instance)
}

func newAdoptionFixture(t *testing.T) *adoptionFixture {
	t.Helper()
	x := newFixture(t)
	f := &adoptionFixture{fixture: x}
	i := x.request.Instance
	i.Source.MountMode = dto.MountReadWrite
	f.candidate = dto.Discovery{Kind: i.Kind, Identity: dto.Identity{ContainerID: "example-recreated", ContainerName: i.Identity.ContainerName, StartedAt: nextEpoch}, Sources: []dto.Source{i.Source}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		send := func(v any) {
			b, er := json.Marshal(v)
			if er != nil {
				t.Error(er)
				return
			}
			_, _ = w.Write(b)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/app-version":
			send(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
		case "/api/environments/0/system/health":
			send(map[string]any{})
		case hostPrefix + "/discover":
			if f.discover != nil {
				f.discover()
			}
			send(map[string]any{"success": true, "data": []dto.Discovery{f.candidate}})
		case hostPrefix + "/inventory":
			var instance dto.Instance
			require.NoError(t, json.UnmarshalRead(r.Body, &instance))
			if f.inventory != nil {
				f.inventory(instance)
			}
			if instance.Identity.ContainerID != f.candidate.Identity.ContainerID || instance.Identity.ContainerName != f.candidate.Identity.ContainerName {
				w.WriteHeader(409)
				send(dto.Problem{Code: dto.ContainerChanged, Status: 409})
				return
			}
			send(map[string]any{"success": true, "data": dto.Inventory{Generation: x.a.Generation(), Identity: f.candidate.Identity, Eligibility: dto.Eligibility{Writable: true}}})
		default:
			t.Errorf("unexpected proxy request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	token, key := "example", "example-key"
	require.NoError(t, x.db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES(?,?,?,?,?)", key, "Example key", "example", "example", "environment").Error)
	require.NoError(t, x.db.Create(&environment.Environment{ID: i.EnvironmentID, ApiUrl: server.URL, Name: "Example", Enabled: true, AccessToken: &token, ApiKeyID: &key}).Error)
	hosts := environment.NewEnvironmentService(x.db, server.Client(), nil, nil, x.settings, nil)
	_, err := hosts.CheckAgentCompatibility(t.Context(), i.EnvironmentID)
	require.NoError(t, err)
	transport := &HostTransport{Hosts: hosts}
	f.m = &Module{Modules: x.mods, Transport: transport, Hub: &Hub{DB: x.db.DB, Modules: x.mods, Transport: transport}}
	i.Registration, err = transport.Registration(t.Context(), i.EnvironmentID)
	require.NoError(t, err)
	x.request.Instance = i
	x.request.Registration = i.Registration
	require.NoError(t, f.m.saveInstance(t.Context(), i))
	return f
}
func (f *adoptionFixture) count(t *testing.T) int64 {
	t.Helper()
	var n int64
	require.NoError(t, f.db.Model(&Record{}).Count(&n).Error)
	return n
}
func TestS2AdoptionPersistenceAndRestart(t *testing.T) {
	f := newAdoptionFixture(t)
	a, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
	require.NoError(t, err)
	require.Equal(t, dto.AdoptionAdopted, a.Outcome)
	require.Equal(t, f.candidate.Identity, a.Instance.Identity)
	require.NotEmpty(t, a.OperationID)
	rows, err := f.m.Operations(t.Context(), a.Instance.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	o := rows[0]
	if os.Getenv("UPDATE_PROXY_ADOPTION_FIXTURE") == "1" {
		raw, er := marshal(o)
		require.NoError(t, er)
		content := "// Source: docklet-next/backend/internal/proxy/adoption_test.go\n// Revision: 22319d6f16ef401b1c247c7a4eae599676564095\n// Retrieved: 2026-11-24\n// License: Apache-2.0\n// Deviations: Real persisted hub adoption receipt for web decoding and Recent operations.\n\nexport const persistedAdoption = " + string(raw) + ";\n"
		require.NoError(t, os.WriteFile(filepath.Join("..", "..", "..", "web", "src", "features", "proxy", "adoption-fixture.test-support.ts"), []byte(content), 0644))
	}

	require.Equal(t, dto.AdoptionOperation, o.Kind)
	require.Equal(t, dto.Applied, o.State)
	require.Equal(t, dto.Phase(dto.Applied), o.Phase)
	require.Empty(t, o.Digest)
	require.NotNil(t, o.FinishedAt)
	require.Equal(t, &dto.AdoptionReceipt{PreviousContainerID: f.request.Instance.Identity.ContainerID, ContainerID: f.candidate.Identity.ContainerID, StartedAt: nextEpoch}, o.Adoption)
	// A fresh hub must return the stored receipt without querying or dispatching to the agent.
	h := &Hub{DB: f.db.DB, Modules: f.mods, Transport: &localTransport{queryErr: errors.New("unexpected agent query")}}
	got, err := h.Query(t.Context(), o.ID)
	require.NoError(t, err)
	require.Equal(t, dto.Applied, got.State)
	require.Equal(t, o.Adoption, got.Adoption)
	f.m.reconcile(t.Context(), "")
	again, err := f.m.Adopt(t.Context(), a.Instance.ID)
	require.NoError(t, err)
	require.Equal(t, dto.AdoptionCurrent, again.Outcome)
	require.EqualValues(t, 1, f.count(t))
}
func TestS2EveryIdentityField(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*dto.Discovery)
		reason dto.AdoptionReason
	}{
		{"name", func(d *dto.Discovery) {
			d.Identity.ContainerName = "renamed"
			d.Identity.ContainerID = "example-container"
		}, dto.AdoptionNameChanged},
		{"missing", func(d *dto.Discovery) { d.Identity.ContainerName = "renamed" }, dto.AdoptionContainerMissing},
		{"proxy kind", func(d *dto.Discovery) { d.Kind = dto.Caddy }, dto.AdoptionKindChanged},
		{"canonical", func(d *dto.Discovery) { d.Sources[0].Canonical += "-other" }, dto.AdoptionSourceChanged},
		{"source kind", func(d *dto.Discovery) { d.Sources[0].Kind = dto.SourceKind("volume") }, dto.AdoptionKindChanged},
		{"target", func(d *dto.Discovery) { d.Sources[0].MountTarget = "/different" }, dto.AdoptionSourceChanged},
		{"mount mode", func(d *dto.Discovery) { d.Sources[0].MountMode = dto.MountReadOnly }, dto.AdoptionModeChanged},
		{"writer mode", func(d *dto.Discovery) { d.Sources[0].ReadOnly = true }, dto.AdoptionModeChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAdoptionFixture(t)
			tc.change(&f.candidate)
			a, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
			require.NoError(t, err)
			require.Equal(t, dto.AdoptionRefused, a.Outcome)
			require.Equal(t, tc.reason, a.Reason)
			require.Zero(t, f.count(t))
		})
	}
	f := newAdoptionFixture(t)
	_, why := adoptionCandidate(f.request.Instance, []dto.Discovery{f.candidate, f.candidate})
	require.Equal(t, dto.AdoptionAmbiguous, why)
	same := f.candidate
	same.Identity.ContainerID = f.request.Instance.Identity.ContainerID
	_, why = adoptionCandidate(f.request.Instance, []dto.Discovery{same})
	require.Equal(t, dto.AdoptionContainerMissing, why)
}
func TestS2OpenOperationRefused(t *testing.T) {
	for _, kind := range []string{"reservation", "unfinished"} {
		t.Run(kind, func(t *testing.T) {
			f := newAdoptionFixture(t)
			if kind == "reservation" {
				require.NoError(t, f.db.Create(&reservation{InstanceID: f.request.Instance.ID, OperationID: "held"}).Error)
			} else {
				require.NoError(t, f.db.Create(&Record{ID: "held", InstanceID: f.request.Instance.ID, Request: []byte(`{}`), Result: []byte(`{}`)}).Error)
			}
			a, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
			require.NoError(t, err)
			require.Equal(t, dto.AdoptionOperationOpen, a.Reason)
			saved, err := f.m.Instance(t.Context(), f.request.Instance.ID)
			require.NoError(t, err)
			require.Equal(t, f.request.Instance.Identity, saved.Identity)
		})
	}
}
func TestS2OwnershipAndRegistrationBothOrders(t *testing.T) {
	for _, registrationFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "registration-first", false: "adoption-first"}[registrationFirst], func(t *testing.T) {
			f := newAdoptionFixture(t)
			q := dto.CreateInstanceRequest{EnvironmentID: f.request.Instance.EnvironmentID, Kind: f.candidate.Kind, ContainerID: f.candidate.Identity.ContainerID, MountTarget: f.candidate.Sources[0].MountTarget, Name: "Example", RouteDirectory: "routes"}
			if registrationFirst {
				owner, err := f.m.CreateInstance(t.Context(), q)
				require.NoError(t, err)
				a, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
				require.NoError(t, err)
				require.Equal(t, dto.AdoptionHeldByInstance, a.Reason)
				require.Equal(t, owner.ID, a.SupersededBy)
				require.NoError(t, f.m.RemoveInstance(t.Context(), owner.ID))
				a, err = f.m.Adopt(t.Context(), f.request.Instance.ID)
				require.NoError(t, err)
				require.Equal(t, dto.AdoptionAdopted, a.Outcome)
			} else {
				_, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
				require.NoError(t, err)
				_, err = f.m.CreateInstance(t.Context(), q)
				requireCode(t, err, dto.OperationConflict)
			}
		})
	}
}
func TestS2ConcurrentAdopters(t *testing.T) {
	f := newAdoptionFixture(t)
	arrived, release := make(chan struct{}, 2), make(chan struct{})
	f.discover = func() { arrived <- struct{}{}; <-release }
	var wg sync.WaitGroup
	results := make(chan dto.Adoption, 2)
	errs := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Go(func() { a, err := f.m.Adopt(t.Context(), f.request.Instance.ID); results <- a; errs <- err })
	}
	<-arrived
	<-arrived
	close(release)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	adopted := 0
	for a := range results {
		if a.Outcome == dto.AdoptionAdopted {
			adopted++
		} else {
			require.Equal(t, dto.AdoptionCurrent, a.Outcome)
		}
	}
	require.Equal(t, 1, adopted)
	require.EqualValues(t, 1, f.count(t))
}
func TestS2DiscoveryCASAndRollback(t *testing.T) {
	for _, mode := range []string{"changed-row", "removed-row", "rollback", "registration"} {
		t.Run(mode, func(t *testing.T) {
			f := newAdoptionFixture(t)
			switch mode {
			case "changed-row":
				f.discover = func() {
					i := f.request.Instance
					i.Name = "Concurrent settings"
					require.NoError(t, f.m.saveInstance(t.Context(), i))
				}
			case "removed-row":
				f.discover = func() { require.NoError(t, f.db.Delete(&instanceRow{}, "id = ?", f.request.Instance.ID).Error) }
			case "registration":
				f.discover = func() {
					require.NoError(t, f.db.Exec("UPDATE environments SET credential_revision = credential_revision + 1 WHERE id = ?", f.request.Instance.EnvironmentID).Error)
				}
			case "rollback":
				require.NoError(t, f.db.Exec("CREATE TRIGGER adoption_insert_failure BEFORE INSERT ON proxy_operations BEGIN SELECT RAISE(ABORT, 'example insertion failure'); END").Error)
			}
			a, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
			require.Error(t, err)
			require.NotEqual(t, dto.AdoptionAdopted, a.Outcome)
			require.Zero(t, f.count(t))
			if mode == "registration" {
				requireCode(t, err, dto.RegistrationChanged)
			}
			if mode == "changed-row" {
				saved, e := f.m.Instance(t.Context(), f.request.Instance.ID)
				require.NoError(t, e)
				require.Equal(t, "Concurrent settings", saved.Name)
			}
			if mode == "rollback" {
				saved, e := f.m.Instance(t.Context(), f.request.Instance.ID)
				require.NoError(t, e)
				require.Equal(t, f.request.Instance.Identity, saved.Identity)
			}
		})
	}
}
func TestS2StaleApplyAndMetadataCAS(t *testing.T) {
	f := newAdoptionFixture(t)
	_, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
	require.NoError(t, err)
	_, err = f.m.Hub.Apply(t.Context(), f.request)
	requireCode(t, err, dto.ContainerChanged)
	var n int64
	require.NoError(t, f.db.Model(&reservation{}).Count(&n).Error)
	require.Zero(t, n)
	original, _ := marshal(f.request.Instance)
	err = f.db.Transaction(func(tx *gorm.DB) error { return casInstance(tx, original, f.request.Instance) })
	requireCode(t, err, dto.ContainerChanged)
	saved, err := f.m.Instance(t.Context(), f.request.Instance.ID)
	require.NoError(t, err)
	require.Equal(t, f.candidate.Identity, saved.Identity)
}
func TestS2AdoptApplyRace(t *testing.T) {
	f := newAdoptionFixture(t)
	arrived, release := make(chan struct{}), make(chan struct{})
	f.discover = func() { close(arrived); <-release }
	done := make(chan error, 1)
	go func() {
		a, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
		if err == nil && a.Reason != dto.AdoptionOperationOpen {
			err = errors.New("adoption ignored open operation")
		}
		done <- err
	}()
	<-arrived
	// Apply reserves under the same mutex while discovery remains outside it.
	tr := &localTransport{a: f.a, reg: f.request.Registration, queryErr: errors.New("offline"), drop: true, noAdmission: true}
	f.m.Hub.Transport = tr
	_, err := f.m.Hub.Apply(t.Context(), f.request)
	require.Error(t, err)
	close(release)
	require.NoError(t, <-done)
	require.EqualValues(t, 1, f.count(t))
}
func TestS2ModuleOff(t *testing.T) {
	f := newAdoptionFixture(t)
	require.NoError(t, f.settings.UpdateSetting(context.Background(), features.ProxySettingKey, "false"))
	a, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
	require.NoError(t, err)
	require.Equal(t, dto.AdoptionModuleOff, a.Reason)
	require.Zero(t, f.count(t))
}

func TestS2RegistrationRaceInsideTransaction(t *testing.T) {
	f := newAdoptionFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	f.inventory = func(i dto.Instance) {
		if i.Identity.ContainerID == f.candidate.Identity.ContainerID {
			close(entered)
			<-release
		}
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.m.CreateInstance(t.Context(), dto.CreateInstanceRequest{EnvironmentID: f.request.Instance.EnvironmentID, Kind: f.candidate.Kind, ContainerID: f.candidate.Identity.ContainerID, MountTarget: f.candidate.Sources[0].MountTarget, Name: "Example", RouteDirectory: "routes"})
		result <- err
	}()
	<-entered
	_, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
	require.NoError(t, err)
	close(release)
	requireCode(t, <-result, dto.OperationConflict)
	instances, err := f.m.Instances(t.Context())
	require.NoError(t, err)
	require.Len(t, instances, 1)
}
func TestS2CreateOperationAndSettingsFollowReplacement(t *testing.T) {
	for _, entry := range []string{"apply", "settings"} {
		t.Run(entry, func(t *testing.T) {
			f := newAdoptionFixture(t)
			f.m.Hub.Transport = &localTransport{a: f.a, reg: f.request.Registration}
			f.f.epoch = nextEpoch
			if entry == "apply" {
				o, err := f.m.CreateOperation(t.Context(), f.request.Instance.ID, dto.OperationRequest{Target: f.request.Target, Expected: f.request.Expected, GraphHashes: f.request.GraphHashes, Change: f.request.Change})
				require.NoError(t, err)
				require.Equal(t, dto.Verified, o.State)
			} else {
				_, err := f.m.UpdateInstance(t.Context(), f.request.Instance.ID, dto.Settings{Name: "Updated", RouteDirectory: "routes"})
				require.NoError(t, err)
			}
			stored, err := f.m.Instance(t.Context(), f.request.Instance.ID)
			require.NoError(t, err)
			require.Equal(t, f.candidate.Identity, stored.Identity)
			operations, err := f.m.Operations(t.Context(), stored.ID)
			require.NoError(t, err)
			adoptions := 0
			for _, o := range operations {
				if o.Kind == dto.AdoptionOperation {
					adoptions++
				}
			}
			require.Equal(t, 1, adoptions)
		})
	}
}
func TestS2CreateOperationDoesNotOverwriteConcurrentIdentity(t *testing.T) {
	f := newAdoptionFixture(t)
	f.candidate.Identity = f.request.Instance.Identity
	f.inventory = func(dto.Instance) {
		changed := f.request.Instance
		changed.Identity.ContainerID = "concurrent-container"
		require.NoError(t, f.m.saveInstance(t.Context(), changed))
	}
	_, err := f.m.CreateOperation(t.Context(), f.request.Instance.ID, dto.OperationRequest{Target: f.request.Target, Expected: f.request.Expected, GraphHashes: f.request.GraphHashes, Change: f.request.Change})
	requireCode(t, err, dto.ContainerChanged)
	saved, err := f.m.Instance(t.Context(), f.request.Instance.ID)
	require.NoError(t, err)
	require.Equal(t, "concurrent-container", saved.Identity.ContainerID)
	require.Zero(t, f.count(t))
}

func TestS2RegistrationWinsWhileAdoptionDiscovers(t *testing.T) {
	f := newAdoptionFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	f.discover = func() {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	result := make(chan dto.Adoption, 1)
	errs := make(chan error, 1)
	go func() { a, err := f.m.Adopt(t.Context(), f.request.Instance.ID); result <- a; errs <- err }()
	<-entered
	// Registration's discovery is the second call and passes the barrier.
	owner, err := f.m.CreateInstance(t.Context(), dto.CreateInstanceRequest{EnvironmentID: f.request.Instance.EnvironmentID, Kind: f.candidate.Kind, ContainerID: f.candidate.Identity.ContainerID, MountTarget: f.candidate.Sources[0].MountTarget, Name: "Example", RouteDirectory: "routes"})
	require.NoError(t, err)
	close(release)
	require.NoError(t, <-errs)
	a := <-result
	require.Equal(t, dto.AdoptionHeldByInstance, a.Reason)
	require.Equal(t, owner.ID, a.SupersededBy)
}
func TestS2OldUnknownKindReaderKeepsRecovery(t *testing.T) {
	f := newAdoptionFixture(t)
	a, err := f.m.Adopt(t.Context(), f.request.Instance.ID)
	require.NoError(t, err)
	var row Record
	require.NoError(t, f.db.First(&row, "id = ?", a.OperationID).Error)
	// An older discriminator cannot decode the newly introduced kind and keeps it held.
	row.Request = []byte(`{"kind":"future-example"}`)
	_, o, err := decodeRecord(row)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, o.State)
	require.Equal(t, dto.UnknownOperationKind, o.Reason)
	require.Nil(t, o.FinishedAt)
}

// The transport fixtures use the actual discovery producer's identity/source
// shape, including separate proxy mount mode and module writer authority.
func TestS2RealDiscoveryProducerMatchesAdoption(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", root))
	c := container.InspectResponse{ID: "example-container", Name: "/example-proxy", State: &container.State{Running: true, StartedAt: initialEpoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root, Destination: "/etc/proxy", RW: true}}}
	runtime := dockerSeam(t, x, &c)
	source, err := runtime.Source(t.Context(), c, c.Mounts[0])
	require.NoError(t, err)
	i := x.request.Instance
	i.Source = source
	c.ID = "example-recreated"
	c.State.StartedAt = nextEpoch
	rows, err := runtime.ProjectDiscovery(t.Context(), []container.Summary{{ID: c.ID, Image: "traefik:v3.7.16"}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	identity, why := adoptionCandidate(i, rows)
	require.Empty(t, why)
	require.Equal(t, c.ID, identity.ContainerID)
	require.Equal(t, nextEpoch, identity.StartedAt)
	require.Equal(t, dto.MountReadWrite, rows[0].Sources[0].MountMode)
	require.False(t, rows[0].Sources[0].ReadOnly)
	c.Mounts[0].RW = false
	rows, err = runtime.ProjectDiscovery(t.Context(), []container.Summary{{ID: c.ID, Image: "traefik:v3.7.16"}})
	require.NoError(t, err)
	_, why = adoptionCandidate(i, rows)
	require.Equal(t, dto.AdoptionModeChanged, why)
}
