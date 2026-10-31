// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shell activity end paths through the persisted activity service.

package activity

import (
	"context"
	"net/url"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func openShellActivity(t *testing.T, svc *ActivityService, registry *containershell.Registry) (context.Context, *containershell.Handle, string) {
	t.Helper()
	ctx, handle, err := registry.Accept(t.Context(), url.Values{}, &user.Actor{ID: "example-user"}, "example-host", "example-container")
	require.NoError(t, err)
	var rows []Activity
	require.NoError(t, svc.db.Where("status = ?", activity.StatusRunning).Find(&rows).Error)
	require.Len(t, rows, 1)
	return ctx, handle, rows[0].ID
}

// A shell activity left open by a dead hub process is closed by the next startup repair.
func TestShellActivityOpenAtHubDeathIsRepairedAtStartup(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	previous := NewActivityService(db, nil)
	_, _, id := openShellActivity(t, previous, containershell.New(previous))
	next := matrixNewHub(t, db)
	count, err := next.RepairStartup(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	row := matrixRow(t, db.DB, id)
	require.Equal(t, activity.StatusFailed, row.Status)
	require.NotNil(t, row.EndedAt)
}

// A user cancel and the restore drain both end the tracked shell; its single
// completion is recorded as cancelled.
func TestShellActivityCancelPathsCompleteOnce(t *testing.T) {
	for _, path := range []string{"request-cancel", "restore-drain"} {
		t.Run(path, func(t *testing.T) {
			db := setupActivityServiceTestDBInternal(t)
			svc := NewActivityService(db, nil)
			registry := containershell.New(svc)
			ctx, handle, id := openShellActivity(t, svc, registry)
			// The terminal session releases its handle once its context ends.
			released := make(chan struct{})
			go func() { <-ctx.Done(); handle.Release(); close(released) }()
			if path == "request-cancel" {
				require.True(t, svc.RequestCancel(id))
			} else {
				require.NoError(t, svc.CancelAndDrain(t.Context(), ""))
			}
			<-released
			handle.Release()
			row := matrixRow(t, db.DB, id)
			require.Equal(t, activity.StatusCancelled, row.Status)
			require.NotNil(t, row.EndedAt)
			require.Zero(t, registry.ActiveCount())
		})
	}
}
