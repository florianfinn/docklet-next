// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original owning-host target admission and exact-set policy persistence proofs.

package backup

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func targetAdmissionService(t *testing.T) *Service {
	t.Helper()
	started := []string{}
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected Docker effect %s", r.URL.Path)
		w.WriteHeader(500)
	}), &started)
	require.NoError(t, s.deps.DB.AutoMigrate(&checkSetting{}, &backuptarget.Target{}, &backuptarget.Check{}, &policies.VolumeBackupPolicy{}))
	s.deps.Store.Latest = func(context.Context, string) (*volume.Backup, error) { return nil, nil }
	return s
}
func TestPolicySetCheckedUnderAdmissionAndUnchangedOnRefusal(t *testing.T) {
	for _, cfg := range []*config.Config{{AgentMode: true}, {EdgeAgent: true}} {
		s := targetAdmissionService(t)
		require.NoError(t, s.deps.DB.Exec("INSERT INTO settings(key,value) VALUES ('backupTargetAuthority','authority'),('backupTargetSetVersion','7')").Error)
		h := policies.NewHandler(s.policies, nil, nil, t.Context())
		h.AroundUpdate = s.aroundPolicyUpdate(nil, cfg)
		h.BeforeLocalUpdate = s.checkPolicySet
		for _, tc := range []struct {
			authority string
			version   int64
			valid     bool
		}{{"old", 7, false}, {"authority", 6, false}, {"authority", 8, false}, {"authority", 7, true}} {
			input := &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: "0", VolumeName: "data", Body: volume.UpdateBackupPolicies{Authority: tc.authority, SetVersion: tc.version, Policies: []volume.UpdateBackupPolicy{{LocalEnabled: true, Schedule: "0 0 2 * * *", RetentionCount: 7, TargetID: "local"}}}}
			_, err := h.UpdateBackupPolicy(t.Context(), input)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "backup.target.stale-set")
			}
			var n int64
			require.NoError(t, s.deps.DB.Model(&policies.VolumeBackupPolicy{}).Count(&n).Error)
			if tc.valid {
				require.EqualValues(t, 1, n)
			} else {
				require.Zero(t, n)
			}
		}
	}
}
func TestManualPlanUsesCurrentPolicyAndExplicitTargetBeforeAdmission(t *testing.T) {
	s := targetAdmissionService(t)
	require.NoError(t, s.deps.DB.Create(&policies.VolumeBackupPolicy{VolumeName: "data", LocalEnabled: true, TargetID: "missing", Schedule: "0 0 2 * * *"}).Error)
	_, err := s.resolveBackupPlanInternal(t.Context(), "data", dto.VolumeBackupTriggerManual, volume.CreateBackupRequest{}, nil)
	require.ErrorContains(t, err, "backup.target.unknown")
	plan, err := s.resolveBackupPlanInternal(t.Context(), "data", dto.VolumeBackupTriggerManual, volume.CreateBackupRequest{TargetID: "local"}, nil)
	require.NoError(t, err)
	require.Equal(t, volume.BackupDestinationLocal, plan.destination)
	plan, err = s.resolveBackupPlanInternal(context.Background(), "data", dto.VolumeBackupTriggerSafety, volume.CreateBackupRequest{}, nil)
	require.NoError(t, err)
	require.Equal(t, volume.BackupDestinationLocal, plan.destination)
}

func TestAgentPolicyRoutesRefuseUnknownRetiredAndWrongHostTargets(t *testing.T) {
	for _, cfg := range []*config.Config{{AgentMode: true}, {EdgeAgent: true}} {
		s := targetAdmissionService(t)
		require.NoError(t, s.deps.DB.Exec("INSERT INTO settings(key,value) VALUES ('backupTargetAuthority','authority'),('backupTargetSetVersion','7')").Error)
		at := time.Now().UTC()
		require.NoError(t, s.deps.DB.Create(&backuptarget.Target{ID: "retired", Name: "Retired", Kind: "local", HostID: new("0"), RetiredAt: &at}).Error)
		require.NoError(t, s.deps.DB.Create(&backuptarget.Target{ID: "foreign", Name: "Foreign", Kind: "local", HostID: new("other-host")}).Error)
		h := policies.NewHandler(s.policies, nil, nil, t.Context())
		h.AroundUpdate = s.aroundPolicyUpdate(nil, cfg)
		h.BeforeLocalUpdate = s.checkPolicySet
		for _, tc := range []struct{ id, code string }{{"unknown", "backup.target.unknown"}, {"retired", "backup.target.retired"}, {"foreign", "backup.target.wrong-host"}} {
			_, err := h.UpdateBackupPolicy(t.Context(), &policies.UpdateVolumeBackupPolicyInput{EnvironmentID: "0", VolumeName: "data", Body: volume.UpdateBackupPolicies{Authority: "authority", SetVersion: 7, Policies: []volume.UpdateBackupPolicy{{TargetID: tc.id, LocalEnabled: true, Schedule: "0 0 2 * * *", RetentionCount: 7}}}})
			require.ErrorContains(t, err, tc.code)
			var n int64
			require.NoError(t, s.deps.DB.Model(&policies.VolumeBackupPolicy{}).Count(&n).Error)
			require.Zero(t, n)
		}
	}
}
