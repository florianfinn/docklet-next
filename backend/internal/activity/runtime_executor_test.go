// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: External-package real container service and runtime receipt proof.

package activity_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeExecutorContainerSevenBoundary(t *testing.T) {
	for _, tc := range []struct {
		operation activity.RuntimeOperation
		suffix    string
	}{
		{activity.RuntimeStartContainer, "/start"},
		{activity.RuntimeStopContainer, "/stop"},
		{activity.RuntimeRestartContainer, "/restart"},
	} {
		t.Run(string(tc.operation), func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			command := f.command(tc.operation, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			runID := f.run(command)
			var receiptCount, activityCount, finalCount int64
			require.NoError(t, f.db.Table("runtime_receipts").Where("run_id = ? AND kind = ?", runID, "receipt").Count(&receiptCount).Error)
			require.NoError(t, f.db.Table("activities").Where("id = ?", runID).Count(&activityCount).Error)
			require.NoError(t, f.db.Table("activity_messages").Where("activity_id = ? AND code = ?", runID, string(activitytypes.CodeActivitySuccess)).Count(&finalCount).Error)
			require.EqualValues(t, 1, receiptCount)
			require.EqualValues(t, 1, activityCount)
			require.EqualValues(t, 1, finalCount)
			var stored activity.Activity
			require.NoError(t, f.db.First(&stored, "id = ?", runID).Error)
			require.Equal(t, new(command.Actor.ID), stored.StartedByUserID)
			var attributedEvents int64
			require.NoError(t, f.db.Table("events").Where("user_id = ? AND resource_id = ?", command.Actor.ID, runtimeContainerID).Count(&attributedEvents).Error)
			require.EqualValues(t, 1, attributedEvents)
			effects := runtimeEffectEvents(f.http.snapshot())
			require.Len(t, effects, 1, f.debugEvents())
			require.Equal(t, http.MethodPost, effects[0].Method)
			require.True(t, strings.HasSuffix(effects[0].Path, tc.suffix), f.debugEvents())
			if tc.operation == activity.RuntimeStopContainer {
				require.Contains(t, effects[0].Query, "t=30")
			}
		})
	}
}

func TestRuntimeExecutorFreshContainerIdentityBeforeEffects(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	standalone := activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID}
	_, err := f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID[:12]})
	require.Error(t, err)
	_, err = f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "container", ID: "example-host~" + runtimeContainerID})
	require.Error(t, err)
	_, err = f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID, ParentProjectID: new("example-parent")})
	require.NoError(t, err) // The resolver returns evidence; admission rejects the supplied wrong parent.
	command := f.command(activity.RuntimeStartContainer, standalone)
	f.http.setLabels(map[string]string{"com.docker.compose.project": ""})
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.ErrorContains(t, err, "runtime.parent-unresolved")
	require.Empty(t, admitted.ActivityID)
	require.Empty(t, runtimeEffectEvents(f.http.snapshot()))

	f.http.setLabels(map[string]string{})
	var inspections atomic.Int32
	f.http.mu.Lock()
	f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/json") {
			return false
		}
		if inspections.Add(1) != 2 {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": runtimeContainerID, "Config": map[string]any{"Labels": map[string]string{"com.docker.compose.project": "changed_stack"}}})
		return true
	}
	f.http.mu.Unlock()
	admitted, err = f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	// The second live inspection is the executor preflight, independent of
	// scheduling between Admit's return and worker registration.
	_, waitErr := f.runtime.WaitLocal(t.Context(), activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID})
	require.NoError(t, waitErr) // A settled failed workflow is an authoritative snapshot.
	require.EqualValues(t, 2, inspections.Load())
	require.Empty(t, runtimeEffectEvents(f.http.snapshot()))
}

func TestRuntimeExecutorInspectFailurePreventsContainerEffect(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.http.mu.Lock()
	f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/json") {
			http.Error(w, "synthetic inspect outage", http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	f.http.mu.Unlock()
	command := f.command(activity.RuntimeStopContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
	result, err := f.runtime.Admit(t.Context(), command)
	require.Error(t, err)
	require.Empty(t, result.ActivityID)
	require.Empty(t, runtimeEffectEvents(f.http.snapshot()))
}

func TestRuntimeExecutorRequiredAdmissionAuditFailureHasNoEffect(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	require.NoError(t, f.db.Exec(`CREATE TRIGGER example_reject_start BEFORE INSERT ON activity_messages
		WHEN NEW.code = 'run.started' BEGIN SELECT RAISE(ABORT, 'synthetic start audit refusal'); END`).Error)
	command := f.command(activity.RuntimeStartContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
	result, err := f.runtime.Admit(t.Context(), command)
	require.ErrorContains(t, err, "synthetic start audit refusal")
	require.Empty(t, result.ActivityID)
	require.Empty(t, runtimeEffectEvents(f.http.snapshot()))
	var receipts, activities int64
	require.NoError(t, f.db.Table("runtime_receipts").Count(&receipts).Error)
	require.NoError(t, f.db.Table("activities").Count(&activities).Error)
	require.Zero(t, receipts)
	require.Zero(t, activities)
}

func TestRuntimeExecutorManagedParentAndOfflineKnownKey(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack"})
	parent := "example-project"
	command := f.command(activity.RuntimeStartContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID, ParentProjectID: &parent})
	resolved, err := f.adapter.ResolveRuntimeTarget(t.Context(), command.Target)
	require.NoError(t, err)
	require.True(t, resolved.ComposeManaged)
	require.Equal(t, &parent, resolved.Target.ParentProjectID)
	wrong := command
	wrong.Key = f.command(activity.RuntimeStartContainer, command.Target).Key
	wrong.Target.ParentProjectID = new("other-project")
	refusal, err := f.runtime.Admit(t.Context(), wrong)
	require.Error(t, err)
	require.Empty(t, refusal.ActivityID)
	require.Empty(t, runtimeEffectEvents(f.http.snapshot()))
	runID := f.run(command)
	before := f.http.snapshot()
	f.http.mu.Lock()
	f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/json") {
			http.Error(w, "synthetic offline target", http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	f.http.mu.Unlock()
	retry, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	require.Equal(t, runID, retry.ActivityID)
	require.Equal(t, before, f.http.snapshot(), "retained canonical key must not inspect or issue another effect")
}

func TestRuntimeExecutorChangedManagedParentAndRemovedTarget(t *testing.T) {
	for _, change := range []string{"parent", "removed"} {
		t.Run(change, func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project-a", "alpha", "name: first_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			f.project("example-project-b", "beta", "name: second_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			f.http.setLabels(map[string]string{"com.docker.compose.project": "first_stack"})
			var inspections atomic.Int32
			f.http.mu.Lock()
			f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/json") {
					return false
				}
				if inspections.Add(1) != 2 {
					return false
				}
				if change == "removed" {
					http.NotFound(w, r)
				} else {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": runtimeContainerID, "Config": map[string]any{"Labels": map[string]string{"com.docker.compose.project": "second_stack"}}})
				}
				return true
			}
			f.http.mu.Unlock()
			parent := "example-project-a"
			command := f.command(activity.RuntimeStopContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID, ParentProjectID: &parent})
			admitted, err := f.runtime.Admit(t.Context(), command)
			require.NoError(t, err)
			snapshot, err := f.runtime.WaitLocal(t.Context(), activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID})
			require.NoError(t, err)
			require.Equal(t, "failed", string(snapshot.Status))
			require.EqualValues(t, 2, inspections.Load())
			require.Empty(t, runtimeEffectEvents(f.http.snapshot()))
		})
	}
}
