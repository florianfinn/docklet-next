// Source: docklet-next/issues/626
// Revision: 6dc802124ce20a9c53693493ca66a85d1820ef19
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original bounded producer resource-name regression tests for M41 S3.

package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProjectEditActivityResourceName(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		name := "current-name"
		if fallback {
			name = "lookup-failure"
		}
		t.Run(name, func(t *testing.T) {
			composeLogDocker(t)
			svc, root := runtimeIdentityFixture(t)
			svc.eventService = event.NewEventService(svc.db, nil, nil)
			row := runtimeIdentityProject(t, svc, root, "example-project", "example-stack", "services:\n  app:\n    image: example.invalid/app:1\n")
			if fallback {
				var failed atomic.Bool
				key := "test:project-display-lookup-failure"
				require.NoError(t, svc.db.Callback().Query().Before("gorm:query").Register(key, func(tx *gorm.DB) {
					if tx.Statement.Table == "projects" && failed.CompareAndSwap(false, true) {
						tx.AddError(errors.New("display lookup unavailable"))
					}
				}))
				t.Cleanup(func() { require.NoError(t, svc.db.Callback().Query().Remove(key)) })
			}
			h := &ProjectHandler{projectService: svc, activityService: activity.NewActivityService(svc.db, nil), appCtx: t.Context()}
			ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.Actor{ID: "operator", Username: "operator"})
			content := "services:\n  app:\n    image: example.invalid/app:2\n"
			out, err := h.UpdateProject(ctx, &UpdateProjectInput{EnvironmentID: "0", ProjectID: row.ID, Body: projecttypes.UpdateProject{ComposeContent: &content}})
			require.NoError(t, err, "name lookup failure must not refuse the edit")
			require.Equal(t, row.ID, out.Body.Data.ID)
			actual, err := os.ReadFile(filepath.Join(row.Path, "compose.yaml"))
			require.NoError(t, err)
			require.Equal(t, content, string(actual), "the real edit must still persist")
			var stored activity.Activity
			require.NoError(t, svc.db.First(&stored, "id = ?", *out.Body.Data.ActivityID).Error)
			require.Equal(t, activitytypes.StatusSuccess, stored.Status)
			require.Equal(t, row.ID, *stored.ResourceID, "activity resource id must remain unchanged")
			expected := row.Name
			if fallback {
				expected = row.ID
			}
			require.NotNil(t, stored.ResourceName, "edit activity must retain a resource name")
			require.Equal(t, expected, *stored.ResourceName, "edit activity must use current project name or id fallback")
		})
	}
}
