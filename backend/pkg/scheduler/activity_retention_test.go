// Source: docklet-next/issues/355
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original retention limits and post-save effect regression guards.

package scheduler

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

func TestM19RetentionOnSaveKeepsActiveRuns(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "retention.db")), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	s, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	defer s.Stop(context.Background())
	events := event.NewEventService(db, &config.Config{}, nil)
	activities := activity.NewActivityService(db, nil)
	NewEventCleanupJob(events, activities, s)
	for i := range 70 {
		require.NoError(t, db.Create(&event.Event{BaseModel: database.BaseModel{ID: fmt.Sprintf("e-%03d", i)}, Type: "container.start", Title: "Example", Timestamp: time.Now().Add(time.Duration(i) * time.Second)}).Error)
	}
	for i := range 18 {
		state := activityDTO.StatusSuccess
		var end *time.Time = new(time.Now().Add(time.Duration(i) * time.Second))
		if i >= 15 {
			state = activityDTO.StatusRunning
			end = nil
		}
		require.NoError(t, db.Create(&activity.Activity{BaseModel: database.BaseModel{ID: fmt.Sprintf("r-%03d", i)}, EnvironmentID: "0", Type: activityDTO.TypeResourceAction, Status: state, StartedAt: time.Now(), EndedAt: end}).Error)
	}
	_, err = s.UpdateSettings(t.Context(), settingsDTO.Update{ActivityEventLimit: new("50"), ActivityRunLimit: new("10")})
	require.NoError(t, err)
	// Stop joins every accepted settings effect before inspecting the resulting rows.
	require.NoError(t, s.Stop(context.Background()))
	var n int64
	require.NoError(t, db.Model(&event.Event{}).Count(&n).Error)
	require.EqualValues(t, 50, n)
	require.NoError(t, db.Model(&activity.Activity{}).Where("status='success'").Count(&n).Error)
	require.EqualValues(t, 10, n)
	require.NoError(t, db.Model(&activity.Activity{}).Where("status='running'").Count(&n).Error)
	require.EqualValues(t, 3, n)
}
