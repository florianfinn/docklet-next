// Source: docklet-next/issues/242
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original bounded absent/history retention and conditional commit proofs.

package updater

import (
	"context"
	"encoding/json"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestUpdatePolicyPruning(t *testing.T) {
	f := newPolicyFixture(t)
	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	for _, name := range []string{"check", "scheduled", "admitted", "unknown", "returned"} {
		mode := "check"
		if name == "scheduled" {
			mode = "scheduled"
		}
		row := f.save(t, name, mode)
		require.NoError(t, f.db.Model(&policy.Policy{}).Where("id=?", row.ID).Update("missing_since", old).Error)
		if name == "admitted" || name == "unknown" {
			status, code := "admitted", ""
			if name == "unknown" {
				status = "settled"
				code = "runtime.outcome-unknown"
			}
			require.NoError(t, f.db.Create(&policy.Child{ID: name, PolicyID: row.ID, JobRunID: name, Revision: 1, Status: status, Code: code}).Error)
		}
	}
	require.NoError(t, f.p.store.PruneAbsent(t.Context(), "0", "example-registration", map[string]bool{"stack\x00returned": true}, now))
	var rows []policy.Policy
	require.NoError(t, f.db.Order("target_id").Find(&rows).Error)
	require.Len(t, rows, 4)
	for _, row := range rows {
		require.NotEqual(t, "check", row.TargetID)
		if row.TargetID == "returned" {
			require.Nil(t, row.MissingSince)
		}
	}
	// A held stack survives Docklet destruction; an ordinary row and dependants go.
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return policy.DeleteStack(tx, "0", "admitted") }))
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return policy.DeleteStack(tx, "0", "returned") }))
	require.NoError(t, f.db.Find(&rows).Error)
	require.Len(t, rows, 3)
	// Environment deletion owns all dependants, including unknown children.
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return policy.DeleteEnvironment(tx, "0") }))
	require.NoError(t, f.db.Find(&rows).Error)
	require.Empty(t, rows)
	var n int64
	require.NoError(t, f.db.Model(&policy.Child{}).Count(&n).Error)
	require.Zero(t, n)
}

func TestUpdatePolicyHistoryKeepsCurrentDedupAcrossRestart(t *testing.T) {
	f := newPolicyFixture(t)
	row := f.save(t, "app", "check")
	now := time.Now().UTC()
	old := now.Add(-31 * 24 * time.Hour)
	candidate := dto.UpdateCandidate{ImageRef: "registry.example.com/app:1.0.0", Version: "1.1.0", Digest: f.check.ServiceUpdates["app"].UpdateInfo.LatestDigest, Platform: "linux/amd64"}
	current, e := f.p.store.RecordCandidate(t.Context(), row, candidate, true)
	require.NoError(t, e)
	previous := candidate
	previous.Version = "1.0.1"
	superseded, e := f.p.store.RecordCandidate(t.Context(), row, previous, true)
	require.NoError(t, e)
	payload, e := json.Marshal([]dto.UpdateCandidate{current})
	require.NoError(t, e)
	require.NoError(t, f.db.Model(&policy.Policy{}).Where("id=?", row.ID).Update("candidates", string(payload)).Error)
	require.NoError(t, f.db.Model(&policy.Notice{}).Where("1=1").Updates(map[string]any{"status": "sent", "created_at": old}).Error)
	for _, name := range []string{"settled", "refused", "admitted", "unknown"} {
		status, code := name, ""
		if name == "unknown" {
			status = "settled"
			code = "runtime.outcome-unknown"
		}
		require.NoError(t, f.db.Create(&policy.Child{ID: name, PolicyID: row.ID, JobRunID: name, Revision: 1, Status: status, Code: code, SettledAt: &old}).Error)
	}
	require.NoError(t, f.p.store.PruneHistory(t.Context(), now))
	f.reopen(t)
	var notices []policy.Notice
	require.NoError(t, f.db.Find(&notices).Error)
	require.Len(t, notices, 1)
	require.Equal(t, current.EntryID, notices[0].ID)
	require.NotEqual(t, superseded.EntryID, notices[0].ID)
	again, e := f.p.store.RecordCandidate(t.Context(), row, candidate, true)
	require.NoError(t, e)
	require.Equal(t, current.EntryID, again.EntryID)
	require.Equal(t, "sent", again.NotificationState)
	var children []policy.Child
	require.NoError(t, f.db.Find(&children).Error)
	require.Len(t, children, 2)
}

func TestLateObservationCannotResurrectDeletedEnvironment(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "app", "check")
	f.p.ports.check = func(ctx context.Context, env, target string) (*projectDTO.UpdateInfo, error) {
		require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
			if e := policy.DeleteEnvironment(tx, env); e != nil {
				return e
			}
			return tx.Exec("DELETE FROM environments WHERE id=?", env).Error
		}))
		return f.check, nil
	}
	result, e := f.s.CheckPolicyProject(t.Context(), "0", "app")
	require.NoError(t, e)
	require.Equal(t, "update.policy-changed", result.PolicyCode)
	var n int64
	require.NoError(t, f.db.Model(&policy.Policy{}).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, f.db.Model(&policy.Notice{}).Count(&n).Error)
	require.Zero(t, n)
	projected, e := f.p.store.Policy(t.Context(), "0", "example-registration", "app")
	require.NoError(t, e)
	_, e = f.p.store.EnsureObservedPolicy(t.Context(), projected)
	require.Error(t, e)
}
