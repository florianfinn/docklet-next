// Source: getarcaneapp/arcane/backend/pkg/utils/dbutil/dbutil_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: PostgreSQL retry test runs without database integration.

package dbutil

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestIsRetryableWriteErrorUsesPostgresCodes(t *testing.T) {
	for _, code := range []string{"40001", "40P01", "55P03"} {
		require.True(t, IsRetryableWriteError(fmt.Errorf("write: %w", &pgconn.PgError{Code: code})))
	}
	require.False(t, IsRetryableWriteError(&pgconn.PgError{Code: "23505"}))
	require.False(t, IsRetryableWriteError(nil))
}
