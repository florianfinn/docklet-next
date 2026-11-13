// Source: docklet-next/issues/438
// Revision: 57e835f7cce5778fd3d164298e5b488e11abb71d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original fresh and forward candidate migration constraint proofs. M24 recovery evidence, validation and reason contracts.

package schema

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability/children/candidates"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	dto "github.com/florianfinn/docklet-next/types/vulnerability"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestCandidateFreshAndForwardMigrationConstraints(t *testing.T) {
	for _, forward := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "001-to-002"}[forward], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "host.db")
			if forward {
				baseline, err := fs.ReadFile(resources.FS, "migrations/sqlite/001_baseline.sql")
				require.NoError(t, err)
				pool, err := sqlitestore.Open(t.Context(), path, true, false)
				require.NoError(t, err)
				provider, err := goose.NewProvider(goose.DialectSQLite3, pool, fstest.MapFS{"001_baseline.sql": &fstest.MapFile{Data: baseline}})
				require.NoError(t, err)
				_, err = provider.Up(t.Context())
				require.NoError(t, err)
				require.NoError(t, pool.Close())
			}
			db, err := database.Initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			pool, err := db.SQLDB()
			require.NoError(t, err)
			require.Equal(t, [][]string{{"0"}, {"1"}, {"2"}, {"3"}}, sqlRows(t, pool, "SELECT version_id FROM goose_db_version WHERE is_applied=1 ORDER BY version_id"))
			now := time.Now().UTC()
			run := candidates.Run{CandidateRun: dto.CandidateRun{ID: "run", ActivityID: "activity", Trigger: "manual", Status: "running", StartedAt: now, Intelligence: "available"}}
			require.NoError(t, db.Create(&run).Error)
			invalid := run
			invalid.ID = "invalid"
			invalid.ActivityID = "invalid"
			invalid.Status = "unknown"
			require.Error(t, db.Create(&invalid).Error)
			proof := candidates.Proof{ID: "proof", Repository: "registry.example.com/app", ManifestDigest: "sha256:" + strings.Repeat("a", 64), ConfigDigest: "sha256:" + strings.Repeat("b", 64), Platform: "linux/amd64", ScannerVersion: "0.75.0", DBVersion: 2, DBUpdatedAt: now, Policy: "policy", CompletedAt: now}
			require.NoError(t, db.Create(&proof).Error)
			duplicate := proof
			duplicate.ID = "duplicate"
			require.Error(t, db.Create(&duplicate).Error)
			target := candidates.Target{ID: "target", RunID: run.ID, Repository: proof.Repository, ManifestDigest: proof.ManifestDigest, ConfigDigest: proof.ConfigDigest, Platform: proof.Platform, State: "running", AttemptID: "attempt", Consumer: dto.CandidateConsumer{ContainerID: "consumer"}}
			require.NoError(t, db.Create(&target).Error)
			badState := target
			badState.ID = "bad-state"
			badState.State = "unexpected"
			badState.Consumer.ContainerID = "other-consumer"
			require.Error(t, db.Create(&badState).Error)
			invalidTarget := target
			invalidTarget.ID = "bad-target"
			invalidTarget.RunID = "missing-run"
			require.Error(t, db.Create(&invalidTarget).Error)
			item := candidates.Item{ProofID: proof.ID, CandidateItem: dto.CandidateItem{Advisory: "CVE-2026-1234", Package: "example", Installed: "1", Severity: "CRITICAL"}}
			require.NoError(t, db.Create(&item).Error)
			item.ID = 0
			require.Error(t, db.Create(&item).Error)
			item.ID = 0
			item.ProofID = "missing-proof"
			require.Error(t, db.Create(&item).Error)
			require.Empty(t, sqlRows(t, pool, "PRAGMA foreign_key_check"))
			require.Equal(t, [][]string{{"ok"}}, sqlRows(t, pool, "PRAGMA integrity_check"))
		})
	}
}
