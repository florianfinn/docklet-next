// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original scheduler restart suppression from the inspected replacement label and local receipts.

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	dto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func TestAutoHealReplacementReceiptHold(t *testing.T) {
	j, db := autoHealFixture(t)
	run := uuid.NewString()
	now := time.Now()
	row := activity.RuntimeReceipt{RunID: run, Kind: "receipt", EnvironmentID: "0", StoreGeneration: "store", ExecutorGeneration: "store", ManagerID: "manager", RegistrationID: "registration", RequestDigest: "digest", FinalWriteAt: now}
	require.NoError(t, db.Create(&row).Error)
	inspected := healInspect("container")
	inspected.Config.Labels = map[string]string{"io.docklet.run": run}
	j.inspectContainer = func(context.Context, *client.Client, string) (container.InspectResponse, error) {
		return inspected, nil
	}
	runtime := activity.NewRuntimeService(activity.NewActivityService(db, nil), activity.RuntimeResolverFunc(func(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
		return activity.RuntimeResolvedTarget{Target: target}, nil
	}))
	// Use the production Busy callback contract: the exact fresh scheduler inspect supplies labels.
	j.busy = func(ctx context.Context, v container.InspectResponse) (bool, error) {
		return runtime.HeldForContainer(ctx, v.ID, "", v.Config.Labels)
	}
	restarts := 0
	j.restartContainer = func(context.Context, *client.Client, string) error { restarts++; return nil }
	_, err := j.Run(t.Context())
	require.NoError(t, err)
	require.Zero(t, restarts, "held replacement must not be restarted by Auto Heal")
	require.NoError(t, db.Model(&row).Where("run_id=?", run).UpdateColumn("settled_at", now).Error)
	_, err = j.Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, restarts, "settled replacement must restart exactly once")
	var count int64
	require.NoError(t, db.Model(&activity.Activity{}).Where("status=?", dto.StatusSuccess).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
