// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original tests for the combined update plan and hub phase.

package upgrade

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fleetHub struct {
	*Service
	reserves int
	mu       sync.Mutex
}

// newFleetHub is a hub without an Engine: the local check and the local
// reservation use the real store, admission and activity writes.
func newFleetHub(t *testing.T, db *database.DB, running, candidate string) *fleetHub {
	t.Helper()
	h := &fleetHub{Service: NewService(db, nil, nil, &config.Config{})}
	h.runningVersion = func() string { return running }
	h.allowed = func(context.Context, string) (bool, error) { return true, nil }
	h.resolve = func(_ context.Context, repo, version string, _ http.RoundTripper) (string, error) {
		return repo + ":" + version + "@sha256:" + testDigest, nil
	}
	h.localCheck = func(context.Context) (dto.SelfUpdateCheck, error) {
		out := dto.SelfUpdateCheck{Version: running, Eligibility: dto.SelfUpdateEligibility{State: "current"}}
		if candidate != running {
			out.Eligibility = dto.SelfUpdateEligibility{State: "available", Candidate: &dto.SelfUpdateCandidate{Version: candidate}}
		}
		return out, nil
	}
	h.reserveHub = func(ctx context.Context, envelope dto.SelfUpdateEnvelope, userID, username string, link *fleetLink) (core.Operation, error) {
		h.mu.Lock()
		h.reserves++
		h.mu.Unlock()
		op := core.Operation{OpID: envelope.OpID, Role: "hub", EnvironmentID: "0", TargetRef: envelope.TargetReference, TargetVersion: envelope.TargetVersion, FromVersion: running, Deadline: time.Now().UTC().Add(core.OperationTimeout)}
		op, _, err := h.reserveRun(ctx, h.store, op, userID, username, link)
		return op, err
	}
	return h
}

func addHosts(t *testing.T, db *database.DB, ids ...string) {
	t.Helper()
	for _, id := range ids {
		require.NoError(t, db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: id}, Name: id, ApiUrl: "http://192.0.2.1:9", Enabled: true, AccessToken: new("example"), CredentialRevision: 1}).Error)
	}
}

func readPlan(t *testing.T, s *Service) core.FleetPlan {
	t.Helper()
	plan, _, err := s.fleet().Read(t.Context())
	require.NoError(t, err)
	require.NotNil(t, plan)
	return *plan
}

func advanceUntil(t *testing.T, s *Service, rounds int) {
	t.Helper()
	for range rounds {
		_, err := s.advanceFleet(t.Context())
		require.NoError(t, err)
	}
}

func TestFleetAdmissionKeepsOnePlan(t *testing.T) {
	db := updateDB(t)
	hub := newFleetHub(t, db, "1.0.0", "1.0.1")
	addHosts(t, db, "example-a")
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			results <- err
		})
	}
	wg.Wait()
	close(results)
	admitted := 0
	for err := range results {
		if err == nil {
			admitted++
			continue
		}
		require.True(t, core.HasCode(err, dto.FleetBusy), err)
	}
	require.Equal(t, 1, admitted)
	var parents int64
	require.NoError(t, db.Model(&activity.Activity{}).Where("type=?", activityDTO.TypeFleetUpdate).Count(&parents).Error)
	require.EqualValues(t, 1, parents)
	plan := readPlan(t, hub.Service)
	require.Equal(t, "hub", plan.Phase)
	require.Len(t, plan.Agents, 1)
	require.Equal(t, core.FleetOpID(plan.FleetID, "example-a"), plan.Agents[0].OpID)
	// The confirmed version must be the one admitted.
	_, err := newFleetHub(t, updateDB(t), "1.0.1", "1.0.2").StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.True(t, core.HasCode(err, "update.invalid-target"), err)
}

func TestFleetHubReservesPlannedOperationOnceAcrossRestart(t *testing.T) {
	db := updateDB(t)
	hub := newFleetHub(t, db, "1.0.0", "1.0.1")
	addHosts(t, db, "example-a")
	started, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.NoError(t, err)
	require.Equal(t, core.FleetOpID(started.FleetID, "0"), started.HubOpID)
	// Crash after the reservation, before the plan recorded it: the next process joins.
	hub.advanceFleet(t.Context()) //nolint:errcheck // the next read asserts the stored reference
	plan := readPlan(t, hub.Service)
	require.NotEmpty(t, plan.Hub.TargetRef)
	_, err = hub.reserveHub(t.Context(), dto.SelfUpdateEnvelope{OpID: plan.Hub.OpID, TargetVersion: "1.0.1", TargetReference: plan.Hub.TargetRef}, "operator", "operator", &fleetLink{parentRunID: plan.RunID})
	require.NoError(t, err)
	restarted := newFleetHub(t, db, "1.0.0", "1.0.1")
	advanceUntil(t, restarted.Service, 3)
	require.Zero(t, restarted.reserves, "a planned reserved operation was reserved again")
	plan = readPlan(t, restarted.Service)
	require.Equal(t, core.TargetReserved, plan.Hub.State)
	var runs []activity.Activity
	require.NoError(t, db.Where("type=?", activityDTO.TypeHubUpdate).Find(&runs).Error)
	require.Len(t, runs, 1)
	require.Equal(t, plan.Hub.RunID, runs[0].ID)
	require.Equal(t, plan.RunID, *runs[0].BatchID)
	op, err := restarted.store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, plan.Hub.OpID, op.OpID)
	// The agent phase waits for the hub settlement.
	require.Equal(t, "hub", plan.Phase)
	require.Equal(t, core.TargetPending, plan.Agents[0].State)
}

func TestFleetParentExemptsOnlyItself(t *testing.T) {
	db := updateDB(t)
	hub := newFleetHub(t, db, "1.0.0", "1.0.1")
	_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.NoError(t, err)
	other := activity.Activity{BaseModel: database.BaseModel{ID: uuid.NewString()}, EnvironmentID: "0", Type: activityDTO.TypeJobRun, Status: activityDTO.StatusRunning, StartedAt: time.Now()}
	require.NoError(t, db.Create(&other).Error)
	advanceUntil(t, hub.Service, 1) // stores the target reference
	_, err = hub.advanceFleet(t.Context())
	require.True(t, hostBusy(err), err)
	plan := readPlan(t, hub.Service)
	require.Equal(t, core.TargetPending, plan.Hub.State, "another running activity must still block the hub")
	require.NoError(t, db.Model(&other).Update("status", activityDTO.StatusSuccess).Error)
	advanceUntil(t, hub.Service, 1)
	require.Equal(t, core.TargetReserved, readPlan(t, hub.Service).Hub.State, "the parent blocked its own hub child")
}

func TestFleetGateReleasesAgentsOnlyOnHealthyTarget(t *testing.T) {
	cases := []struct {
		name, outcome, running string
		wrongOp, released      bool
	}{
		{name: "healthy", outcome: "healthy", running: "1.0.1", released: true},
		{name: "healthy-old-process", outcome: "healthy", running: "1.0.0"},
		{name: "failed-health", outcome: "failed-health", running: "1.0.1"},
		{name: "failed-pre-effect", outcome: "failed-pre-effect", running: "1.0.0"},
		{name: "unknown", outcome: "unknown", running: "1.0.1"},
		{name: "other-operation", outcome: "healthy", running: "1.0.1", wrongOp: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := updateDB(t)
			hub := newFleetHub(t, db, "1.0.0", "1.0.1")
			addHosts(t, db, "example-a", "example-b")
			_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.NoError(t, err)
			advanceUntil(t, hub.Service, 2)
			plan := readPlan(t, hub.Service)
			require.Equal(t, core.TargetReserved, plan.Hub.State)
			opID := plan.Hub.OpID
			if c.wrongOp {
				// Another operation replaced the slot after this one settled.
				_, err = hub.store.Change(t.Context(), opID, func(_ *gorm.DB, op *core.Operation) error {
					op.Phase, op.Outcome = "settled", "failed-health"
					return nil
				})
				require.NoError(t, err)
				_, _, err = hub.store.Reserve(t.Context(), core.Operation{OpID: "replacement", Role: "hub", EnvironmentID: "0", TargetRef: plan.Hub.TargetRef, TargetVersion: "1.0.1", Deadline: time.Now().Add(time.Minute)}, nil)
				require.NoError(t, err)
				opID = "replacement"
			}
			_, err = hub.store.Change(t.Context(), opID, func(_ *gorm.DB, op *core.Operation) error {
				op.Phase, op.Outcome = "settled", c.outcome
				return nil
			})
			require.NoError(t, err)
			hub.runningVersion = func() string { return c.running }
			advanceUntil(t, hub.Service, 1)
			plan = readPlan(t, hub.Service)
			if c.released {
				require.Equal(t, "agents", plan.Phase)
				require.Equal(t, "healthy", plan.Hub.Outcome)
				return
			}
			require.Equal(t, "settled", plan.Phase)
			require.Equal(t, "failed", plan.Outcome)
			for _, agent := range plan.Agents {
				require.Equal(t, core.TargetSkipped, agent.State)
				require.Equal(t, dto.FleetHubFailed, agent.Code)
				var child activity.Activity
				require.NoError(t, db.First(&child, "id=?", agent.RunID).Error)
				require.Equal(t, activityDTO.StatusCancelled, child.Status)
				require.Equal(t, dto.FleetHubFailed, child.Metadata["reason"])
				require.Equal(t, plan.RunID, *child.BatchID)
			}
			var remote int64
			require.NoError(t, db.Model(&kv.KVEntry{}).Where("key LIKE ?", "selfupdate.remote.%").Count(&remote).Error)
			require.Zero(t, remote, "a failed hub touched an agent")
			var parent activity.Activity
			require.NoError(t, db.First(&parent, "id=?", plan.RunID).Error)
			require.Equal(t, activityDTO.StatusFailed, parent.Status)
			require.Equal(t, dto.FleetHubFailed, parent.Metadata["reason"])
		})
	}
}

func TestFleetStopsNewEffectsWithoutPermission(t *testing.T) {
	db := updateDB(t)
	hub := newFleetHub(t, db, "1.0.0", "1.0.1")
	addHosts(t, db, "example-a")
	_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.NoError(t, err)
	hub.allowed = func(context.Context, string) (bool, error) { return false, nil }
	advanceUntil(t, hub.Service, 1)
	plan := readPlan(t, hub.Service)
	require.Zero(t, hub.reserves)
	require.Equal(t, dto.FleetPermission, plan.Hub.Code)
	require.Equal(t, "failed", plan.Outcome)
}

func TestFleetUnreadablePlanFailsClosed(t *testing.T) {
	for name, raw := range map[string]string{"future": `{"schemaVersion":2,"fleetId":"a","runId":"%s","phase":"hub"}`, "corrupt": `{"runId":"%s","phase":"hub",`} {
		t.Run(name, func(t *testing.T) {
			db := updateDB(t)
			hub := newFleetHub(t, db, "1.0.0", "1.0.1")
			parent := activity.Activity{BaseModel: database.BaseModel{ID: "parent-run"}, EnvironmentID: "0", Type: activityDTO.TypeFleetUpdate, Status: activityDTO.StatusRunning, StartedAt: time.Now()}
			require.NoError(t, db.Create(&parent).Error)
			require.NoError(t, db.Create(&kv.KVEntry{Key: core.FleetKey, Value: fmtRun(raw, parent.ID)}).Error)
			_, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.True(t, core.HasCode(err, dto.FleetBusy), "admitted over an unreadable plan")
			done, err := hub.advanceFleet(t.Context())
			require.NoError(t, err)
			require.True(t, done)
			if name == "future" {
				require.NoError(t, db.First(&parent, "id=?", parent.ID).Error)
				require.Equal(t, activityDTO.StatusFailed, parent.Status)
				require.Equal(t, dto.FleetUnreadable, parent.Metadata["reason"])
			}
			_, err = hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
			require.NoError(t, err)
		})
	}
}

func fmtRun(raw, id string) string {
	out := []byte{}
	for i := 0; i < len(raw); i++ {
		if raw[i] == '%' && i+1 < len(raw) && raw[i+1] == 's' {
			out = append(out, id...)
			i++
			continue
		}
		out = append(out, raw[i])
	}
	return string(out)
}

func TestFleetParentSurvivesRepairWhileActive(t *testing.T) {
	db := updateDB(t)
	hub := newFleetHub(t, db, "1.0.0", "1.0.1")
	addHosts(t, db, "example-a")
	started, err := hub.StartFleet(t.Context(), dto.FleetUpdateRequest{TargetVersion: "1.0.1"}, "operator", "operator")
	require.NoError(t, err)
	repaired, err := activity.NewActivityService(db, nil).RepairStartup(t.Context())
	require.NoError(t, err)
	require.Zero(t, repaired)
	var parent activity.Activity
	require.NoError(t, db.First(&parent, "id=?", started.RunID).Error)
	require.Equal(t, activityDTO.StatusRunning, parent.Status)
}

func TestFleetRoutesAreHubOnly(t *testing.T) {
	_, agentAPI := humatest.New(t, huma.DefaultConfig("Agent", "1.0.0"))
	RegisterAgentRoutes(agentAPI, &Handler{})
	require.Nil(t, agentAPI.OpenAPI().Paths["/system/upgrade/all"])
	_, hubAPI := humatest.New(t, huma.DefaultConfig("Hub", "1.0.0"))
	RegisterRoutes(hubAPI, &Handler{})
	path := hubAPI.OpenAPI().Paths["/system/upgrade/all"]
	require.NotNil(t, path)
	require.NotNil(t, path.Post)
	require.NotNil(t, path.Get)
}
