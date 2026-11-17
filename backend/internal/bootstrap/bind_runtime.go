// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6af01ffd644ceb458b56c80d51c78e3f58847754
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original registration of resolved runtime paths for the projects-mount exemption.

package bootstrap

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
)

// configureBindRuntime hands bindpath the state paths as their owners resolved them; any value
// that cannot be made absolute registers nothing, which keeps every Docklet mount protected.
func configureBindRuntime(cfg *config.Config, db *database.DB, settingsService *settings.SettingsService) {
	if db == nil || settingsService == nil {
		return
	}
	state, ok := bindRuntimeState(cfg, db.Path())
	if !ok {
		return
	}
	bindpath.Configure(bindpath.Runtime{
		Projects: func(ctx context.Context) string {
			return settingsService.GetStringSetting(ctx, "projectsDirectory", "/app/data/projects")
		},
		State: state,
	})
}

// bindRuntimeState resolves relative values against the working directory, as their owners'
// file operations do. The database path is the one the database layer already decoded.
func bindRuntimeState(cfg *config.Config, databasePath string) ([]string, bool) {
	if cfg == nil || strings.TrimSpace(databasePath) == "" {
		return nil, false
	}
	state := []string{filepath.Dir(databasePath), cfg.DockerConfig}
	if !cfg.AgentMode {
		state = append(state, cfg.BackupDir)
	}
	if socket, ok := strings.CutPrefix(cfg.DockerHost, "unix://"); ok {
		state = append(state, socket)
	}
	resolved := make([]string, 0, len(state))
	for _, value := range state {
		if strings.TrimSpace(value) == "" {
			continue
		}
		abs, err := filepath.Abs(value)
		if err != nil {
			return nil, false
		}
		resolved = append(resolved, filepath.Clean(abs))
	}
	return resolved, true
}
