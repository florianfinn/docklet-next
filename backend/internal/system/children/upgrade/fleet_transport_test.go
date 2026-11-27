// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original real-transport tests for the combined update's agent phase.

package upgrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fleetOwner configures a hub that is already current, so the plan starts in
// the agent phase and every agent request crosses the real transport.
func fleetOwner(s *Service) {
	s.allowed = func(context.Context, string) (bool, error) { return true, nil }
	s.localCheck = func(context.Context) (dto.SelfUpdateCheck, error) {
		return dto.SelfUpdateCheck{Version: "1.0.1", Eligibility: dto.SelfUpdateEligibility{State: "current"}}, nil
	}
}
func restartHub(t *testing.T, f *updateTransportFixture, start bool) *Service {
	t.Helper()
	hub := NewService(f.db, nil, nil, &config.Config{})
	hub.SetEnvironments(f.hub.environments)
	hub.runningVersion = func() string { return "1.0.1" }
	hub.resolve = f.hub.resolve
	fleetOwner(hub)
	if start {
		require.NoError(t, hub.Start(t.Context()))
		t.Cleanup(hub.Stop)
	}
	return hub
}
func fleetTarget(t *testing.T, s *Service, id string) core.FleetTarget {
	t.Helper()
	plan := readPlan(t, s)
	return *plan.Target(id)
}

func TestFleetAgentsDispatchOnceAcrossRestartDirectAndEdge(t *testing.T) {
	updateStamp(t)
	for _, edge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[edge], func(t *testing.T) {
			f := newUpdateTransport(t, edge)
			configureUpdateFeed(t, f)
			f.currentContract.Store(true)
			closed := httptest.NewServer(http.NotFoundHandler())
			closed.Close()
			require.NoError(t, f.db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: "example-offline"}, Name: "Offline", ApiUrl: closed.URL, Enabled: true, AccessToken: new("example"), CredentialRevision: 1}).Error)
			f.hub.Stop()
			hub := restartHub(t, f, true)
			started, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.NoError(t, err)
			require.Equal(t, "current", started.HubPhase)
			select {
			case <-f.daemon.created:
			case <-time.After(10 * time.Second):
				t.Fatal("agent helper was not started")
			}
			require.Eventually(t, func() bool {
				return fleetTarget(t, hub, f.remote.ID).State == core.TargetReserved && fleetTarget(t, hub, "example-offline").Terminal()
			}, 15*time.Second, 50*time.Millisecond)
			require.Equal(t, "agent.unavailable", fleetTarget(t, hub, "example-offline").Code)
			// A second hub process observes the dispatched operation and never sends again.
			hub.Stop()
			seen := f.postChecks.Load()
			second := restartHub(t, f, true)
			require.Eventually(t, func() bool { return f.postChecks.Load() > seen+1 }, 15*time.Second, 50*time.Millisecond)
			require.EqualValues(t, 1, f.posts.Load())
			second.Stop()
			f.agent.Stop()
			target := fleetTarget(t, second, f.remote.ID)
			require.NoError(t, core.WriteJSON(core.ResultPath(f.agent.data, target.OpID), core.Result{OpID: target.OpID, TargetRef: target.TargetRef, Phase: "healthy"}))
			f.version.Store("1.0.1")
			done, err := f.agent.observer().Advance(t.Context())
			require.NoError(t, err)
			require.True(t, done)
			done, err = second.advanceRemote(t.Context(), second.remoteStore(f.remote.ID))
			require.NoError(t, err)
			require.True(t, done)
			// A third process starts after the agent settled and before the aggregate was written.
			third := restartHub(t, f, true)
			require.Eventually(t, func() bool { return readPlan(t, third).Phase == "settled" }, 10*time.Second, 50*time.Millisecond)
			third.Stop()
			plan := readPlan(t, third)
			require.Equal(t, "settled", plan.Phase)
			require.Equal(t, "partial", plan.Outcome)
			require.Equal(t, "healthy", plan.Target(f.remote.ID).Outcome)
			var parent activity.Activity
			require.NoError(t, f.db.First(&parent, "id=?", started.RunID).Error)
			require.Equal(t, activityDTO.StatusFailed, parent.Status)
			require.Equal(t, "partial", parent.Metadata["jobStatus"])
			var children []activity.Activity
			require.NoError(t, f.db.Where("batch_id=?", started.RunID).Order("environment_id").Find(&children).Error)
			// One child per host: the current hub, the skipped offline agent and the updated agent.
			require.Len(t, children, 3)
			hosts := map[string]bool{}
			for _, child := range children {
				hosts[child.EnvironmentID] = true
			}
			require.Equal(t, map[string]bool{"0": true, "example-offline": true, f.remote.ID: true}, hosts)
			require.EqualValues(t, 1, f.posts.Load())
			f.daemon.mu.Lock()
			creates := len(f.daemon.creates)
			f.daemon.mu.Unlock()
			require.Equal(t, 1, creates)
		})
	}
}

func TestFleetReservedWithoutMarkerIsNeverSent(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	configureUpdateFeed(t, f)
	f.currentContract.Store(true)
	f.hub.Stop()
	owner := restartHub(t, f, false)
	_, err := owner.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.NoError(t, err)
	plan := readPlan(t, owner)
	agent := *plan.Target(f.remote.ID)
	// The process reserved the operation and died before its dispatch marker.
	op := core.Operation{OpID: agent.OpID, Role: "agent", EnvironmentID: f.remote.ID, CredentialRevision: agent.CredentialRevision, TargetRef: repository("agent") + ":1.0.1@sha256:" + testDigest, TargetVersion: "1.0.1", Deadline: time.Now().Add(core.OperationTimeout)}
	_, _, err = owner.reserveRun(t.Context(), owner.remoteStore(f.remote.ID), op, "operator", "operator", &fleetLink{parentRunID: plan.RunID, registeredAt: agent.RegisteredAt})
	require.NoError(t, err)
	restarted := restartHub(t, f, true)
	require.Eventually(t, func() bool { return fleetTarget(t, restarted, f.remote.ID).State == core.TargetReserved }, 10*time.Second, 50*time.Millisecond)
	require.Never(t, func() bool { return f.posts.Load() > 0 }, 5*time.Second, 100*time.Millisecond, "a reserved operation without its marker was sent after restart")
}

func TestFleetBusyAgentIsRetriedThenSkipped(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	configureUpdateFeed(t, f)
	f.currentContract.Store(true)
	f.hub.Stop()
	hub := restartHub(t, f, false)
	busy := activity.Activity{BaseModel: database.BaseModel{ID: uuid.NewString()}, EnvironmentID: f.remote.ID, Type: activityDTO.TypeJobRun, Status: activityDTO.StatusRunning, StartedAt: time.Now()}
	require.NoError(t, f.db.Create(&busy).Error)
	_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.NoError(t, err)
	advanceUntil(t, hub, 2)
	target := fleetTarget(t, hub, f.remote.ID)
	require.Equal(t, core.TargetPending, target.State)
	require.NotNil(t, target.BusySince)
	require.NotNil(t, target.NextTry)
	plan := readPlan(t, hub)
	_, err = hub.fleet().Change(t.Context(), plan.FleetID, func(_ *gorm.DB, p *core.FleetPlan) error {
		past := time.Now().Add(-fleetBusyLimit - time.Second)
		p.Target(f.remote.ID).BusySince, p.Target(f.remote.ID).NextTry = &past, nil
		return nil
	})
	require.NoError(t, err)
	advanceUntil(t, hub, 2)
	target = fleetTarget(t, hub, f.remote.ID)
	require.Equal(t, core.TargetSkipped, target.State)
	require.Equal(t, "host.busy", target.Code)
	require.Zero(t, f.posts.Load())
}

func TestFleetAgentVersionAndIdentityDecisions(t *testing.T) {
	updateStamp(t)
	for name, c := range map[string]struct{ version, code, outcome string }{
		"current":  {version: "1.0.1", code: dto.FleetCurrent, outcome: "success"},
		"newer":    {version: "1.0.2", code: dto.FleetNewerThanHub, outcome: "partial"},
		"rotation": {version: "1.0.0", code: dto.FleetTargetChanged, outcome: "partial"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newUpdateTransport(t, false)
			configureUpdateFeed(t, f)
			f.currentContract.Store(true)
			f.hub.Stop()
			hub := restartHub(t, f, false)
			_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.NoError(t, err)
			// The agent changes after admission; the frozen target is never downgraded.
			f.version.Store(c.version)
			if name == "rotation" {
				require.NoError(t, f.db.Model(&f.remote).Update("credential_revision", f.remote.CredentialRevision+1).Error)
			}
			advanceUntil(t, hub, 2)
			plan := readPlan(t, hub)
			require.Equal(t, c.code, plan.Target(f.remote.ID).Code)
			require.Equal(t, c.outcome, plan.Outcome)
			require.Zero(t, f.posts.Load())
		})
	}
}

func TestFleetReservationFencesReplacedRegistration(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	f.hub.Stop()
	store := f.hub.remoteStore(f.remote.ID)
	op := core.Operation{OpID: core.FleetOpID("example-fleet", f.remote.ID), Role: "agent", EnvironmentID: f.remote.ID, CredentialRevision: f.remote.CredentialRevision, TargetRef: repository("agent") + ":1.0.1@sha256:" + testDigest, TargetVersion: "1.0.1", Deadline: time.Now().Add(core.OperationTimeout)}
	_, _, err := f.hub.reserveRun(t.Context(), store, op, "operator", "operator", &fleetLink{parentRunID: "example-parent", registeredAt: f.remote.CreatedAt.Add(-time.Hour)})
	require.True(t, core.HasCode(err, dto.FleetTargetChanged), err)
	current, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Nil(t, current, "a replaced registration was reserved")
}

func TestFleetCurrentHubNeedsAnAgentBelowTarget(t *testing.T) {
	updateStamp(t)
	for name, version := range map[string]string{"current": "1.0.1", "newer": "1.0.2", "old-contract": "1.0.0", "none": ""} {
		t.Run(name, func(t *testing.T) {
			f := newUpdateTransport(t, false)
			configureUpdateFeed(t, f)
			f.currentContract.Store(name != "old-contract")
			f.hub.Stop()
			if name == "none" {
				require.NoError(t, f.db.Delete(&f.remote).Error)
			} else {
				f.version.Store(version)
			}
			hub := restartHub(t, f, false)
			_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.True(t, core.HasCode(err, dto.FleetCurrent), err)
			plan, _, err := hub.fleet().Read(t.Context())
			require.NoError(t, err)
			require.Nil(t, plan, "a no-op combined update was admitted")
		})
	}
}
