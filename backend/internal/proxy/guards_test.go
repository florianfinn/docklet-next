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
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"strings"
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

// L2 wiring: the agent-online callback does not wait for its pass and the pass queries.
func TestL2AgentOnlinePassIsAsynchronous(t *testing.T) {
	x, tr, m := reconcilerFixture(t, true)
	var online func(context.Context, string)
	m.startReconciler(func(f func(context.Context, string)) func() { online = f; return func() {} }, time.Hour)
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, tr.queries.Load())
	persistRecord(t, x, dto.Pending)
	returnsWithin(t, "the agent-online callback", func() { online(t.Context(), x.request.Instance.EnvironmentID) })
	require.Eventually(t, func() bool { return tr.queries.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
}

// L2 wiring: the periodic pass follows a record that appeared after startup.
func TestL2PeriodicPassFollowsLateRecords(t *testing.T) {
	x, tr, m := reconcilerFixture(t, false)
	m.startReconciler(nil, 50*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
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
	root := t.TempDir()
	base := container.InspectResponse{ID: "example-recreated", Name: "/example-proxy", State: &container.State{Running: true, StartedAt: nextEpoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root, Destination: "/etc/proxy", RW: true}}}
	for _, tt := range []struct {
		name   string
		change func(*container.InspectResponse)
		ok     bool
	}{
		{"same source", func(*container.InspectResponse) {}, true},
		{"changed source", func(c *container.InspectResponse) { c.Mounts[0].Source = root + "-other" }, false},
		{"read-only", func(c *container.InspectResponse) { c.Mounts[0].RW = false }, false},
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
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
				switch {
				case p == "/_ping":
					w.Header().Set("API-Version", "1.54")
					_, _ = w.Write([]byte("OK"))
				case strings.HasSuffix(p, "/json") && tt.change != nil:
					_ = json.NewEncoder(w).Encode(c)
				case strings.HasSuffix(p, "/archive") && tt.change != nil:
					raw, _ := json.Marshal(container.PathStat{Name: path.Base(r.URL.Query().Get("path")), Mode: os.ModeDir | 0755})
					w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			d := &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
			i := x.request.Instance
			i.Source = dto.Source{Kind: dto.ProjectSource, Canonical: root, MountTarget: "/etc/proxy"}
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

// L5: the hub refuses a receipt from another generation or with another digest.
func TestL5HubRefusesMismatchingReceipt(t *testing.T) {
	for _, field := range []string{"generation", "digest"} {
		t.Run(field, func(t *testing.T) {
			x := newFixture(t)
			_, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			persistRecord(t, x, dto.RecoveryRequired)
			tr := &localTransport{a: x.a, reg: x.request.Registration, releaseMutate: func(o *dto.Operation) {
				if field == "generation" {
					o.Generation = "example-other-generation"
				} else {
					o.Digest = "example-other-digest"
				}
			}}
			h := &Hub{DB: x.db.DB, Modules: x.mods, Transport: tr}
			_, err = h.Release(t.Context(), x.request.ID, x.f.files["route.yml"].FileState, "example-admin")
			var p *dto.Problem
			require.ErrorAs(t, err, &p)
			require.Equal(t, dto.OperationConflict, p.Code)
			var row Record
			require.NoError(t, x.db.First(&row, "id = ?", x.request.ID).Error)
			require.Equal(t, dto.RecoveryRequired, row.State)
			var n int64
			require.NoError(t, x.db.Model(&reservation{}).Count(&n).Error)
			require.EqualValues(t, 1, n)
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
