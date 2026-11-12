// Source: docklet-next/issues/361
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original agent refusal through the real bootstrap entry before service construction.

package bootstrap

import (
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestAgentBootstrapRefusesPrereleaseDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.db")
	pool, err := sql.Open("sqlite", sqlitestore.URL(path))
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, pool, os.DirFS("../schema/testdata/prerelease-chain"))
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	require.NoError(t, os.Chmod(path, 0640))
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	t.Setenv("AGENT_MODE", "true")
	t.Setenv("DATABASE_URL", sqlitestore.URL(path))
	t.Setenv("DOCKER_CONFIG", filepath.Join(dir, "docker-config"))
	t.Setenv("PROJECTS_DIRECTORY", filepath.Join(dir, "projects"))
	err = Bootstrap(t.Context())
	require.ErrorContains(t, err, "database.prerelease")
	require.ErrorContains(t, err, path)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(before), sha256.Sum256(after))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0640), info.Mode().Perm())
}
