// Source: getarcaneapp/arcane/backend/internal/project/children/sync/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed Git sync env merge; functional cut: removed GitOps and saved Git repositories.

// Package sync keeps direct project environment files and compose metadata in sync.
package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/types/project"
)

func PersistEffectiveEnvContent(ctx context.Context, projectPath, projectsDirectory, envContent string) error {
	state, err := projects.ReadProjectEnvState(projectPath)
	if err != nil {
		return fmt.Errorf("read project env state: %w", err)
	}
	if info, err := os.Stat(filepath.Join(projectPath, projects.EffectiveEnvFileName)); err == nil && info.IsDir() {
		return fmt.Errorf("cannot save environment: %s is a directory", projects.EffectiveEnvFileName)
	}
	return projects.WriteManagedEnvFile(ctx, projectsDirectory, projectPath, projects.EffectiveEnvFileName, state.EffectiveUnreadable, envContent)
}

func ResolveStoredEffectiveEnvContent(state projects.ProjectEnvState) (string, error) {
	return state.EffectiveContent, nil
}

func EffectiveEnvContentForUpdate(projectPath string, envContent *string) (*string, error) {
	if envContent != nil {
		return envContent, nil
	}
	state, err := projects.ReadProjectEnvState(projectPath)
	if err != nil {
		return nil, err
	}
	if !state.HasEffective {
		return nil, nil
	}
	return &state.EffectiveContent, nil
}

func EnsureEffectiveEnvFile(ctx context.Context, projectPath, projectsDirectory string) error {
	return projects.EnsureEnvFile(ctx, projectsDirectory, projectPath)
}

// LoadComposeMetadata loads a discovered project's compose file once and
// returns the service count plus compose-go's effective project name.
func LoadComposeMetadata(ctx context.Context, dirPath, dirName, projectsDirectory string, autoInjectEnv bool, pathMapper *projects.PathMapper) (project.ComposeIdentity, error) {
	normName := projects.NormalizeProjectName(dirName)
	meta := project.ComposeIdentity{
		ResolvedProjectName: normName,
	}

	// Load unnamed first so COMPOSE_PROJECT_NAME from .env wins; fall back to
	// the normalized directory name when that fails.
	proj, _, err := projects.LoadComposeProjectFromDir(ctx, dirPath, "", projectsDirectory, autoInjectEnv, pathMapper)
	if err != nil {
		proj, _, err = projects.LoadComposeProjectFromDir(ctx, dirPath, normName, projectsDirectory, autoInjectEnv, pathMapper)
		if err != nil {
			return meta, err
		}
	} else if proj.Name != "" && proj.Name != normName {
		meta.ExplicitProjectName = true
	}

	meta.ServiceCount = len(proj.Services)
	if proj.Name != "" {
		meta.ResolvedProjectName = proj.Name
	}

	// Keep a COMPOSE_PROJECT_NAME override so containers match.
	if proj.Name != "" && proj.Name != normName {
		meta.ComposeProjectName = new(proj.Name)
	}

	return meta, nil
}
