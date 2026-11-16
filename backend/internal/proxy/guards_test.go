// Source: docs/design/proxy-module.md
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original S2 R2 guard proofs: reconciler wiring, identity refusal, rebind, receipt identity and settlement tolerance.

package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	dockerinternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func returnsWithin(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { fn(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.Failf(t, "blocked", "%s must not wait for a reconciliation pass", what)
	}
}

func reconcilerFixture(t *testing.T, block bool) (*fixture, *localTransport, *Module) {
	t.Helper()
	x := newFixture(t)
	tr := &localTransport{a: x.a, reg: x.request.Registration}
	if block {
		tr.block = make(chan struct{})
	}
	m := &Module{Hub: &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}}
	t.Cleanup(func() {
		if tr.block != nil {
			select {
			case <-tr.block:
			default:
				close(tr.block)
			}
		}
		require.NoError(t, m.Stop(context.Background()))
	})
	return x, tr, m
}

// L2 wiring: startup does not wait for its pass and the pass queries the unfinished record.
func TestL2StartupPassIsAsynchronous(t *testing.T) {
	x, tr, m := reconcilerFixture(t, true)
	persistRecord(t, x, dto.Pending)
	returnsWithin(t, "startup", func() { m.startReconciler(nil, time.Hour) })
	require.Eventually(t, func() bool { return tr.queries.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
}

// startupDone records finished passes; tests wait for the startup pass before adding records.
func startupDone(m *Module) chan string {
	done := make(chan string, 16)
	m.passHook = func(key string, finished bool) {
		if finished {
			done <- key
		}
	}
	return done
}
func waitPass(t *testing.T, done chan string, key string) {
	t.Helper()
	select {
	case got := <-done:
		require.Equal(t, key, got)
	case <-time.After(5 * time.Second):
		require.Failf(t, "no pass", "pass %s did not finish", key)
	}
}

// L2 wiring: the agent-online callback does not wait for its pass and the pass queries.
func TestL2AgentOnlinePassIsAsynchronous(t *testing.T) {
	x, tr, m := reconcilerFixture(t, true)
	done := startupDone(m)
	var online func(context.Context, string)
	m.startReconciler(func(f func(context.Context, string)) func() { online = f; return func() {} }, time.Hour)
	waitPass(t, done, "all")
	require.Zero(t, tr.queries.Load())
	persistRecord(t, x, dto.Pending)
	returnsWithin(t, "the agent-online callback", func() { online(t.Context(), x.request.Instance.EnvironmentID) })
	require.Eventually(t, func() bool { return tr.queries.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
}

// L2 wiring: the periodic pass follows a record that appeared after startup.
func TestL2PeriodicPassFollowsLateRecords(t *testing.T) {
	x, tr, m := reconcilerFixture(t, false)
	done := startupDone(m)
	m.startReconciler(nil, 50*time.Millisecond)
	waitPass(t, done, "all")
	require.Zero(t, tr.queries.Load())
	persistRecord(t, x, dto.Pending)
	require.Eventually(t, func() bool { return tr.queries.Load() >= 1 }, 5*time.Second, 10*time.Millisecond)
}

// N1: the all-hosts pass skips an environment its own pass is following.
func TestReconcilerOnePassPerEnvironment(t *testing.T) {
	x, tr, m := reconcilerFixture(t, true)
	persistRecord(t, x, dto.Pending)
	m.reconcileAsync(x.request.Instance.EnvironmentID)
	require.Eventually(t, func() bool { return tr.queries.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	m.reconcileAsync("")
	time.Sleep(100 * time.Millisecond)
	require.EqualValues(t, 1, tr.queries.Load(), "no second concurrent pass may query the same environment")
}

// N2: Stop waits for a running pass.
func TestReconcilerStopWaitsForPasses(t *testing.T) {
	x, tr, m := reconcilerFixture(t, true)
	persistRecord(t, x, dto.Pending)
	m.startReconciler(nil, time.Hour)
	require.Eventually(t, func() bool { return tr.queries.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	stopped := make(chan struct{})
	go func() { _ = m.Stop(context.Background()); close(stopped) }()
	select {
	case <-stopped:
		require.Fail(t, "Stop returned while a pass was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(tr.block)
	require.Eventually(t, func() bool {
		select {
		case <-stopped:
			return true
		default:
			return false
		}
	}, 5*time.Second, 10*time.Millisecond)
}

// L4: a non-active unfinished operation whose identity changed becomes recovery-required
// (agent), the hub records it, and it is releasable after a rebind.
func TestL4IdentityChangeBecomesReleasableRecovery(t *testing.T) {
	x := newFixture(t)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Observing {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	x.a.afterCommit = nil
	persistRecord(t, x, dto.Pending)
	x.f.checkErr = dto.Fail(dto.ContainerChanged, 409)
	tr := &localTransport{a: x.a, reg: x.request.Registration}
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	o, err := h.Query(t.Context(), x.request.ID)
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, o.State)
	require.Equal(t, dto.ContainerChanged, o.Reason)
	o, err = h.Release(t.Context(), x.request.ID, x.f.files["route.yml"].FileState, "example-admin")
	require.NoError(t, err)
	require.Equal(t, dto.Released, o.State)
	var n int64
	require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
	require.Zero(t, n)
}

// L4 rebind on the real Docker inspect seam: only the same source at the same mount target is followed.
func TestL4RebindRealDockerSeam(t *testing.T) {
	for _, kind := range []struct {
		name         string
		mount, other mount.Type
		source       dto.SourceKind
	}{{"bind", mount.TypeBind, mount.TypeVolume, dto.ProjectSource}, {"volume", mount.TypeVolume, mount.TypeBind, dto.NamedVolume}} {
		t.Run(kind.name, func(t *testing.T) { rebindCases(t, kind.mount, kind.other, kind.source) })
	}
}
func rebindCases(t *testing.T, kind, other mount.Type, source dto.SourceKind) {
	root := t.TempDir()
	base := container.InspectResponse{ID: "example-recreated", Name: "/example-proxy", State: &container.State{Running: true, StartedAt: nextEpoch}, Mounts: []container.MountPoint{{Type: kind, Source: root, Destination: "/etc/proxy", RW: true}}}
	for _, tt := range []struct {
		name   string
		change func(*container.InspectResponse)
		ok     bool
	}{
		{"same source", func(*container.InspectResponse) {}, true},
		{"changed kind", func(c *container.InspectResponse) { c.Mounts[0].Type = other }, false},
		{"changed source", func(c *container.InspectResponse) { c.Mounts[0].Source = root + "-other" }, false},
		{"proxy-side read-only", func(c *container.InspectResponse) { c.Mounts[0].RW = false }, true},
		{"other mount target", func(c *container.InspectResponse) { c.Mounts[0].Destination = "/etc/elsewhere" }, false},
		{"gone", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x := newFixture(t)
			require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", root))
			c := base
			c.Mounts = append([]container.MountPoint{}, base.Mounts...)
			if tt.change != nil {
				tt.change(&c)
			}
			var inspected *container.InspectResponse
			if tt.change != nil {
				inspected = &c
			}
			d := dockerSeam(t, x, inspected)
			i := x.request.Instance
			i.Source = dto.Source{Kind: source, Canonical: root, MountTarget: "/etc/proxy"}
			if source == dto.NamedVolume { // registered as classified: helper-only, read-only
				i.Source.ReadOnly, i.Source.Reason = true, dto.UnsupportedMetadata
			}
			got, err := d.Rebind(t.Context(), i)
			if !tt.ok {
				var p *dto.Problem
				require.ErrorAs(t, err, &p)
				require.Equal(t, dto.ContainerChanged, p.Code)
				require.Equal(t, i.Identity, got.Identity)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "example-recreated", got.Identity.ContainerID)
			require.Equal(t, nextEpoch, got.Identity.StartedAt)
		})
	}
}

// dockerSeam serves one container inspect and directory stats through a real Docker
// client; a nil container answers every inspect with not-found.
func dockerSeam(t *testing.T, x *fixture, c *container.InspectResponse) *DockerRuntime {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
		switch {
		case p == "/_ping":
			w.Header().Set("API-Version", "1.54")
			_, _ = w.Write([]byte("OK"))
		case p == "/info": // bind sources run the real bind-path rules
			_, _ = w.Write([]byte(`{"DockerRootDir":"/example/docker"}`))
		case p == "/containers/json":
			_ = json.NewEncoder(w).Encode([]container.Summary{})
		case strings.HasSuffix(p, "/json") && c != nil:
			_ = json.NewEncoder(w).Encode(c)
		case strings.HasSuffix(p, "/archive") && c != nil:
			raw, _ := json.Marshal(container.PathStat{Name: path.Base(r.URL.Query().Get("path")), Mode: os.ModeDir | 0755})
			w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	return &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
}

// L5: the hub refuses a receipt with another ID, generation, registration or digest;
// a receipt naming another existing record leaves that record untouched.
func TestL5HubRefusesMismatchingReceipt(t *testing.T) {
	for _, field := range []string{"id", "generation", "registration", "digest"} {
		t.Run(field, func(t *testing.T) {
			x := newFixture(t)
			_, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			persistRecord(t, x, dto.RecoveryRequired)
			other := x.request
			other.ID = "example-other-operation"
			other.Instance.ID = "example-other-instance"
			y := *x
			y.request = other
			persistRecord(t, &y, dto.Pending)
			tr := &localTransport{a: x.a, reg: x.request.Registration, releaseMutate: func(o *dto.Operation) {
				switch field {
				case "id":
					o.ID = other.ID
				case "generation":
					o.Generation = "example-other-generation"
				case "registration":
					o.Registration = "example-other-registration"
				default:
					o.Digest = "example-other-digest"
				}
			}}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			_, err = h.Release(t.Context(), x.request.ID, x.f.files["route.yml"].FileState, "example-admin")
			var p *dto.Problem
			require.ErrorAs(t, err, &p)
			require.Equal(t, dto.OperationConflict, p.Code)
			var row, unrelated Record
			require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
			require.Equal(t, dto.RecoveryRequired, row.State)
			require.NoError(t, x.db.First(&unrelated, "id = ?", other.ID).Error)
			require.Equal(t, dto.Pending, unrelated.State)
			var n int64
			require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
			require.EqualValues(t, 2, n)
		})
	}
}

// N8: a registration change during Release keeps the record and reservation.
func TestReleaseRechecksRegistration(t *testing.T) {
	x := newFixture(t)
	recoveryRequired(t, x)
	persistRecord(t, x, dto.RecoveryRequired)
	tr := &localTransport{a: x.a, reg: x.request.Registration}
	tr.afterRelease = func() { tr.reg = "example-replaced" }
	h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
	o, err := h.Release(t.Context(), x.request.ID, x.f.files["route.yml"].FileState, "example-admin")
	require.NoError(t, err)
	require.Equal(t, dto.RecoveryRequired, o.State)
	require.Equal(t, dto.RegistrationChanged, o.Reason)
	var n int64
	require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
}

// L6: an undecodable row is released only for its own registration.
func TestL6UndecodableReleaseNeedsItsRegistration(t *testing.T) {
	x := newFixture(t)
	require.NoError(t, x.db.Create(&Journal{ID: "broken-one", Source: "host\x00" + x.request.Instance.Source.Canonical, Generation: x.a.Generation(), Digest: "example", Registration: x.request.Registration, State: dto.Pending, Phase: dto.Staging, Intent: []byte("{"), Result: []byte("{"), CreatedAt: time.Now().UTC()}).Error)
	q := dto.Query{ID: "broken-one", Instance: x.request.Instance, Generation: x.a.Generation(), Registration: "example-other"}
	_, err := x.a.Release(t.Context(), dto.ReleaseRequest{Query: q, Confirmed: x.f.files["route.yml"].FileState, Actor: "example-admin"})
	require.Error(t, err)
	var j Journal
	require.NoError(t, x.db.First(&j, "id = ?", "broken-one").Error)
	require.Nil(t, j.FinishedAt, "the row must stay locked")
}

// N8: a pruning failure does not fail startup settlement.
func TestSettleToleratesPruneFailure(t *testing.T) {
	x := newFixture(t)
	require.NoError(t, x.db.Callback().Delete().Before("gorm:delete").Register("example:fail-prune", func(tx *gorm.DB) { _ = tx.AddError(errors.New("example prune failure")) }))
	require.NoError(t, x.a.Settle(t.Context()))
}

// Start installs the reconciler: its startup pass and the periodic pass both run.
func TestStartInstallsReconciler(t *testing.T) {
	x, tr, m := reconcilerFixture(t, false)
	m.Agent, m.every = x.a, 50*time.Millisecond
	done := startupDone(m)
	persistRecord(t, x, dto.Pending)
	returnsWithin(t, "Start", func() { require.NoError(t, m.Start(t.Context())) })
	waitPass(t, done, "all")
	require.EqualValues(t, 1, tr.queries.Load(), "the startup pass must query the unfinished record")
	waitPass(t, done, "all")
}

// Stop against a callback the online notifier already captured: no pass is admitted
// after Stop, and concurrent callbacks never race the WaitGroup.
func TestReconcilerStopRefusesCapturedCallback(t *testing.T) {
	x, _, m := reconcilerFixture(t, false)
	var started atomic.Int32
	m.passHook = func(_ string, finished bool) {
		if !finished {
			started.Add(1)
		}
	}
	var online func(context.Context, string)
	m.startReconciler(func(f func(context.Context, string)) func() { online = f; return func() {} }, time.Hour)
	require.NoError(t, m.Stop(context.Background()))
	before := started.Load()
	online(t.Context(), x.request.Instance.EnvironmentID)
	m.reconcileAsync("")
	// Anything admitted by the late calls has finished once the group drains.
	m.wg.Wait()
	require.Equal(t, before, started.Load(), "no pass may start after Stop")
	for range 200 {
		n := &Module{Hub: m.Hub}
		var captured func(context.Context, string)
		n.startReconciler(func(f func(context.Context, string)) func() { captured = f; return func() {} }, time.Hour)
		var calls sync.WaitGroup
		calls.Add(1)
		go func() {
			defer calls.Done()
			for i := range 10 {
				captured(context.Background(), fmt.Sprint("example-host-", i))
				runtime.Gosched()
			}
		}()
		require.NoError(t, n.Stop(context.Background()))
		calls.Wait()
	}
}

// L6 continuation: an undecodable row and a recovery-required row do not keep Settle
// from finishing a later settleable row.
func TestL6SettlementContinuesAfterFailedRows(t *testing.T) {
	x := newFixture(t)
	require.NoError(t, x.db.Create(&Journal{ID: "broken-first", Source: "example-other-source", Generation: x.a.Generation(), Digest: "example", Registration: x.request.Registration, State: dto.Pending, Phase: dto.Staging, Intent: []byte("{"), Result: []byte("{"), CreatedAt: time.Now().UTC()}).Error)
	stuck := x.request
	stuck.ID, stuck.Instance.Source.Canonical = "stuck-second", "/example/projects/stuck"
	y := *x
	y.request = stuck
	recoveryRequired(t, &y)
	x.f.files["route.yml"] = Snapshot([]byte("old"), true)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Observing {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	x.a.afterCommit = nil
	require.NoError(t, x.a.Settle(t.Context()))
	rows := map[string]Journal{}
	var all []Journal
	require.NoError(t, x.db.Find(&all).Error)
	for _, j := range all {
		rows[j.ID] = j
	}
	require.Equal(t, dto.Verified, rows[x.request.ID].State, "the settleable row after the failed rows must finish")
	require.Nil(t, rows["broken-first"].FinishedAt)
	require.Equal(t, dto.RecoveryRequired, rows["stuck-second"].State)
}
