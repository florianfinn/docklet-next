// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Late fenced requests cannot revive after clearing module data.

package proxy

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/types/features"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestP20DataDeletionChangesJournalGeneration(t *testing.T) {
	x := newFixture(t)
	m, err := NewModule(x.db, &config.Config{AgentMode: true}, x.mods, x.settings, nil, nil, nil)
	require.NoError(t, err)
	m.Agent.Runtime = x.f
	m.Agent.Engines = x.a.Engines
	oldGeneration := m.Agent.Generation()
	q := x.query()
	q.ID = "late-fenced"
	fenced, err := m.Agent.Fence(t.Context(), q)
	require.NoError(t, err)
	require.True(t, fenced.Fenced)
	require.NoError(t, x.settings.UpdateSetting(t.Context(), features.ProxySettingKey, "false"))
	require.NoError(t, x.mods.DeleteData(t.Context(), features.Proxy))
	require.NoError(t, x.settings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
	late := x.request
	late.ID = q.ID
	_, err = m.Agent.Apply(t.Context(), late)
	require.ErrorContains(t, err, "generation-mismatch")
	require.NotEqual(t, oldGeneration, m.Agent.Generation())
	require.Zero(t, x.f.publications)
	require.Equal(t, "old", string(x.f.files[x.request.Target].Content))
}
