// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Runtime activation preserves the existing merge of ordinary activity summaries.

package activity_test

import (
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationOrdinaryActivityMergeIsUnchanged(t *testing.T) {
	f := activationApp(t, false, "")
	const env = "example-ordinary"
	require.NoError(t, f.services.db.Create(&environment.Environment{ID: env, Name: "Example ordinary", ApiUrl: "https://agent.example.com", Enabled: true}).Error)
	started, err := f.services.activity.StartActivity(t.Context(), activity.StartActivityRequest{EnvironmentID: env, Type: activitytypes.TypeImagePull, ResourceType: new("image"), ResourceID: new("example-image")})
	require.NoError(t, err)
	observed := *started
	observed.EnvironmentID = "0"
	observed.Status = activitytypes.StatusSuccess
	observed.MessageCode = string(activitytypes.CodeActivitySuccess)
	observed.EndedAt = new(time.Now().UTC())
	var receipts int64
	require.NoError(t, f.services.db.Model(&activity.RuntimeReceipt{}).Count(&receipts).Error)
	require.Zero(t, receipts, "ordinary work has no runtime receipt authority")
	rows, page, err := f.services.activity.ListRemoteActivities(t.Context(), env, []activitytypes.Activity{observed}, 1, pagination.QueryParams{Limit: 50})
	require.NoError(t, err)
	require.Len(t, rows, 2, "runtime activation must preserve the existing ordinary merge")
	require.EqualValues(t, 2, page.TotalItems)
	require.Equal(t, started.ID, rows[0].ID)
	require.Equal(t, started.ID, rows[1].ID)
	require.Equal(t, activitytypes.StatusRunning, rows[0].Status)
	require.Equal(t, activitytypes.StatusSuccess, rows[1].Status)
	require.Nil(t, rows[0].RuntimeKnowledge)
	require.Nil(t, rows[1].RuntimeKnowledge)
}
