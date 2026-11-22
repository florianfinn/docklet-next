// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original hub service tests for the mTLS boundary and the agent role.

package tailnet_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

// A hub restarted with Edge mTLS optional or required refuses the stored, enabled route without starting a node.
func TestServiceRefusesTheRouteUnderEdgeMTLSAfterRestart(t *testing.T) {
	initCrypto(t)
	for _, mode := range []string{"optional", "required"} {
		t.Run(mode, func(t *testing.T) {
			settings := &memorySettings{values: map[string]string{}}
			require.NoError(t, tailnet.NewStore(settings).Save(t.Context(), true, manual))
			cfg := &config.Config{EdgeMTLSMode: mode, TailscaleStateDir: filepath.Join(t.TempDir(), "tailscale")}
			service := tailnet.NewService(cfg, settings)
			t.Cleanup(func() { service.Stop(context.Background()) })
			service.Start(t.Context())
			status := service.Status(t.Context())
			require.True(t, status.Enabled)
			require.Equal(t, "error", status.Connection.State)
			require.Equal(t, tailnettypes.CodeMTLSUnsupported, status.Connection.Code)
			require.False(t, status.RouteAvailable)
			require.NoDirExists(t, cfg.TailscaleStateDir, "no node was started")
		})
	}
}

func TestAgentsHaveNoHubService(t *testing.T) {
	require.Nil(t, tailnet.NewService(&config.Config{AgentMode: true}, nil))
	require.Nil(t, tailnet.NewService(&config.Config{EdgeAgent: true}, nil))
	var none *tailnet.Service
	require.Nil(t, none.Manager())
}

func TestServiceUpdateKeepsTheSecretWriteOnly(t *testing.T) {
	initCrypto(t)
	settings := &memorySettings{values: map[string]string{}}
	service := tailnet.NewService(&config.Config{EdgeMTLSMode: "required", TailscaleStateDir: t.TempDir()}, settings)
	t.Cleanup(func() { service.Stop(context.Background()) })
	out, err := service.Update(t.Context(), tailnettypes.SettingsUpdate{Enabled: true, Mode: tailnettypes.ModeOAuth, ClientID: "example-client", ClientSecret: secret})
	require.NoError(t, err)
	require.True(t, out.SecretConfigured)
	require.Equal(t, "example-client", out.ClientID)
	read, err := service.Settings(t.Context())
	require.NoError(t, err)
	require.Equal(t, out, read)
	status, err := service.Reset(t.Context())
	require.NoError(t, err)
	require.False(t, status.Enabled)
	read, err = service.Settings(t.Context())
	require.NoError(t, err)
	require.True(t, read.SecretConfigured, "reset keeps the stored credential")
	require.False(t, read.Enabled)
}
