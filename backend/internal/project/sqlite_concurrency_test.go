// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Concurrent UI edits and whole-set Compose tag reconciliation on the migrated baseline.
package project

import (
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/tags"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"sync"
	"testing"
)

func TestSQLiteConcurrentProjectTagsAreAtomic(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	svc := &ProjectService{db: db}
	model := Project{ID: "tagged", Name: "tagged", Path: "example-project", Status: ProjectStatusStopped}
	require.NoError(t, db.Create(&model).Error)
	start := make(chan struct{})
	failures := make(chan error, 64)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				color := projecttypes.TagColorBlue
				if i%4 == 0 {
					color = projecttypes.TagColorPurple
				}
				err := svc.reconcileComposeProjectTags(t.Context(), model.ID, []projecttypes.TagOption{{Name: "alpha", Color: color}, {Name: "beta", Color: color}})
				if err != nil {
					failures <- err
				}
			} else {
				_, err := svc.UpdateProjectTag(t.Context(), model.ID, "alpha", projecttypes.TagColorGreen, true, usertypes.Actor{})
				if err != nil && !errors.Is(err, tags.ErrComposeTagReadOnly) {
					failures <- err
				}
			}
			var rows []ProjectTag
			if err := db.Where("project_id=? AND source='compose'", model.ID).Order("name").Find(&rows).Error; err != nil {
				failures <- err
				return
			}
			if len(rows) != 0 && (len(rows) != 2 || rows[0].Color != rows[1].Color) {
				failures <- fmt.Errorf("partial Compose tag set: %v", rows)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&ProjectTag{}).Where("project_id=? AND source='compose'", model.ID).Count(&count).Error)
	require.EqualValues(t, 2, count)
	require.NoError(t, svc.reconcileComposeProjectTags(t.Context(), model.ID, nil))
	_, err = svc.UpdateProjectTag(t.Context(), model.ID, "alpha", "", false, usertypes.Actor{})
	require.NoError(t, err)
}
