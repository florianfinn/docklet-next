// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original authoritative policy refusal and scheduler projection proof.

package policies

import (
	"errors"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPolicyValidationBeforePersistenceOrReschedule(t *testing.T) {
	g, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, g.AutoMigrate(&VolumeBackupPolicy{}))
	s := NewService(Dependencies{DB: &database.DB{DB: g}, LatestRun: noLatestRunInternal})
	scheduler := &volumeBackupPolicySchedulerInternal{jobs: map[string]scheduler.Job{}}
	require.NoError(t, s.SetScheduler(t.Context(), scheduler, newVolumeAdmissionForTestInternal(t)))
	valid := volume.UpdateBackupPolicy{Enabled: true, Schedule: "0 0 2 * * *", RetentionCount: 7, LocalEnabled: true}
	saved, err := s.UpdateBackupPolicies(t.Context(), "data", []volume.UpdateBackupPolicy{valid})
	require.NoError(t, err)
	valid.ID = saved.Policies[0].ID
	for _, c := range []struct {
		name    string
		updates []volume.UpdateBackupPolicy
		want    string
	}{
		{"multiple", []volume.UpdateBackupPolicy{valid, valid}, "volume.backup.policy-single"},
		{"five-field", []volume.UpdateBackupPolicy{func() volume.UpdateBackupPolicy { p := valid; p.Schedule = "0 2 * * *"; return p }()}, "volume.backup.policy-invalid"},
		{"cron-value", []volume.UpdateBackupPolicy{func() volume.UpdateBackupPolicy { p := valid; p.Schedule = "0 90 2 * * *"; return p }()}, "volume.backup.policy-invalid"},
		{"negative-retention", []volume.UpdateBackupPolicy{func() volume.UpdateBackupPolicy { p := valid; p.RetentionCount = -1; return p }()}, "volume.backup.policy-invalid"},
		{"large-retention", []volume.UpdateBackupPolicy{func() volume.UpdateBackupPolicy { p := valid; p.RetentionCount = 3651; return p }()}, "volume.backup.policy-invalid"},
		{"disabled-local", []volume.UpdateBackupPolicy{func() volume.UpdateBackupPolicy { p := valid; p.LocalEnabled = false; return p }()}, "volume.backup.policy-invalid"},
		{"foreign-id", []volume.UpdateBackupPolicy{func() volume.UpdateBackupPolicy { p := valid; p.ID = "foreign"; return p }()}, "volume.backup.policy-invalid"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := s.UpdateBackupPolicies(t.Context(), "data", c.updates)
			var p *dto.Problem
			require.True(t, errors.As(err, &p))
			require.Equal(t, c.want, p.Code)
			now, err := s.GetBackupPolicies(t.Context(), "data")
			require.NoError(t, err)
			require.Equal(t, saved, now)
			require.Len(t, scheduler.jobs, 1)
		})
	}
	valid.RetentionCount = 0
	_, err = s.UpdateBackupPolicies(t.Context(), "data", []volume.UpdateBackupPolicy{valid})
	require.NoError(t, err)
	valid.RetentionCount = 3650
	_, err = s.UpdateBackupPolicies(t.Context(), "data", []volume.UpdateBackupPolicy{valid})
	require.NoError(t, err)
}

type projectedScheduler struct {
	volumeBackupPolicySchedulerInternal
	next time.Time
}

func (s *projectedScheduler) GetJobRuntimeState(string) (scheduler.JobRuntimeState, bool) {
	return scheduler.JobRuntimeState{NextRun: &s.next, Scheduled: true}, true
}
func TestPolicyTimeZoneAndNextRunComeFromOwningScheduler(t *testing.T) {
	g, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, g.AutoMigrate(&VolumeBackupPolicy{}))
	zone, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	s := NewService(Dependencies{DB: &database.DB{DB: g}, LatestRun: noLatestRunInternal, Location: zone})
	scheduler := &projectedScheduler{volumeBackupPolicySchedulerInternal: volumeBackupPolicySchedulerInternal{jobs: map[string]scheduler.Job{}}, next: time.Date(2026, 11, 1, 7, 0, 0, 0, zone)}
	require.NoError(t, s.SetScheduler(t.Context(), scheduler, newVolumeAdmissionForTestInternal(t)))
	result, err := s.UpdateBackupPolicies(t.Context(), "data", []volume.UpdateBackupPolicy{{Enabled: true, Schedule: "0 0 2 * * *", RetentionCount: 0, LocalEnabled: true}})
	require.NoError(t, err)
	require.Equal(t, "Asia/Tokyo", result.TimeZone)
	require.Equal(t, scheduler.next, *result.Policies[0].NextRunAt)
}
