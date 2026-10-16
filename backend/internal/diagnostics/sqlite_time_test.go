// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Explicit Francis millisecond diagnostics boundary.
package diagnostics

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestFrancisDiagnosticsUseMilliseconds(t *testing.T) {
	expected := time.Date(2026, 10, 16, 12, 34, 56, 123000000, time.UTC)
	var scanned actorTimeInternal
	require.NoError(t, scanned.Scan(expected.UnixMilli()))
	require.Equal(t, expected, time.Time(scanned))
	require.Equal(t, time.UTC, time.Time(scanned).Location())
	require.NoError(t, scanned.Scan(nil))
	require.True(t, time.Time(scanned).IsZero())
	require.Error(t, scanned.Scan("untyped timestamp"))
}
