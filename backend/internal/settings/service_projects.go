// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push, template catalogue, GitOps, Swarm, standalone build workshop, OIDC and passkeys.

package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

func (s *SettingsService) NormalizeProjectsDirectory(ctx context.Context, projectsDirEnv string) error {
	if projectsDirEnv != "" {
		slog.DebugContext(ctx, "PROJECTS_DIRECTORY environment variable is set, skipping normalization", "value", projectsDirEnv)
		return nil
	}

	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	var projectsDirSetting SettingVariable
	err := s.db.WithContext(ctx).Where("key = ?", "projectsDirectory").First(&projectsDirSetting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		slog.DebugContext(ctx, "No projectsDirectory setting found, skipping normalization")
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to load projectsDirectory setting: %w", err)
	}

	value := strings.TrimSpace(projectsDirSetting.Value)
	isMapping := strings.Contains(value, ":") && (strings.HasPrefix(value, "/") || projects.IsWindowsDrivePath(value))
	if filepath.IsAbs(value) || isMapping {
		slog.DebugContext(ctx, "Projects directory already normalized or custom, skipping", "value", projectsDirSetting.Value)
		return nil
	}

	cwd, _ := os.Getwd()
	absPath, absErr := filepath.Abs(value)
	if absErr != nil {
		return fmt.Errorf("failed to resolve relative path to absolute: %w", absErr)
	}
	slog.InfoContext(ctx, "Normalizing projects directory from relative to absolute path", "from", value, "to", absPath, "base", cwd)
	if updateSettingValueNoRefreshErr := s.updateSettingValueNoRefreshInternal(ctx, "projectsDirectory", absPath); updateSettingValueNoRefreshErr != nil {
		return fmt.Errorf("failed to update projectsDirectory: %w", updateSettingValueNoRefreshErr)
	}
	if refreshSettingsCacheErr := s.refreshSettingsCacheInternal(context.WithoutCancel(ctx)); refreshSettingsCacheErr != nil {
		return refreshSettingsCacheErr
	}
	slog.InfoContext(ctx, "Successfully normalized projects directory")
	s.publishSettingsChangesInternal([]libarcane.SettingUpdate{{Key: "projectsDirectory", Value: absPath}})
	return nil
}
