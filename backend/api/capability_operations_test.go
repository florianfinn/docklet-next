// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D6 host producer vocabulary regression proof. M14 host maintenance and availability.

package api

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

func TestOperationCapabilityHostStatuses(t *testing.T) {
	for _, tc := range []struct {
		status environment.EnvironmentStatus
		reason string
	}{
		{environment.EnvironmentStatusTokenRejected, "agent.invalid-token"},
		{environment.EnvironmentStatusUpdateRequired, "agent.update-required"},
		{environment.EnvironmentStatusOffline, "agent.unavailable"},
		{environment.EnvironmentStatusOnline, ""},
		{environment.EnvironmentStatusStandby, ""},
	} {
		t.Run(string(tc.status), func(t *testing.T) { require.Equal(t, tc.reason, operationHostStatus(string(tc.status))) })
	}
}

func TestD7CapabilityReadersUseInjectedSettingsAndFreshEdgeState(t *testing.T) {
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "capabilities.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	hosts := environment.NewEnvironmentService(db, nil, nil, nil, nil, nil)
	cfg := &config.Config{UIConfigurationDisabled: true}
	readers := operationCapabilityReaders(HandlerDeps{Environment: environment.New(hosts, nil, nil, nil, cfg, nil)}, cfg)
	require.Equal(t, "settings.ui-configuration-disabled", readers.SettingsWriteReason(t.Context()))
	cfg.UIConfigurationDisabled = false
	require.Empty(t, readers.SettingsWriteReason(t.Context()))
	env := environment.Environment{Name: "Example", ApiUrl: "edge://pending", Enabled: true, IsEdge: true, Status: "offline"}
	env.ID = "capability-example"
	require.NoError(t, db.Create(&env).Error)
	require.Equal(t, "agent.unavailable", readers.HostReason(t.Context(), env.ID))
	edge.GetPollRuntimeRegistry().Update(env.ID, time.Second, time.Now())
	require.Empty(t, readers.HostReason(t.Context(), env.ID))
	require.NoError(t, db.Model(&env).Update("status", "update-required").Error)
	require.Equal(t, "agent.update-required", readers.HostReason(t.Context(), env.ID))
}

// Edit and connection checks admit disabled and update-required hosts like
// their writes; only a missing host is refused (AUTO-8).
func TestD7HostOperationReaderMatchesWrites(t *testing.T) {
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "operations.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	hosts := environment.NewEnvironmentService(db, nil, nil, nil, nil, nil)
	cfg := &config.Config{}
	readers := operationCapabilityReaders(HandlerDeps{Environment: environment.New(hosts, nil, nil, nil, cfg, nil)}, cfg)
	for _, tc := range []struct {
		id, status string
		enabled    bool
	}{{"operation-online", "online", true}, {"operation-disabled", "online", false}, {"operation-v10", "update-required", true}, {"operation-offline", "offline", true}} {
		env := environment.Environment{Name: tc.id, ApiUrl: "http://" + tc.id + ".example.com", Enabled: tc.enabled, Status: tc.status}
		env.ID = tc.id
		require.NoError(t, db.Create(&env).Error)
		if !tc.enabled {
			require.NoError(t, db.Model(&env).Update("enabled", false).Error)
		}
		require.Empty(t, readers.HostOperationReason(t.Context(), tc.id), tc.id)
	}
	require.Equal(t, "capability.unavailable", readers.HostOperationReason(t.Context(), "operation-missing"))
}
