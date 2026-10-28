// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original activity-result collection with admitted configuration and identity checks.

package backup

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	activitydto "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/stretchr/testify/require"
)

func TestTargetCheckResultCollectedOnlyForAdmittedIdentity(t *testing.T) {
	s := targetAdmissionService(t)
	require.NoError(t, s.deps.DB.Create(&backuptarget.Target{ID: "target", Name: "Archive", ConfigVersion: 7, CreatedAt: time.Now()}).Error)
	require.NoError(t, s.recordRemoteCheck(t.Context(), "target", "host", 12, "accepted"))
	result := dto.TargetCheck{HostID: "host", SetVersion: 12, ConfigVersion: 7, ActivityID: "accepted", OK: true, At: time.Now().UTC(), SnapshotCount: new(int64(5))}
	activityID := "other-generation"
	calls := 0
	proxy := func(_ context.Context, host, method, path string, _ []byte, out any) error {
		calls++
		require.Equal(t, "host", host)
		require.Equal(t, http.MethodGet, method)
		require.Equal(t, "/api/environments/0/activities/accepted?limit=1", path)
		*out.(*base.ApiResponse[activitydto.Detail]) = base.ApiResponse[activitydto.Detail]{Success: true, Data: activitydto.Detail{Activity: activitydto.Activity{ID: activityID, Status: "success", Metadata: map[string]any{"backupTargetId": "target", "backupTargetCheck": result}}}}
		return nil
	}
	require.NoError(t, s.refreshTargetChecks(t.Context(), proxy))
	var saved backuptarget.Check
	require.NoError(t, s.deps.DB.First(&saved).Error)
	require.False(t, saved.OK, "an unrelated activity cannot complete the admitted test")
	activityID = "accepted"
	require.NoError(t, s.deps.DB.Model(&backuptarget.Target{}).Where("id = ?", "target").Update("config_version", 8).Error)
	s.targets.ConfigureCheckRefresh(func(ctx context.Context) error { return s.refreshTargetChecks(ctx, proxy) })
	projected, err := s.targets.Get(t.Context(), "target")
	require.NoError(t, err)
	require.True(t, projected.Checks[0].OK)
	require.True(t, projected.Checks[0].EarlierSettings)
	require.EqualValues(t, 7, projected.Checks[0].ConfigVersion)
	require.EqualValues(t, 5, *projected.Checks[0].SnapshotCount)
	_, err = s.targets.Get(t.Context(), "target")
	require.NoError(t, err)
	require.Equal(t, 2, calls, "terminal results must not be polled again")
}
