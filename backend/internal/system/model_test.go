// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original copy-state mapping test for issue #32.

package system

import (
	"testing"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
)

func TestSystemBackupRunFromDTOCopyState(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		want        backuptypes.SystemBackupCopyState
	}{
		{"empty defaults to none", "", backuptypes.SystemBackupCopyNone},
		{"explicit state survives", string(backuptypes.SystemBackupCopyPending), backuptypes.SystemBackupCopyPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := systemBackupRunFromDTOInternal(backuptypes.SystemBackupRun{CopyState: backuptypes.SystemBackupCopyState(tc.input)})
			require.Equal(t, tc.want, run.CopyState)
		})
	}
}
