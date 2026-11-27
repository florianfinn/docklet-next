// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original evidence tests of the combined update (verify S1 r1 class tables).

package upgrade

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
)

func TestFleetAdmitReplacesOnlyValidatedSettledPlans(t *testing.T) {
	cases := map[string]func(core.FleetPlan) string{
		"unsupported-settled": func(core.FleetPlan) string {
			return `{"schemaVersion":2,"fleetId":"other","runId":"example-old-parent","phase":"settled"}`
		},
		"settled-with-pending-targets": func(p core.FleetPlan) string {
			p.Phase, p.Outcome = "settled", "success"
			p.RunID = "example-old-parent"
			return mustJSON(t, p)
		},
		"settled-with-pending-agent": func(p core.FleetPlan) string {
			p.Phase, p.Outcome, p.Hub.State, p.Hub.Outcome = "settled", "success", core.TargetSkipped, "current"
			p.RunID = "example-old-parent"
			return mustJSON(t, p)
		},
		"agents-with-reserved-hub": func(p core.FleetPlan) string {
			p.Phase, p.Hub.State, p.Hub.Outcome = "agents", core.TargetReserved, "healthy"
			p.RunID = "example-old-parent"
			return mustJSON(t, p)
		},
	}
	for name, stored := range cases {
		t.Run(name, func(t *testing.T) {
			db := updateDB(t)
			hub := newFleetHub(t, db, "1.0.0", "1.0.1")
			addHosts(t, db, "example-a")
			_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.NoError(t, err)
			plan := readPlan(t, hub.Service)
			old := activity.Activity{BaseModel: database.BaseModel{ID: "example-old-parent"}, EnvironmentID: "0", Type: activityDTO.TypeFleetUpdate, Status: activityDTO.StatusRunning, StartedAt: time.Now()}
			require.NoError(t, db.Create(&old).Error)
			require.NoError(t, db.Model(&kv.KVEntry{}).Where("key=?", core.FleetKey).Update("value", stored(plan)).Error)
			_, err = hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.True(t, core.HasCode(err, dto.FleetBusy), "admitted over an unreadable plan: %v", err)
			done, err := hub.advanceFleet(t.Context())
			require.NoError(t, err)
			require.True(t, done)
			require.NoError(t, db.First(&old, "id=?", old.ID).Error)
			require.Equal(t, activityDTO.StatusFailed, old.Status, "the owner of the replaced plan was not failed")
			_, err = hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.NoError(t, err)
		})
	}
}

func TestFleetCurrentHubEvidenceMustSurviveAdmission(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	configureUpdateFeed(t, f)
	f.currentContract.Store(true)
	f.hub.Stop()
	entered, release := make(chan struct{}), make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/app-version" {
			close(entered)
			<-release
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(slow.Close)
	require.NoError(t, f.db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: "example-slow"}, Name: "Slow", ApiUrl: slow.URL, Enabled: true, AccessToken: new("example"), CredentialRevision: 1}).Error)
	hub := restartHub(t, f, false)
	admitted := make(chan error, 1)
	go func() {
		_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
		admitted <- err
	}()
	<-entered
	// The qualifying agent's reading completed; it is removed while another host is still read.
	require.Eventually(t, func() bool { return f.checks.Load() > 0 }, 10*time.Second, 10*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, f.db.Unscoped().Delete(&f.remote).Error)
	close(release)
	err := <-admitted
	require.True(t, core.HasCode(err, dto.FleetCurrent), "admitted without a qualifying agent: %v", err)
	plan, _, err := hub.fleet().Read(t.Context())
	require.NoError(t, err)
	require.Nil(t, plan)
}

func TestFleetFrozenTargetIgnoresNewerFeed(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	configureUpdateFeed(t, f)
	f.currentContract.Store(true)
	f.hub.Stop()
	hub := restartHub(t, f, false)
	_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.NoError(t, err)
	// After admission the hub runs 1.0.2 and the feed offers it.
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.2","published_at":"2026-11-02T12:00:00Z"}]`))
	}))
	t.Cleanup(feed.Close)
	hub.versions = version.NewVersionService(feed.Client(), false, feed.URL, "1.0.2", "example", nil, nil)
	hub.runningVersion = func() string { return "1.0.2" }
	advanceUntil(t, hub, 2)
	op, err := hub.remoteStore(f.remote.ID).Read(t.Context())
	require.NoError(t, err)
	require.NotNil(t, op)
	require.Equal(t, "1.0.1", op.TargetVersion)
}

func TestFleetRecoveryCheckFailureSkipsAgent(t *testing.T) {
	updateStamp(t)
	for _, mode := range []string{"status", "drop"} {
		t.Run(mode, func(t *testing.T) {
			f := newUpdateTransport(t, false)
			configureUpdateFeed(t, f)
			f.currentContract.Store(true)
			f.hub.Stop()
			hub := restartHub(t, f, false)
			seedAgentPlan(t, hub, f.remote.ID)
			f.checkFailure.Store(mode)
			advanceUntil(t, hub, 1)
			target := fleetTarget(t, hub, f.remote.ID)
			require.Equal(t, core.TargetSkipped, target.State)
			require.Equal(t, "agent.unavailable", target.Code)
			require.Zero(t, f.posts.Load())
		})
	}
}

func TestFleetEdgeAgentConnectsOnDemand(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransportMode(t, true, true)
	configureUpdateFeed(t, f)
	f.currentContract.Store(true)
	f.hub.Stop()
	hub := restartHub(t, f, true)
	seedAgentPlan(t, hub, f.remote.ID)
	hub.wakeFleet()
	require.Eventually(t, func() bool { return fleetTarget(t, hub, f.remote.ID).State != core.TargetPending }, 30*time.Second, 50*time.Millisecond)
	target := fleetTarget(t, hub, f.remote.ID)
	require.Equal(t, core.TargetReserved, target.State, "the Edge agent was not asked to connect: %s", target.Code)
	select {
	case <-f.daemon.created:
	case <-time.After(10 * time.Second):
		t.Fatal("helper was not started through the on-demand tunnel")
	}
	require.EqualValues(t, 1, f.posts.Load())
}
