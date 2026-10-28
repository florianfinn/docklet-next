// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original scheduled capture and safety-pin retention proofs.

package backup

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestRetentionCountsPartialAcrossPoliciesAndProtectsManualSafety(t *testing.T) {
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Table("volume_backups").AutoMigrate(&volume.Backup{}))
	require.NoError(t, db.AutoMigrate(&restore.Run{}))
	s := NewService(Dependencies{DB: &database.DB{DB: db}})
	at := time.Now().UTC()
	for i, row := range []volume.Backup{
		{ID: "current", VolumeName: "data", Trigger: "scheduled", Status: "running", PolicyID: "new-policy", LocalSnapshotID: "snapshot-current"},
		{ID: "partial", VolumeName: "data", Trigger: "scheduled", Status: "partial", PolicyID: "old-policy", LocalSnapshotID: "snapshot-partial"},
		{ID: "old", VolumeName: "data", Trigger: "scheduled", Status: "succeeded", PolicyID: "old-policy", LocalSnapshotID: "snapshot-old"},
		{ID: "manual", VolumeName: "data", Trigger: "manual", Status: "succeeded", LocalSnapshotID: "snapshot-manual"},
		{ID: "failed", VolumeName: "data", Trigger: "scheduled", Status: "failed"},
		{ID: "other", VolumeName: "other", Trigger: "scheduled", Status: "succeeded", LocalSnapshotID: "snapshot-other"},
		{ID: "safety-new", VolumeName: "data", Trigger: "safety", Status: "succeeded", LocalSnapshotID: "snapshot-safety1"},
		{ID: "safety-middle", VolumeName: "data", Trigger: "safety", Status: "succeeded", LocalSnapshotID: "snapshot-safety2"},
		{ID: "pinned", VolumeName: "data", Trigger: "safety", Status: "succeeded", LocalSnapshotID: "snapshot-safety3"},
		{ID: "expired-safety", VolumeName: "data", Trigger: "safety", Status: "succeeded", LocalSnapshotID: "snapshot-safety4"},
	} {
		row.CreatedAt = at.Add(-time.Duration(i) * time.Hour)
		require.NoError(t, db.Table("volume_backups").Create(&row).Error)
	}
	require.NoError(t, db.Create(&restore.Run{ID: "failed-restore", VolumeName: "data", Kind: "full", Phase: "writing", Status: "failed", Mode: "replace", SafetyBackupID: "pinned", PendingRestart: true, StartedAt: at}).Error)
	candidates, err := s.retentionCandidates(t.Context(), "data", "current", "scheduled", 2)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "old", candidates[0].ID)
	candidates, err = s.retentionCandidates(t.Context(), "data", "current", "scheduled", 1)
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.Equal(t, "partial", candidates[0].ID)
	candidates, err = s.retentionCandidates(t.Context(), "data", "", "safety", 2)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "expired-safety", candidates[0].ID)
	candidates, err = s.retentionCandidates(t.Context(), "data", "current", "scheduled", 0)
	require.NoError(t, err)
	require.Empty(t, candidates)
}
