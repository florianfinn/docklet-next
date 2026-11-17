// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6af01ffd644ceb458b56c80d51c78e3f58847754
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original tests for the resolved runtime state paths of the projects-mount exemption.

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

func TestBindRuntimeStateUsesOwnerResolvedPaths(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	// The database layer decodes file URLs; bindpath receives that decoded path.
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	databasePath, err := sqlitestore.Path("file:" + root + "/pro%6aects/state/docklet.db")
	require.NoError(t, err)
	cfg := &config.Config{DockerConfig: "projects/registry", BackupDir: "projects/backups", DockerHost: "unix:///run/docker.sock"}
	state, ok := bindRuntimeState(cfg, databasePath)
	require.True(t, ok)
	require.Equal(t, []string{filepath.Join(root, "projects/state"), filepath.Join(cwd, "projects/registry"), filepath.Join(cwd, "projects/backups"), "/run/docker.sock"}, state)

	cfg.AgentMode = true
	state, ok = bindRuntimeState(cfg, databasePath)
	require.True(t, ok)
	require.NotContains(t, state, filepath.Join(cwd, "projects/backups"), "agents keep no hub backup directory")

	_, ok = bindRuntimeState(cfg, "")
	require.False(t, ok, "an unknown database path registers nothing")
}
