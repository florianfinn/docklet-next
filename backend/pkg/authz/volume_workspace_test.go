// Source: getarcaneapp/arcane/backend/pkg/authz/volume_workspace_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M12 persisted volume lifecycle and legacy restore cut.

package authz

import (
	"testing"

	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestVolumeWorkspaceRequiredPermissions(t *testing.T) {
	permissions, valid := VolumeWorkspaceRequiredPermissions([]volumetypes.WorkspaceFileChange{
		{Operation: volumetypes.FileOpCreateFile},
		{Operation: volumetypes.FileOpRename},
	})
	require.True(t, valid)
	require.Equal(t, []string{PermVolumesUpload, PermVolumesDelete}, permissions)

	permissions, valid = VolumeWorkspaceRequiredPermissions([]volumetypes.WorkspaceFileChange{{Operation: "restore_file"}})
	require.False(t, valid)
	require.Nil(t, permissions)
}
