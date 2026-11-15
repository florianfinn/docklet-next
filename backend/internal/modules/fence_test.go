// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original assertion that persisted unfinished reads share the admission gate.

package modules

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/features"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestM27M4UnfinishedSourceSharesAdmissionGate(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "gate.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	s, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Stop(context.WithoutCancel(t.Context()))) })
	m := New(s)
	reads := 0
	require.NoError(t, m.Register(features.Proxy, Hooks{Unfinished: func(context.Context) (string, error) {
		reads++
		// A read outside the gate can race an admission committing its pending row.
		unlocked := m.gates[features.Proxy].TryLock()
		if unlocked {
			m.gates[features.Proxy].Unlock()
		}
		require.False(t, unlocked, "the persisted unfinished read must run inside the admission gate")
		return "", nil
	}}))
	_, err = s.UpdateSettings(t.Context(), settingsDTO.Update{ModuleProxyEnabled: new("false")})
	require.NoError(t, err)
	require.Equal(t, 1, reads)
}
