// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original AUTO-12 observed availability and metadata-only list regression proofs.

package backup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/browser"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func availabilityRow(t *testing.T, f *runtimePlan, id, destination, snapshot string) *volume.Backup {
	t.Helper()
	row := &volume.Backup{ID: id, VolumeName: "data", CreatedAt: time.Now().UTC(), Status: "partial", Format: volume.BackupFormatRustic, Destination: volume.BackupDestination(destination), LocalSnapshotID: snapshot}
	require.NoError(t, f.service.deps.DB.Table("volume_backups").Create(row).Error)
	return row
}
func loadAvailability(t *testing.T, f *runtimePlan, id string) volume.Backup {
	t.Helper()
	var row volume.Backup
	require.NoError(t, f.service.deps.DB.Table("volume_backups").Where("id = ?", id).First(&row).Error)
	return row
}
func TestBrowseMissingSnapshotPersistsObservationWithoutConfusingMissingPath(t *testing.T) {
	f := newRuntimePlan(t)
	ctx := f.targetContext("local")
	snapshot := strings.Repeat("f", 64)
	row := availabilityRow(t, f, "row", "archive", snapshot)
	cli, err := f.service.deps.Docker.GetClient(ctx)
	require.NoError(t, err)
	repository, err := f.service.repositoryFor(ctx, cli, "archive", true)
	require.NoError(t, err)
	b := browser.NewService(browser.Dependencies{Docker: f.service.deps.Docker, Engine: f.service.deps.Engine,
		Run: func(context.Context, string) (*volume.Backup, error) { return row, nil },
		Repository: func(context.Context, *client.Client, *volume.Backup) (backup.Repository, string, error) {
			return repository, snapshot, nil
		},
		Password: func(context.Context, *client.Client, ...backup.Repository) (string, error) { return "synthetic", nil }})
	f.exit = 1
	f.stdout = ""
	f.stderr = "Node `absent` not found in tree."
	_, _, err = b.BrowseBackupFiles(ctx, row.ID, "absent", pagination.QueryParams{Limit: 20})
	require.Error(t, err)
	require.Nil(t, loadAvailability(t, f, row.ID).SnapshotUnavailableAt)
	f.stderr = "Failed to read the contents of the file.\nContext:\n- path: /repository/snapshots/" + snapshot + "\nCaused by:\nNo such file or directory (os error 2)"
	before := time.Now().UTC()
	_, _, err = b.BrowseBackupFiles(ctx, row.ID, "", pagination.QueryParams{Limit: 20})
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "volume.backup.snapshot-not-found", problem.Code)
	observed := loadAvailability(t, f, row.ID)
	require.NotNil(t, observed.SnapshotUnavailableAt)
	require.False(t, observed.SnapshotUnavailableAt.Before(before))
	require.Equal(t, "partial", observed.Status, "availability must preserve the original execution result")
	require.Equal(t, "unavailable", entryInternal(&observed).Status)
	f.exit = 0
	f.stderr = ""
	f.stdout = "[]"
	_, _, err = b.BrowseBackupFiles(ctx, row.ID, "", pagination.QueryParams{Limit: 20})
	require.NoError(t, err)
	require.Nil(t, loadAvailability(t, f, row.ID).SnapshotUnavailableAt)
}
func TestRepositoryCheckReconcilesOnlyItsDestinationFromOneInventory(t *testing.T) {
	f := newRuntimePlan(t)
	ctx := f.targetContext("local")
	snapshot := strings.Repeat("a", 64)
	availabilityRow(t, f, "row", "archive", snapshot)
	availabilityRow(t, f, "other", "other-target", snapshot)
	cli, err := f.service.deps.Docker.GetClient(ctx)
	require.NoError(t, err)
	repository, err := f.service.repositoryFor(ctx, cli, "archive", false)
	require.NoError(t, err)
	for _, found := range []bool{false, true, false} {
		f.creates = nil
		f.stdout = "[]"
		if found {
			f.stdout = `[{"id":"` + snapshot + `"}]`
		}
		_, err = f.service.deps.Engine.CheckRepository(ctx, cli, repository, "synthetic", false)
		require.NoError(t, err)
		row := loadAvailability(t, f, "row")
		require.Equal(t, !found, row.SnapshotUnavailableAt != nil)
		require.Nil(t, loadAvailability(t, f, "other").SnapshotUnavailableAt)
		inventories := 0
		for _, created := range f.creates {
			if len(created.Cmd) > 0 && created.Cmd[0] == "snapshots" {
				inventories++
			}
		}
		require.Equal(t, 1, inventories)
	}
}
func TestBackupListUsesOnlyStoredAvailabilityWithoutEngineOrMount(t *testing.T) {
	f := newRuntimePlan(t)
	row := availabilityRow(t, f, "row", "archive", strings.Repeat("b", 64))
	observed := time.Now().UTC()
	require.NoError(t, f.service.deps.DB.Table("volume_backups").Where("id = ?", row.ID).Update("snapshot_unavailable_at", observed).Error)
	f.service.deps.Store.List = func(ctx context.Context, name string, _ pagination.QueryParams) ([]volume.Backup, int64, error) {
		var rows []volume.Backup
		err := f.service.deps.DB.WithContext(ctx).Table("volume_backups").Where("volume_name = ?", name).Find(&rows).Error
		return rows, int64(len(rows)), err
	}
	h := &Handler{service: f.service}
	output, err := h.ListBackups(t.Context(), &ListBackupsInput{VolumeName: "data"})
	require.NoError(t, err)
	require.Len(t, output.Body.Data, 1)
	require.Equal(t, "unavailable", output.Body.Data[0].Status)
	require.NotNil(t, output.Body.Data[0].SnapshotUnavailableAt)
	require.Empty(t, f.creates, "page views must never create a helper or run an engine")
	require.Empty(t, f.steps)
}
