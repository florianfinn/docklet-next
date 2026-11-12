// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original cross-host and cross-kind update progress identity coverage.

package updater

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

func TestUpdateOutcomeIdentityKeepsCoordinatorProgressDistinct(t *testing.T) {
	f := newPolicyFixture(t)
	require.NoError(t, f.db.Exec("INSERT INTO environments(id,name,api_url,enabled) VALUES(?,?,?,1)", "other", "Other", "https://agent.example.com").Error)
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "stack", ID: "app", Name: "app", Eligible: true}, {Kind: "container", ID: "app", Name: "app", ContainerID: "instance", Eligible: true}}, nil
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) { return f.check, nil }
	runtime := francistest.New(t)
	q := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, q.Register(runtime))
	francistest.Start(t, runtime)
	run, err := q.Submit(t.Context(), st.Request{JobID: "update-check", Trigger: "scheduled"})
	require.NoError(t, err)
	run.Status, run.Owner = st.Running, "test-owner"
	require.NoError(t, q.UpdateRun(t.Context(), run, func(current *st.Run) error { current.Status, current.Owner = run.Status, run.Owner; return nil }))
	progressCount := 0
	ctx := jobcontext.WithExecution(t.Context(), run, func(target st.TargetOutcome) error {
		if err := q.Progress(t.Context(), run, target); err != nil {
			return err
		}
		progressCount++
		persisted, err := q.Get(t.Context(), "0", "update-check", run.ID)
		require.NoError(t, err)
		require.Len(t, persisted.Outcome.Targets, progressCount)
		return nil
	})
	out, err := f.s.RunUpdateCheck(ctx)
	require.NoError(t, err)
	require.Len(t, out.Targets, 4)
	require.Equal(t, 4, progressCount)
	seen := map[string]bool{}
	for _, target := range out.Targets {
		require.Equal(t, target.EnvironmentID+"~"+target.TargetKind+"~app", target.ID)
		require.Equal(t, "app", target.TargetID)
		require.Contains(t, []string{"0", "other"}, target.EnvironmentID)
		require.Contains(t, []string{"stack", "container"}, target.TargetKind)
		require.False(t, seen[target.ID])
		seen[target.ID] = true
	}
	persisted, err := q.Get(t.Context(), "0", "update-check", run.ID)
	require.NoError(t, err)
	require.Equal(t, out.Targets, persisted.Outcome.Targets)
	row := f.save(t, "app", "scheduled")
	window, err := f.s.RunUpdateWindow(jobcontext.WithExecution(t.Context(), st.Run{ID: "window", AttemptCount: 1}, nil), row.ID)
	require.NoError(t, err)
	require.Len(t, window.Targets, 1)
	require.Equal(t, "0~stack~app", window.Targets[0].ID)
	require.Equal(t, "0", window.Targets[0].EnvironmentID)
	require.Equal(t, "stack", window.Targets[0].TargetKind)
	require.Equal(t, "app", window.Targets[0].TargetID)
}
