// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original fresh and sequential recovery migration constraints. M24 recovery evidence, validation and reason contracts.

package schema

import (
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestRecoveryFreshAndSequentialMigration(t *testing.T) {
	for _, forward := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "001-002-003"}[forward], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recovery.db")
			if forward {
				prior := fstest.MapFS{}
				for _, name := range []string{"001_baseline.sql", "002_candidate_scans.sql"} {
					body, err := fs.ReadFile(resources.FS, "migrations/sqlite/"+name)
					require.NoError(t, err)
					prior[name] = &fstest.MapFile{Data: body}
				}
				pool, err := sqlitestore.Open(t.Context(), path, true, false)
				require.NoError(t, err)
				provider, err := goose.NewProvider(goose.DialectSQLite3, pool, prior)
				require.NoError(t, err)
				_, err = provider.UpTo(t.Context(), 1)
				require.NoError(t, err)
				_, err = provider.UpTo(t.Context(), 2)
				require.NoError(t, err)
				require.NoError(t, pool.Close())
			}
			db, err := database.Initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			row := activity.RuntimeObservedRun{RunID: "run", Kind: "stack", Mode: "scheduled", CreatedAt: time.Now()}
			require.NoError(t, db.Create(&row).Error)
			for _, field := range []string{"kind", "mode", "outcome"} {
				require.Error(t, db.Model(&activity.RuntimeObservedRun{}).Where("run_id = ?", row.RunID).Update(field, "invalid").Error)
			}
			require.NoError(t, db.Model(&activity.RuntimeObservedRun{}).Where("run_id = ?", row.RunID).Update("evidence", strings.Repeat("a", 65536)).Error)
			require.Error(t, db.Model(&activity.RuntimeObservedRun{}).Where("run_id = ?", row.RunID).Update("evidence", strings.Repeat("a", 65537)).Error)
			require.Error(t, db.Model(&activity.RuntimeObservedRun{}).Where("run_id = ?", row.RunID).Update("evidence", strings.Repeat("ä", 32769)).Error)
			failed := policy.FailedCandidate{PolicyID: "policy", TargetIdentity: "identity", Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64), RunID: "run", CreatedAt: row.CreatedAt}
			require.NoError(t, db.Create(&failed).Error)
			require.Error(t, db.Create(&failed).Error)
			for _, model := range BaselineModels() {
				require.NotEqual(t, "*activity.RuntimeObservedRun", fmt.Sprintf("%T", model))
				require.NotEqual(t, "*policies.FailedCandidate", fmt.Sprintf("%T", model))
			}
		})
	}
}
