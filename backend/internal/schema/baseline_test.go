// Source: docklet-next/issues/361
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original frozen prerelease chain catalog and SQL seed equivalence proof.

package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestFrozenPrereleaseChainMatchesReleaseBaseline(t *testing.T) {
	oldFS := os.DirFS("testdata/prerelease-chain")
	data, err := fs.ReadFile(oldFS, "hashes.json")
	require.NoError(t, err)
	var hashes map[string]string
	require.NoError(t, json.Unmarshal(data, &hashes))
	require.Len(t, hashes, 20)
	entries, err := fs.Glob(oldFS, "*.sql")
	require.NoError(t, err)
	require.Len(t, entries, 20)
	for _, name := range entries {
		data, err := fs.ReadFile(oldFS, name)
		require.NoError(t, err)
		sum := sha256.Sum256(data)
		require.Equal(t, hashes[name], hex.EncodeToString(sum[:]), name)
	}
	baselineFS, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	// Only 001 participates; later migrations must not change this frozen proof.
	baseline, err := fs.ReadFile(baselineFS, "001_baseline.sql")
	require.NoError(t, err)
	sources := []fs.FS{oldFS, fstest.MapFS{"001_baseline.sql": &fstest.MapFile{Data: baseline}}}
	var catalogs [][]string
	var seeds [][][]string
	for i, source := range sources {
		pool, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "schema.db"), true, false)
		require.NoError(t, err)
		defer pool.Close()
		provider, err := goose.NewProvider(goose.DialectSQLite3, pool, source)
		require.NoError(t, err)
		_, err = provider.Up(t.Context())
		require.NoError(t, err)
		owner := sqlRows(t, pool, "SELECT identity FROM docklet_database_metadata")
		require.Equal(t, [][]string{{[]string{"docklet-next:sqlite:1", "docklet-next:sqlite:2"}[i]}}, owner)
		dropped := sqlRows(t, pool, "SELECT type,name FROM sqlite_master WHERE tbl_name='environment_update_jobs' ORDER BY type,name")
		if i == 0 {
			require.Equal(t, [][]string{{"index", "idx_environment_update_jobs_status"}, {"index", "sqlite_autoindex_environment_update_jobs_1"}, {"table", "environment_update_jobs"}}, dropped)
		} else {
			require.Empty(t, dropped)
		}
		c := catalog(t, pool)
		if i == 0 {
			c = slices.DeleteFunc(c, func(row string) bool {
				for _, prefix := range []string{
					"table:environment_update_jobs:environment_update_jobs:",
					"index:idx_environment_update_jobs_status:environment_update_jobs:",
					"table_info:environment_update_jobs:", "foreign_key_list:environment_update_jobs:",
					"index_list:environment_update_jobs:",
					"index_xinfo:idx_environment_update_jobs_status:",
					"index_xinfo:sqlite_autoindex_environment_update_jobs_1:",
				} {
					if strings.HasPrefix(row, prefix) {
						return true
					}
				}
				return false
			})
			for j := range c {
				if strings.HasPrefix(c[j], "table:docklet_database_metadata:docklet_database_metadata:") {
					c[j] = strings.ReplaceAll(c[j], "docklet-next:sqlite:1", "docklet-next:sqlite:2")
				}
			}
		}
		catalogs = append(catalogs, c)
		// Every table's SQL seed data participates, including the asserted identity.
		var rows [][]string
		for _, table := range sqlRows(t, pool, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT IN ('goose_db_version','environment_update_jobs') ORDER BY name") {
			for _, row := range sqlRows(t, pool, "SELECT * FROM "+quote(table[0])) {
				if table[0] == "docklet_database_metadata" && i == 0 {
					row[0] = "docklet-next:sqlite:2"
				}
				rows = append(rows, append([]string{table[0]}, row...))
			}
		}
		seeds = append(seeds, rows)
	}
	require.Equal(t, catalogs[0], catalogs[1])
	require.Equal(t, seeds[0], seeds[1])
	require.Equal(t, [][]string{{"docklet_database_metadata", "docklet-next:sqlite:2"}, {"settings", "updateNotify", "false"}, {"settings", "vulnerabilityScanSchedule", `{"mode":"manual"}`}}, seeds[1])
}
