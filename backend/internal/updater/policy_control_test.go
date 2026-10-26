// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Remote freshness, durable notification truth and untrusted executor result boundaries.

package updater

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	a "github.com/florianfinn/docklet-next/types/activity"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPolicyRemoteOfflineOrDisabledIsStale(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "stack", "notify")
	now := time.Now().UTC()
	require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", one.ID).Updates(map[string]any{"environment_id": "remote", "checked_at": now}).Error)
	host := environment.Environment{BaseModel: database.BaseModel{ID: "remote"}, Name: "Example remote", ApiUrl: "https://agent.example.com", Status: "online", Enabled: true}
	require.NoError(t, f.db.Create(&host).Error)
	for _, tc := range []struct {
		name, status   string
		enabled, stale bool
	}{{"online", "online", true, false}, {"offline", "offline", true, true}, {"disabled", "online", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, f.db.Model(&host).Updates(map[string]any{"status": tc.status, "enabled": tc.enabled}).Error)
			got, err := f.s.GetImagePolicy(t.Context(), "remote", "stack")
			require.NoError(t, err)
			require.Equal(t, tc.stale, got.Stale)
			require.NotNil(t, got.CheckedAt)
			require.Empty(t, got.Code)
			require.Equal(t, "notify", got.Mode)
		})
	}
}

func TestPolicyNotificationCrashRemainsSendingWithoutReplay(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "stack", "notify")
	require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_delivery_commit_failure BEFORE UPDATE ON image_update_notifications WHEN NEW.status='sent' BEGIN SELECT RAISE(ABORT,'synthetic delivery commit failure'); END").Error)
	first, err := f.s.CheckPolicyProject(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Equal(t, "update.persistence-failed", first.PolicyCode)
	require.Equal(t, 1, f.notifications)
	var before policy.Notice
	require.NoError(t, f.db.First(&before).Error)
	require.Equal(t, "sending", before.Status)
	f.reopen(t)
	_, err = f.s.CheckPolicyProject(t.Context(), "0", "stack")
	require.NoError(t, err)
	row, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Len(t, row.Candidates, 1)
	require.Equal(t, "sending", row.Candidates[0].NotificationState)
	require.Equal(t, before.ID, row.Candidates[0].EntryID)
	require.Equal(t, 1, f.notifications)
}

func TestPolicyConcurrentObserverReportsSendingThenSent(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "stack", "notify")
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var sends atomic.Int32
	var once sync.Once
	f.p.ports.notify = func(context.Context, string, string, dto.UpdateCandidate) (int, error) {
		sends.Add(1)
		close(entered)
		<-release
		return 1, nil
	}
	result := make(chan error, 1)
	go func() {
		defer close(finished)
		_, err := f.s.CheckPolicyProject(t.Context(), "0", "stack")
		result <- err
	}()
	t.Cleanup(func() { once.Do(func() { close(release) }); <-finished })
	select {
	case <-entered:
	case err := <-result:
		require.FailNow(t, "First send did not start", "%v", err)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "First send did not start")
	}
	_, err := f.s.CheckPolicyProject(t.Context(), "0", "stack")
	require.NoError(t, err)
	during, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Len(t, during.Candidates, 1)
	require.Equal(t, "sending", during.Candidates[0].NotificationState)
	require.EqualValues(t, 1, sends.Load())
	once.Do(func() { close(release) })
	require.NoError(t, <-result)
	after, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Equal(t, "sent", after.Candidates[0].NotificationState)
	require.Equal(t, during.Candidates[0].EntryID, after.Candidates[0].EntryID)
	require.EqualValues(t, 1, sends.Load())
}

func TestPolicyUnknownExecutorCodeUsesDeclaredFallbackAndReceiptRetry(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-stack", "apply")
	f.save(t, "b-stack", "apply")
	f.wait = func(id string) (a.RuntimeSnapshot, error) {
		snapshot := f.receipts[id]
		snapshot.ImageLifecycle = &dto.LifecycleResult{Code: "agent.future-result"}
		f.receipts[id] = snapshot
		return snapshot, nil
	}
	run := st.Run{ID: "wrapper", AttemptCount: 1}
	out := f.run(t, "auto-update", run)
	require.Len(t, out.Targets, 1)
	require.Equal(t, "runtime.outcome-unknown", out.Targets[0].Code)
	require.NoError(t, out.ValidateReasons())
	require.Equal(t, 1, f.admissions)
	retry := f.run(t, "auto-update", st.Run{ID: run.ID, AttemptCount: 2})
	require.Len(t, retry.Targets, 1)
	require.Equal(t, "runtime.outcome-unknown", retry.Targets[0].Code)
	require.NoError(t, retry.ValidateReasons())
	require.Equal(t, 1, f.admissions)
	row, err := f.s.GetImagePolicy(t.Context(), "0", "a-stack")
	require.NoError(t, err)
	require.Equal(t, "runtime.outcome-unknown", row.ApplyCode)
	for id, snapshot := range f.receipts {
		snapshot.ImageLifecycle.Code = "update.deploy-failed"
		snapshot.Status = a.StatusFailed
		f.receipts[id] = snapshot
	}
	recovered := f.run(t, "auto-update", st.Run{ID: run.ID, AttemptCount: 3})
	require.Equal(t, "update.deploy-failed", recovered.Targets[0].Code)
	require.NoError(t, recovered.ValidateReasons())
	require.Equal(t, 1, f.admissions, "receipt retries never replay the update")
}

func TestPolicyNotifyToApplyPreservesSentHistory(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "stack", "notify")
	_, err := f.s.CheckPolicyProject(t.Context(), "0", "stack")
	require.NoError(t, err)
	before, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Len(t, before.Candidates, 1)
	require.Equal(t, "sent", before.Candidates[0].NotificationState)
	f.save(t, "stack", "apply")
	_, err = f.s.CheckPolicyProject(t.Context(), "0", "stack")
	require.NoError(t, err)
	after, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Len(t, after.Candidates, 1)
	require.Equal(t, "sent", after.Candidates[0].NotificationState)
	require.Equal(t, before.Candidates[0].EntryID, after.Candidates[0].EntryID)
	require.Equal(t, 1, f.notifications)
	require.Zero(t, f.admissions)
	var notices int64
	require.NoError(t, f.db.Model(&policy.Notice{}).Count(&notices).Error)
	require.EqualValues(t, 1, notices)
}

func TestPolicyApplyDoesNotDispatchPreviouslyPendingNotice(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "stack", "notify")
	candidate := dto.UpdateCandidate{ImageRef: "registry.example.com/app:1.0.0", Version: "1.1.0", Digest: f.check.ServiceUpdates["app"].UpdateInfo.LatestDigest, Platform: "linux/amd64"}
	entry, err := f.p.store.RecordCandidate(t.Context(), one, candidate, true)
	require.NoError(t, err)
	f.save(t, "stack", "apply")
	_, err = f.s.CheckPolicyProject(t.Context(), "0", "stack")
	require.NoError(t, err)
	after, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Len(t, after.Candidates, 1)
	require.Equal(t, "pending", after.Candidates[0].NotificationState)
	require.Equal(t, entry.EntryID, after.Candidates[0].EntryID)
	require.Zero(t, f.notifications)
}
