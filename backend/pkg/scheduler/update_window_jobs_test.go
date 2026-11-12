// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original real window registration/removal and startup rebuild proof.

package scheduler

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

func TestRefreshUpdateWindowsCreatesRemovesAndRebuildsJobs(t *testing.T) {
	db, e := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "windows.db")), database.MigrationOptions{})
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	cfg, e := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, cfg.Stop(context.WithoutCancel(t.Context()))) })
	require.NoError(t, db.Exec("INSERT INTO environments(id,name,api_url,enabled) VALUES('0','Local','',1)").Error)
	controller := newJobSchedulerForTestInternal(t, t.Context(), nil)
	owner, e := updater.NewUpdaterService(db, cfg, nil, nil, nil, nil, nil, nil, nil, nil, &config.Config{}, nil, nil, nil)
	require.NoError(t, e)
	owner.ConfigureImagePolicies(nil, func(ctx context.Context) error { return RefreshUpdateWindows(ctx, controller, owner) })
	store := policy.Store{DB: db.DB}
	row, e := store.Policy(t.Context(), "0", "registration", "stack")
	require.NoError(t, e)
	row.Mode = "scheduled"
	row.SetWindow(&dto.UpdateWindow{Kind: "weekly", Day: new(2), Time: "03:15"})
	require.NoError(t, store.SavePolicy(t.Context(), row, 0))
	row.Revision = 1
	require.NoError(t, owner.RefreshUpdateWindows(t.Context()))
	require.True(t, controller.HasJob("update-check"))
	name := updater.UpdateWindowJobKey("0", "stack", "stack")
	require.True(t, controller.HasJob(name))
	job, _ := controller.GetJob(name)
	require.Equal(t, "0 15 3 * * 2", job.Schedule(t.Context()))
	// Restart reconstructs jobs from the same persisted row.
	restarted := newJobSchedulerForTestInternal(t, t.Context(), nil)
	require.NoError(t, RefreshUpdateWindows(t.Context(), restarted, owner))
	require.True(t, restarted.HasJob(name))
	row.Mode = "check"
	row.SetWindow(nil)
	require.NoError(t, store.SavePolicy(t.Context(), row, 1))
	require.NoError(t, owner.RefreshUpdateWindows(t.Context()))
	require.False(t, controller.HasJob(name))
	row.Mode = "scheduled"
	row.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
	require.NoError(t, store.SavePolicy(t.Context(), row, 2))
	require.NoError(t, owner.RefreshUpdateWindows(t.Context()))
	require.True(t, controller.HasJob(name))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if e := policy.DeleteEnvironment(tx, "0"); e != nil {
			return e
		}
		return tx.Exec("DELETE FROM environments WHERE id='0'").Error
	}))
	require.NoError(t, owner.RefreshUpdateWindows(t.Context()))
	require.False(t, controller.HasJob(name))
}
