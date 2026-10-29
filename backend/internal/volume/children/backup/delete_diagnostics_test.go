// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original proof that delete activities never store raw engine or daemon text.

package backup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestDeleteActivityStoresOnlyStableCode(t *testing.T) {
	const sentinel = "rustic exited with code 1: /srv/private-sentinel/config denied"
	f := newRuntimePlan(t)
	f.forgetEmpties = true
	db := f.service.deps.DB
	require.NoError(t, db.AutoMigrate(activity.Models()...))
	require.NoError(t, db.AutoMigrate(&event.Event{}))
	f.service.deps.Activity = activity.NewActivityService(db, nil)
	f.service.deps.Events = event.NewEventService(db, nil, nil)
	defer f.service.deps.Activity.CancelAndDrain(t.Context(), "")
	snapshot := strings.Repeat("a", 64)
	f.stdout = `[{"id":"` + snapshot + `"}]`
	entry := &volume.Backup{ID: "row", VolumeName: "data", Format: volume.BackupFormatRustic, Destination: "archive", LocalSnapshotID: snapshot, Status: "succeeded", CreatedAt: time.Now().UTC()}
	require.NoError(t, db.Table("volume_backups").Create(entry).Error)
	f.service.deps.Store.Run = func(ctx context.Context, id string) (*volume.Backup, error) {
		var row volume.Backup
		err := db.WithContext(ctx).Table("volume_backups").First(&row, "id = ?", id).Error
		return &row, err
	}
	reached := false
	f.service.deps.Store.Delete = func(context.Context, string) error {
		reached = true
		return fmt.Errorf("delete: %w", errors.New(sentinel))
	}
	ctx := context.WithValue(f.targetContext("local"), user.CurrentUserContextKey{}, &user.Actor{ID: "user", Username: "admin"})
	_, err := NewHandler(f.service, f.service.deps.Activity, nil, nil, t.Context(), nil).DeleteBackup(ctx, &DeleteBackupInput{BackupID: "row", EnvironmentID: "0"})
	require.Error(t, err)
	require.True(t, reached, "the raw failure must come from the delete path")
	require.NotContains(t, err.Error(), "private-sentinel")
	var rows []map[string]any
	require.NoError(t, db.Table("activities").Find(&rows).Error)
	require.NotEmpty(t, rows)
	for _, row := range rows {
		for column, value := range row {
			require.NotContains(t, fmt.Sprint(value), "private-sentinel", "activity column %s stores raw engine text", column)
		}
	}
}
