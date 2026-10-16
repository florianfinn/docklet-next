// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: SQLite retry classification with real driver errors.
package dbutil

import (
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIsRetryableWriteErrorUsesSQLiteBusy(t *testing.T) {
	require.True(t, IsRetryableWriteError(fmt.Errorf("write: %w", testsqlite.BusyError(t))))
	require.False(t, IsRetryableWriteError(nil))
	require.False(t, IsRetryableWriteError(errors.New("timeout")))
	for _, code := range []int{5, 5 | 1<<8, 5 | 2<<8} {
		require.True(t, isBusyCode(code))
	}
	for _, code := range []int{6, 19, 10, 13, 11} {
		require.False(t, isBusyCode(code))
	}
}
