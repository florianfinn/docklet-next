// Source: docklet-next/issues/302
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original host projection and standby liveness tests.

package environment

import (
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestHostProjectionRejectsStaleHeartbeatAndPreservesStandby(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	now := f.tunnel.GetLastHeartbeat()
	for _, tc := range []struct {
		age       time.Duration
		status    string
		connected bool
	}{{90 * time.Second, "online", true}, {91 * time.Second, "offline", false}} {
		projected := dto.Environment{ID: f.id, IsEdge: true, Status: "online"}
		f.service.ApplyHostRuntimeStateAt(t.Context(), &projected, now.Add(tc.age))
		require.Equal(t, tc.status, projected.Status)
		require.Equal(t, tc.connected, *projected.Connected)
	}
	edge.GetPollRuntimeRegistry().Update(f.id, time.Hour, now)
	projected := dto.Environment{ID: f.id, IsEdge: true, Status: "online"}
	f.service.ApplyHostRuntimeStateAt(t.Context(), &projected, now.Add(91*time.Second))
	require.Equal(t, "standby", projected.Status)
	require.False(t, *projected.Connected)
}

// A polling agent below the minimum protocol keeps update-required instead of
// standby, as the tunnel branch already does (UP1, #100).
func TestHostProjectionKeepsUpdateRequiredForPollingAgent(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	now := f.tunnel.GetLastHeartbeat()
	edge.GetPollRuntimeRegistry().Update(f.id, time.Hour, now)
	projected := dto.Environment{ID: f.id, IsEdge: true, Status: "update-required"}
	f.service.ApplyHostRuntimeStateAt(t.Context(), &projected, now.Add(91*time.Second))
	require.Equal(t, "update-required", projected.Status)
	require.False(t, *projected.Connected)
	require.NotNil(t, projected.LastPollAt)
}

// The stored incompatible verdict survives a tunnel drop, a failed Edge health
// probe and a hub restart, so later polls still project update-required.
func TestHostVerdictSurvivesTransportLossBeforePoll(t *testing.T) {
	for _, path := range []string{"disconnect", "restart", "health"} {
		t.Run(path, func(t *testing.T) {
			f := newAgentFixture(t, "edge", version.Info{AgentContractVersion: 10, AgentContractFingerprint: contract.AgentHubFingerprint})
			_, err := f.service.CheckAgentCompatibility(t.Context(), f.id)
			require.ErrorIs(t, err, ErrAgentUpdateRequired)
			edge.GetRegistry().Unregister(f.id)
			service := f.service
			switch path {
			case "disconnect":
				require.NoError(t, service.UpdateEnvironmentConnectionState(t.Context(), f.id, false))
			case "health":
				require.NoError(t, service.updateEnvironmentStatusInternal(t.Context(), f.id, string(EnvironmentStatusOffline)))
			default:
				service = NewEnvironmentService(f.service.db, f.service.httpClient, nil, nil, nil, nil)
				require.NoError(t, service.ReconcileEdgeStatusesOnStartup(t.Context()))
			}
			row, err := service.GetEnvironmentByID(t.Context(), f.id)
			require.NoError(t, err)
			require.Equal(t, "update-required", row.Status)
			edge.GetPollRuntimeRegistry().Update(f.id, time.Second, time.Now())
			detail, err := NewHandler(service, nil, nil, nil, nil, nil).GetEnvironment(t.Context(), &GetEnvironmentInput{ID: f.id})
			require.NoError(t, err)
			require.Equal(t, "update-required", detail.Body.Data.Status)
		})
	}
}

// A transport-loss write that read the row earlier must not replace a verdict
// stored in between; the write re-checks the protected status itself.
func TestHostVerdictSurvivesInterleavedTransportLossWrite(t *testing.T) {
	for _, path := range []string{"disconnect", "health"} {
		t.Run(path, func(t *testing.T) {
			f := newAgentFixture(t, "edge", version.Info{})
			require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Update("status", "online").Error)
			name := "test:verdict-interleave-" + path
			flipped := false
			require.NoError(t, f.service.db.Callback().Update().Before("gorm:begin_transaction").Register(name, func(tx *gorm.DB) {
				updates, ok := tx.Statement.Dest.(map[string]any)
				if !ok || flipped {
					return
				}
				if _, writesStatus := updates["status"]; !writesStatus {
					return
				}
				flipped = true
				require.NoError(t, tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE environments SET status = ? WHERE id = ?", "update-required", f.id).Error)
			}))
			t.Cleanup(func() { _ = f.service.db.Callback().Update().Remove(name) })
			if path == "disconnect" {
				require.NoError(t, f.service.UpdateEnvironmentConnectionState(t.Context(), f.id, false))
			} else {
				require.NoError(t, f.service.updateEnvironmentStatusInternal(t.Context(), f.id, string(EnvironmentStatusOffline)))
			}
			require.True(t, flipped)
			row, err := f.service.GetEnvironmentByID(t.Context(), f.id)
			require.NoError(t, err)
			require.Equal(t, "update-required", row.Status)
		})
	}
}

// A pairing callback that read the pending row before a verdict was stored must
// not write pending over it; pairing confirms the registration without a status write.
func TestPairingNeverReplacesNewerVerdict(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	keys := apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
	key, err := keys.CreateEnvironmentApiKey(t.Context(), f.id)
	require.NoError(t, err)
	require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Updates(map[string]any{"api_key_id": key.ID, "access_token": key.Key, "status": "pending"}).Error)
	// No transport: the pairing's background check fails and may not touch the verdict either.
	edge.GetRegistry().Unregister(f.id)
	flipped := false
	require.NoError(t, f.service.db.Callback().Query().After("gorm:query").Register("test:pairing-verdict", func(tx *gorm.DB) {
		row, ok := tx.Statement.Dest.(*Environment)
		if !ok || flipped || row.ID != f.id || row.Status != "pending" {
			return
		}
		flipped = true
		require.NoError(t, tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE environments SET status = ? WHERE id = ?", "update-required", f.id).Error)
	}))
	t.Cleanup(func() { _ = f.service.db.Callback().Query().Remove("test:pairing-verdict") })
	h := &EnvironmentHandler{environmentService: f.service, apiKeyService: keys}
	_, err = h.PairEnvironment(t.Context(), &PairEnvironmentInput{XAPIKey: key.Key})
	require.NoError(t, err)
	h.background.Wait()
	require.True(t, flipped)
	row, err := f.service.GetEnvironmentByID(t.Context(), f.id)
	require.NoError(t, err)
	require.Equal(t, "update-required", row.Status)
}
