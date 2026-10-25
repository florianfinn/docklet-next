// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"testing"

	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/image"

	"github.com/florianfinn/docklet-next/backend/internal/project/children/listing"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/tags"

	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
)

func TestProjectTags_ReconcilePreservesUIAndComposeOwnershipIsReadOnly(t *testing.T) {
	db := setupProjectTestDB(t)
	service := &ProjectService{db: db}
	service.initChildren(nil, nil)
	ctx := t.Context()
	projectModel := Project{
		ID:         "project-tags",
		Name:       "tagged",
		Path:       "/tmp/tagged",
		Status:     ProjectStatusStopped,
		IsArchived: true,
	}
	require.NoError(t, db.Create(&projectModel).Error)

	projectTags, err := service.UpdateProjectTag(ctx, projectModel.ID, " Database ", projecttypes.TagColorPurple, true, usertypes.Actor{})
	require.NoError(t, err)
	require.Equal(t, []projecttypes.Tag{{Name: "database", Color: projecttypes.TagColorPurple, Sources: []projecttypes.TagSource{projecttypes.TagSourceUI}}}, projectTags)

	require.NoError(t, service.reconcileComposeProjectTags(ctx, projectModel.ID, []projecttypes.TagOption{
		{Name: "DATABASE", Color: projecttypes.TagColorBlue},
		{Name: "maintenance-window", Color: projecttypes.TagColorOrange},
	}))
	projectTags, err = service.GetProjectTags(ctx, projectModel.ID)
	require.NoError(t, err)
	require.Equal(t, []projecttypes.Tag{
		{Name: "database", Color: projecttypes.TagColorBlue, Sources: []projecttypes.TagSource{projecttypes.TagSourceUI, projecttypes.TagSourceCompose}},
		{Name: "maintenance-window", Color: projecttypes.TagColorOrange, Sources: []projecttypes.TagSource{projecttypes.TagSourceCompose}},
	}, projectTags)

	_, err = service.UpdateProjectTag(ctx, projectModel.ID, "database", "", false, usertypes.Actor{})
	require.ErrorIs(t, err, tags.ErrComposeTagReadOnly)
	_, err = service.UpdateProjectTag(ctx, projectModel.ID, "maintenance-window", projecttypes.TagColorBlue, true, usertypes.Actor{})
	require.ErrorIs(t, err, tags.ErrComposeTagReadOnly)

	require.NoError(t, service.reconcileComposeProjectTags(ctx, projectModel.ID, nil))
	projectTags, err = service.UpdateProjectTag(ctx, projectModel.ID, "database", "", false, usertypes.Actor{})
	require.NoError(t, err)
	require.Empty(t, projectTags)
}

func TestProjectTags_FilterUsesExactORNames(t *testing.T) {
	db := setupProjectTestDB(t)
	ctx := t.Context()
	seededProjects := []Project{
		{ID: "one", Name: "one", Path: "/tmp/one", Status: ProjectStatusStopped},
		{ID: "two", Name: "two", Path: "/tmp/two", Status: ProjectStatusStopped},
		{ID: "three", Name: "three", Path: "/tmp/three", Status: ProjectStatusStopped},
	}
	require.NoError(t, db.Create(&seededProjects).Error)
	require.NoError(t, db.Create(&[]ProjectTag{
		{ProjectID: "one", Name: "database", Source: string(projecttypes.TagSourceUI)},
		{ProjectID: "two", Name: "maintenance", Source: string(projecttypes.TagSourceCompose)},
		{ProjectID: "three", Name: "database-prod", Source: string(projecttypes.TagSourceUI)},
	}).Error)

	var result []Project
	err := listing.ApplyProjectTagsDBFilter(db.WithContext(ctx).Model(&Project{}), " DATABASE,maintenance ").Order("id").Find(&result).Error
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.ElementsMatch(t, []string{"one", "two"}, []string{result[0].ID, result[1].ID})
	require.NotContains(t, []string{result[0].ID, result[1].ID}, "three")

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	svc := NewProjectService(db, settingsService, nil, nil, nil, nil, nil, config.Load(), nil, nil)
	items, _, err := svc.ListProjects(ctx, pagination.QueryParams{Search: "MAINT", Limit: -1})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "two", items[0].ID)
}

func discoveryListingForTest(imageService *image.ImageService) *listing.Service {
	service := &ProjectService{imageService: imageService}
	service.initChildren(nil, nil)
	return service.listing
}

func TestListProjectReferencesIncludesArchivedProjects(t *testing.T) {
	db := setupProjectTestDB(t)
	service := &ProjectService{db: db}
	require.NoError(t, db.Create(&Project{ID: "project-active", Name: "active", Path: "/projects/active", Status: ProjectStatusStopped}).Error)
	require.NoError(t, db.Create(&Project{ID: "project-archived", Name: "archived", Path: "/projects/archived", Status: ProjectStatusStopped, IsArchived: true}).Error)

	references, err := service.ListProjectReferences(t.Context())
	require.NoError(t, err)
	require.Equal(t, []projecttypes.Reference{
		{ID: "project-active", Name: "active"},
		{ID: "project-archived", Name: "archived"},
	}, references)
}
