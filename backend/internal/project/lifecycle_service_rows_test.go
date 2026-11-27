// Source: docklet-next/issues/655
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original proof that automatic recovery keeps the update run's service rows.

package project

import (
	"context"
	"errors"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestObservedStackRecoveryKeepsUpdateServiceRows(t *testing.T) {
	f, _ := observedStackFixture(t)
	intent := observedStackIntent(t, f, "manual", "version")
	var patch map[string]any
	reporter := projects.NewDeployReporter(func(string, bool) {}, func(p map[string]any) { patch = p }, time.Now)
	var observed []bool
	f.up = func(ctx context.Context, _ *compose.Project) error {
		attempt := projects.DeployReporterFromContext(ctx)
		observed = append(observed, attempt != nil)
		// A recovery deploy reporting through the run's reporter would overwrite this failure.
		attempt.On(api.Resource{ID: "Container example-stack-app-1", Text: api.StatusStarted, Status: api.Done})
		if f.deploys == 1 {
			attempt.On(api.Resource{ID: "Container example-stack-app-1", Text: "boom", Status: api.Error})
			return errors.New("synthetic deploy failure")
		}
		return nil
	}
	ctx := projects.WithDeployReporter(dto.WithObservedExecution(t.Context(), dto.ObservedExecution{Mode: "manual"}), reporter)
	_, _ = f.service.ExecuteImageLifecycle(ctx, f.project.ID, uuid.NewString(), f.registration, intent, user.Actor{ID: "operator", Username: "operator"})
	require.Equal(t, 2, f.deploys, "the failed update must enter automatic recovery")
	require.Equal(t, []bool{true, false}, observed, "only the update attempt reports service rows")
	reporter.Flush(true)
	rows := patch["deployServices"].(map[string]any)["services"].([]map[string]any)
	require.Len(t, rows, 1)
	require.Equal(t, "error", rows[0]["health"], "the failed attempt's evidence stays on the row")
}
