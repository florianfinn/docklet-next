// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original boundary tests of the combined update (review S1 r1).

package upgrade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
)

func TestFleetStartReservesTheHubFromAdmissionAlone(t *testing.T) {
	db := updateDB(t)
	hub := newFleetHub(t, db, "1.0.0", "1.0.1")
	addHosts(t, db, "example-a")
	require.NoError(t, hub.Start(t.Context()))
	t.Cleanup(hub.Stop)
	started, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return readPlan(t, hub.Service).Hub.State == core.TargetReserved }, 10*time.Second, 50*time.Millisecond)
	op, err := hub.store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, started.HubOpID, op.OpID)
	require.Equal(t, 1, hub.reserves)
}

func TestFleetSkippedHubGetsOneChild(t *testing.T) {
	db := updateDB(t)
	hub := newFleetHub(t, db, "1.0.0", "1.0.1")
	addHosts(t, db, "example-a")
	started, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.NoError(t, err)
	hub.localCheck = func(context.Context) (dto.SelfUpdateCheck, error) {
		return dto.SelfUpdateCheck{Version: "1.0.0", Eligibility: dto.SelfUpdateEligibility{State: "not-eligible", Code: "update.self-custom-image"}}, nil
	}
	advanceUntil(t, hub.Service, 2)
	plan := readPlan(t, hub.Service)
	require.Equal(t, "failed", plan.Outcome)
	require.Equal(t, "update.self-custom-image", plan.Hub.Code)
	var children []activity.Activity
	require.NoError(t, db.Where("batch_id=?", started.RunID).Find(&children).Error)
	require.Len(t, children, 2)
	var hubChild activity.Activity
	require.NoError(t, db.First(&hubChild, "id=?", plan.Hub.RunID).Error)
	require.Equal(t, activityDTO.TypeHubUpdate, hubChild.Type)
	require.Equal(t, "update.self-custom-image", hubChild.Metadata["reason"])
}

func TestFleetPlanStateIsValidatedBeforeEffects(t *testing.T) {
	for name, change := range map[string]func(*core.FleetPlan){
		"missing-phase":       func(p *core.FleetPlan) { p.Phase = "" },
		"agents-before-hub":   func(p *core.FleetPlan) { p.Phase = "agents" },
		"unknown-agent-state": func(p *core.FleetPlan) { p.Agents[0].State = "waiting" },
		"duplicate-agent":     func(p *core.FleetPlan) { p.Agents = append(p.Agents, p.Agents[0]) },
		"agent-reserved-in-hub-phase": func(p *core.FleetPlan) {
			p.Agents[0].State = core.TargetReserved
		},
	} {
		t.Run(name, func(t *testing.T) {
			db := updateDB(t)
			hub := newFleetHub(t, db, "1.0.0", "1.0.1")
			addHosts(t, db, "example-a")
			started, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.NoError(t, err)
			plan := readPlan(t, hub.Service)
			change(&plan)
			raw := mustJSON(t, plan)
			require.NoError(t, db.Model(&kv.KVEntry{}).Where("key=?", core.FleetKey).Update("value", raw).Error)
			done, err := hub.advanceFleet(t.Context())
			require.NoError(t, err)
			require.True(t, done)
			require.Zero(t, hub.reserves)
			var parent activity.Activity
			require.NoError(t, db.First(&parent, "id=?", started.RunID).Error)
			require.Equal(t, activityDTO.StatusFailed, parent.Status)
			require.Equal(t, dto.FleetUnreadable, parent.Metadata["reason"])
		})
	}
}

func TestFleetKeepsAtMostThreeAgentsInFlight(t *testing.T) {
	db := updateDB(t)
	hub := newFleetHub(t, db, "1.0.1", "1.0.1")
	addHosts(t, db, "example-a", "example-b", "example-c", "example-d")
	hub.SetEnvironments(environment.NewEnvironmentService(db, http.DefaultClient, nil, nil, nil, nil))
	plan := seedAgentPlan(t, hub.Service, "example-a", "example-b", "example-c", "example-d")
	// Three planned operations are already dispatched and unsettled.
	for _, agent := range plan.Agents[:3] {
		op := core.Operation{OpID: agent.OpID, Role: "agent", EnvironmentID: agent.EnvironmentID, CredentialRevision: 1, TargetRef: repository("agent") + ":1.0.1@sha256:" + testDigest, TargetVersion: "1.0.1", Deadline: time.Now().Add(core.OperationTimeout)}
		_, _, err := hub.remoteStore(agent.EnvironmentID).Reserve(t.Context(), op, nil)
		require.NoError(t, err)
		require.NoError(t, hub.changeTarget(t.Context(), plan, agent.EnvironmentID, func(target *core.FleetTarget) { target.State = core.TargetReserved }))
	}
	advanceUntil(t, hub.Service, 1)
	require.Equal(t, core.TargetPending, fleetTarget(t, hub.Service, "example-d").State, "a fourth agent started")
}

// seedAgentPlan admits a plan already in the agent phase without remote reads.
func seedAgentPlan(t *testing.T, s *Service, ids ...string) core.FleetPlan {
	t.Helper()
	plan := core.FleetPlan{SchemaVersion: core.FleetSchemaVersion, FleetID: "example-fleet", RunID: "example-parent", Phase: "agents", TargetVersion: "1.0.1", StartedBy: "operator", StartedByName: "operator", CreatedAt: time.Now().UTC()}
	plan.Hub = core.FleetTarget{EnvironmentID: "0", OpID: core.FleetOpID(plan.FleetID, "0"), State: core.TargetSkipped, Outcome: "current"}
	for _, id := range ids {
		var env environment.Environment
		require.NoError(t, s.db.First(&env, "id=?", id).Error)
		plan.Agents = append(plan.Agents, core.FleetTarget{EnvironmentID: id, CredentialRevision: env.CredentialRevision, RegisteredAt: env.CreatedAt, OpID: core.FleetOpID(plan.FleetID, id), State: core.TargetPending})
	}
	require.NoError(t, s.db.Create(&activity.Activity{BaseModel: database.BaseModel{ID: plan.RunID}, EnvironmentID: "0", Type: activityDTO.TypeFleetUpdate, Status: activityDTO.StatusRunning, StartedAt: time.Now()}).Error)
	require.NoError(t, s.fleet().Admit(t.Context(), plan, nil))
	return plan
}

func TestFleetOfflineAndFailingAgentsAreSkipped(t *testing.T) {
	db := updateDB(t)
	hub := newFleetHub(t, db, "1.0.1", "1.0.1")
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	t.Cleanup(failing.Close)
	for id, url := range map[string]string{"example-offline": closed.URL, "example-failing": failing.URL} {
		require.NoError(t, db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: id}, Name: id, ApiUrl: url, Enabled: true, AccessToken: new("example"), CredentialRevision: 1}).Error)
	}
	hub.SetEnvironments(environment.NewEnvironmentService(db, http.DefaultClient, nil, nil, nil, nil))
	seedAgentPlan(t, hub.Service, "example-offline", "example-failing")
	advanceUntil(t, hub.Service, 1)
	plan := readPlan(t, hub.Service)
	for _, id := range []string{"example-offline", "example-failing"} {
		require.Equal(t, core.TargetSkipped, plan.Target(id).State, id)
		require.Equal(t, "agent.unavailable", plan.Target(id).Code, id)
	}
	advanceUntil(t, hub.Service, 1)
	require.Equal(t, "settled", readPlan(t, hub.Service).Phase)
}

func TestFleetOldContractAgentIsSkippedDirectAndEdge(t *testing.T) {
	updateStamp(t)
	for _, edge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[edge], func(t *testing.T) {
			f := newUpdateTransport(t, edge)
			configureUpdateFeed(t, f)
			f.hub.Stop()
			hub := restartHub(t, f, false)
			seedAgentPlan(t, hub, f.remote.ID)
			advanceUntil(t, hub, 2)
			target := fleetTarget(t, hub, f.remote.ID)
			require.Equal(t, core.TargetSkipped, target.State)
			require.Equal(t, dto.FleetAgentTooOld, target.Code)
			require.Zero(t, f.posts.Load())
		})
	}
}

func TestFleetPermissionIsCheckedAtEachEffect(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	configureUpdateFeed(t, f)
	f.currentContract.Store(true)
	f.hub.Stop()
	hub := restartHub(t, f, false)
	seedAgentPlan(t, hub, f.remote.ID)
	advanceUntil(t, hub, 1) // stores the target reference
	// The permission disappears while the agent check of the reserving round is in flight.
	var allowed atomic.Bool
	allowed.Store(true)
	hub.allowed = func(context.Context, string) (bool, error) { return allowed.Load(), nil }
	f.holdCheck.Store(true)
	round := make(chan error, 1)
	go func() { _, err := hub.advanceFleet(t.Context()); round <- err }()
	<-f.checkEntered
	allowed.Store(false)
	close(f.checkRelease)
	require.NoError(t, <-round)
	target := fleetTarget(t, hub, f.remote.ID)
	require.Equal(t, dto.FleetPermission, target.Code)
	require.Zero(t, f.posts.Load())
	current, err := hub.remoteStore(f.remote.ID).Read(t.Context())
	require.NoError(t, err)
	require.Nil(t, current, "an operation was reserved after the permission was revoked")
}

func TestFleetRegistrationReplacedDuringCheckIsNotReserved(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	configureUpdateFeed(t, f)
	f.currentContract.Store(true)
	f.hub.Stop()
	hub := restartHub(t, f, false)
	seedAgentPlan(t, hub, f.remote.ID)
	advanceUntil(t, hub, 1)
	f.holdCheck.Store(true)
	round := make(chan error, 1)
	go func() { _, err := hub.advanceFleet(t.Context()); round <- err }()
	<-f.checkEntered
	// Delete and re-register the same host id with an unchanged revision.
	replacement := f.remote
	replacement.CreatedAt = f.remote.CreatedAt.Add(time.Minute)
	require.NoError(t, f.db.Unscoped().Delete(&f.remote).Error)
	require.NoError(t, f.db.Create(&replacement).Error)
	close(f.checkRelease)
	require.NoError(t, <-round)
	require.Equal(t, dto.FleetTargetChanged, fleetTarget(t, hub, f.remote.ID).Code)
	require.Zero(t, f.posts.Load())
}

func TestFleetMarkerWithoutRequestSettlesUnknown(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	configureUpdateFeed(t, f)
	f.currentContract.Store(true)
	f.hub.Stop()
	owner := restartHub(t, f, false)
	plan := seedAgentPlan(t, owner, f.remote.ID)
	agent := plan.Agents[0]
	// The marker was written, the process died before the request, and the deadline passed.
	op := core.Operation{OpID: agent.OpID, Role: "agent", EnvironmentID: f.remote.ID, CredentialRevision: agent.CredentialRevision, TargetRef: repository("agent") + ":1.0.1@sha256:" + testDigest, TargetVersion: "1.0.1", Deadline: time.Now().Add(-time.Second)}
	store := owner.remoteStore(f.remote.ID)
	op, _, err := owner.reserveRun(t.Context(), store, op, "operator", "operator", &fleetLink{parentRunID: plan.RunID, registeredAt: agent.RegisteredAt})
	require.NoError(t, err)
	acquired, err := store.AcquireDispatch(t.Context(), op.OpID, nil)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, owner.changeTarget(t.Context(), plan, f.remote.ID, func(target *core.FleetTarget) { target.State, target.RunID = core.TargetReserved, op.RunID }))
	restarted := restartHub(t, f, true)
	require.Eventually(t, func() bool { return readPlan(t, restarted).Phase == "settled" }, 15*time.Second, 50*time.Millisecond)
	final := readPlan(t, restarted)
	require.Equal(t, "unknown", final.Target(f.remote.ID).Outcome)
	require.Equal(t, "failed", final.Outcome)
	require.Zero(t, f.posts.Load())
	var child activity.Activity
	require.NoError(t, f.db.First(&child, "id=?", op.RunID).Error)
	require.Equal(t, activityDTO.StatusFailed, child.Status)
	var children int64
	require.NoError(t, f.db.Model(&activity.Activity{}).Where("batch_id=?", plan.RunID).Count(&children).Error)
	require.EqualValues(t, 1, children)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}
