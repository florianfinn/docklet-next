// Source: docklet-next/docs/design/update-policies.md
// Revision: 8907719aba6cd2d3037d287729f689d5ad9e3ec9
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Runtime reason rejection, polling freshness and durable host permission refusals.

package policies

import (
	"github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestPolicyReasonWritesRejectUndeclaredValues(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "apply")
	for _, field := range []string{"code", "apply_code", "bookkeeping_code", "Code", "ApplyCode", "BookkeepingCode"} {
		for _, bad := range []any{"FORBIDDEN", "update.undeclared", nil, gorm.Expr("?", "FORBIDDEN")} {
			err := f.db.Model(&Policy{}).Where("id = ?", p.ID).Updates(map[string]any{field: bad, "candidates": "[1]"}).Error
			require.ErrorIs(t, err, image.ErrUndeclaredReason, field)
			row, e := f.Policy(t.Context(), p.EnvironmentID, p.RegistrationID, p.TargetID)
			require.NoError(t, e)
			require.Equal(t, "[]", row.Candidates)
			require.Empty(t, row.Code)
			require.Empty(t, row.ApplyCode)
			require.Empty(t, row.BookkeepingCode)
		}
	}
	for _, field := range []string{"code", "apply", "bookkeeping"} {
		row := p
		switch field {
		case "code":
			row.Code = "FORBIDDEN"
		case "apply":
			row.ApplyCode = "BOGUS"
		case "bookkeeping":
			row.BookkeepingCode = "BOGUS"
		}
		require.ErrorIs(t, f.SavePolicy(t.Context(), row, row.Revision), image.ErrUndeclaredReason)
		require.ErrorIs(t, f.db.Model(&Policy{}).Where("id = ?", p.ID).Updates(row).Error, image.ErrUndeclaredReason)
		require.ErrorIs(t, f.db.Model(&Policy{}).Where("id = ?", p.ID).Updates(&row).Error, image.ErrUndeclaredReason)
		row.ID = "new-" + field
		row.Revision = 0
		require.ErrorIs(t, f.SavePolicy(t.Context(), row, 0), image.ErrUndeclaredReason)
	}
	require.NoError(t, f.db.Model(&Policy{}).Where("id = ?", p.ID).Updates(map[string]any{"code": "update.check-unavailable", "apply_code": "update.permission-denied", "bookkeeping_code": "update.persistence-failed"}).Error)
	require.NoError(t, f.db.Model(&Policy{}).Where("id = ?", p.ID).Updates(map[string]any{"code": "", "apply_code": "", "bookkeeping_code": ""}).Error)
}

func TestChildReasonWritesRejectUndeclaredValues(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "apply")
	child, _, err := f.PrepareChild(t.Context(), p, "wrapper")
	require.NoError(t, err)
	require.ErrorIs(t, f.db.Model(&Child{}).Where("id = ?", child.ID).Updates(map[string]any{"code": "FORBIDDEN", "status": "refused"}).Error, image.ErrUndeclaredReason)
	var stored Child
	require.NoError(t, f.db.First(&stored, "id = ?", child.ID).Error)
	require.Equal(t, "prepared", stored.Status)
	require.Empty(t, stored.Code)
	child.Code = "BOGUS"
	require.ErrorIs(t, f.db.Model(&Child{}).Where("id = ?", child.ID).Updates(child).Error, image.ErrUndeclaredReason)
	require.ErrorIs(t, f.db.Model(&Child{}).Where("id = ?", child.ID).Updates(&child).Error, image.ErrUndeclaredReason)
	child.ID = "another-child"
	child.JobRunID = "another-wrapper"
	child.Code = "BOGUS"
	require.ErrorIs(t, f.db.Create(&child).Error, image.ErrUndeclaredReason)
}
