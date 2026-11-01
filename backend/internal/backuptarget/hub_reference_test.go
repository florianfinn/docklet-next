// Source: docklet-next/issues/286
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backuptarget

import (
	"context"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRetireRefusesHubPolicyUnderTargetFence(t *testing.T) {
	store := testStore(t)
	svc := NewService(Dependencies{Store: store, References: func(context.Context, Target) (dto.TargetReferences, []string, error) {
		return dto.TargetReferences{}, nil, nil
	}})
	target, err := svc.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Archive"})
	require.NoError(t, err)
	require.NoError(t, store.DB.Exec("CREATE TABLE system_backup_policies(target_id TEXT)").Error)
	require.NoError(t, store.DB.Exec("INSERT INTO system_backup_policies VALUES (?)", target.ID).Error)
	require.EqualError(t, svc.Retire(t.Context(), target.ID), "backup.target.in-use")
	var row Target
	require.NoError(t, store.DB.First(&row, "id = ?", target.ID).Error)
	require.Nil(t, row.RetiredAt)
}
