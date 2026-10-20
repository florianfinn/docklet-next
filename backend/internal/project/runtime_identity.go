// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Fresh, read-only project identity for dormant runtime control.

package project

import (
	"cmp"
	"context"
	"fmt"

	projectsync "github.com/florianfinn/docklet-next/backend/internal/project/children/sync"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	kit "go.getarcane.app/kit/pkg"
	"strconv"
)

// ResolveRuntimeProjectIdentity finds one persisted owner using current files,
// environment and the executable Compose model. The boolean is identity
// evidence, not a permission or an archived-project mutability decision.
func (s *ProjectService) ResolveRuntimeProjectIdentity(ctx context.Context, projectID, composeName string) (projecttypes.Reference, bool, error) {
	if projectID == "" && composeName == "" {
		return projecttypes.Reference{}, false, nil
	}
	var rows []Project
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return projecttypes.Reference{}, false, fmt.Errorf("list runtime project identities: %w", err)
	}
	var match projecttypes.Reference
	owners := make(map[string]int, len(rows))
	for i := range rows {
		row := &rows[i]
		// A requested ID must be checked, even when the name lookup would not
		// otherwise select it. Unreadable peers, including archived rows, can
		// conceal duplicate ownership and therefore block every successful lookup.
		name, err := s.runtimeComposeName(ctx, row)
		if err != nil {
			return projecttypes.Reference{}, false, fmt.Errorf("read runtime project identity: %w", err)
		}
		if name == "" {
			continue
		}
		owners[name]++
		if projectID != "" {
			if row.ID == projectID {
				match = projecttypes.Reference{ID: row.ID, Name: name}
			}
		} else if name == composeName {
			match = projecttypes.Reference{ID: row.ID, Name: name}
		}
	}
	if match.ID == "" || (composeName != "" && match.Name != composeName) || owners[match.Name] != 1 {
		return projecttypes.Reference{}, false, nil
	}
	return match, match.ID != "", nil
}

func (s *ProjectService) runtimeComposeName(ctx context.Context, row *Project) (string, error) {
	selected, err := s.ResolveProjectComposeFile(ctx, row)
	if err != nil {
		return "", err
	}
	settings := s.settingsService.GetSettingsOrDefaults(ctx)
	projectsDirectory, err := projects.GetProjectsDirectory(ctx, settings.ProjectsDirectory.Value)
	if err != nil {
		return "", err
	}
	autoInject := kit.ParseOrDefault(settings.AutoInjectEnv.Value, false, strconv.ParseBool)
	dirName := cmp.Or(derefRuntimeDirName(row.DirName), row.Name)
	metadata, err := projectsync.LoadComposeMetadata(ctx, row.Path, dirName, projectsDirectory, &row.Name, autoInject, s.projectPathMapper(ctx))
	if err != nil {
		return "", err
	}
	model, executablePath, err := s.loadComposeProjectForProject(ctx, row, nil)
	if err != nil {
		return "", err
	}
	if executablePath != selected || model.Name == "" || metadata.ResolvedProjectName == "" || model.Name != metadata.ResolvedProjectName {
		return "", fmt.Errorf("project Compose identity differs between metadata and executable model")
	}
	return model.Name, nil
}

func derefRuntimeDirName(name *string) string {
	if name == nil {
		return ""
	}
	return *name
}
