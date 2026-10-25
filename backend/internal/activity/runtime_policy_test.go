// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Actual runtime receipt transaction freezes policy revision and child identity.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestRuntimePolicyAdministrativeReleaseSettlesHostCheckpointAtomically(t *testing.T) {
	f := b1Fixture(t)
	c, _ := policyCommand(t, f)
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", admitted.ActivityID).Updates(map[string]any{"executor_state": "unknown", "unknown_reason": "synthetic-transport-loss"}).Error)
	ref := RuntimeRunReference{Store: c.Store, RunID: admitted.ActivityID}
	disposition := runtimeDispositionInternal{Disposition: "released", By: "example-admin", Reason: "Stopped executor and inspected the target", ReleaseAcknowledged: true}
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER synthetic_release_failure BEFORE UPDATE ON image_policy_children BEGIN SELECT RAISE(ABORT,'synthetic release checkpoint failure'); END").Error)
	_, err = f.service.remoteDispositionInternal(t.Context(), ref, disposition)
	require.Error(t, err)
	var row RuntimeReceipt
	require.NoError(t, f.store.svc.db.First(&row, "run_id = ?", admitted.ActivityID).Error)
	require.Nil(t, row.HubDisposition)
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER synthetic_release_failure").Error)
	snapshot, err := f.service.remoteDispositionInternal(t.Context(), ref, disposition)
	require.NoError(t, err)
	require.Equal(t, "unknown", string(snapshot.OutcomeState), "administrative release does not claim effect success")
	var child policy.Child
	require.NoError(t, f.store.svc.db.First(&child, "id = ?", admitted.ActivityID).Error)
	require.Equal(t, "settled", child.Status)
	require.NoError(t, f.store.tx(func(tx *gorm.DB, _ time.Time) error {
		var saved RuntimeReceipt
		require.NoError(t, tx.First(&saved, "run_id = ?", admitted.ActivityID).Error)
		require.True(t, saved.ReleaseAcknowledged)
		return nil
	}))
}

func TestRuntimePolicyBridgePreservesBindingAcrossPreflight(t *testing.T) {
	f := b1Fixture(t)
	c, _ := policyCommand(t, f)
	prepared := imageCommand(f.command(), RuntimeUpdateProject).Options.Lifecycle
	httpService, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{
		ApplicationContext: t.Context(),
		Store:              func(context.Context, string) (RuntimeStoreBinding, error) { return c.Store, nil },
		PrepareTarget: func(_ context.Context, _ string, target RuntimeTarget) (RuntimeResolvedTarget, error) {
			return RuntimeResolvedTarget{Target: target}, nil
		},
		PrepareLifecycle: func(context.Context, string, RuntimeTarget, RuntimeStoreBinding, dto.LifecycleRequest) (*dto.LifecycleIntent, error) {
			return prepared, nil
		},
	})
	require.NoError(t, err)
	httpService.SetReady(true)
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{Sudo: true})
	ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &c.Actor)
	admitted, err := f.store.svc.AdmitPolicyUpdate(ctx, "0", c.Target.ID, c.Key, c.Options.Lifecycle.Request, *c.Options.Lifecycle.Policy)
	require.NoError(t, err)
	require.Equal(t, c.ProposedRunID, admitted.ActivityID)
	retained, found, err := f.service.RetainedCommand(t.Context(), c)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, c.Options.Lifecycle.Policy, retained.Options.Lifecycle.Policy)
	_, found, err = f.store.svc.PolicyUpdateSnapshot(t.Context(), "0", admitted.ActivityID)
	require.NoError(t, err)
	require.True(t, found)
	_, found, err = f.store.svc.PolicyUpdateSnapshot(t.Context(), "0", "example-absent-run")
	require.NoError(t, err)
	require.False(t, found)
}

func policyCommand(t *testing.T, f *b1FixtureInternal) (RuntimeCommand, policy.Policy) {
	t.Helper()
	c := imageCommand(f.command(), RuntimeUpdateProject)
	store := policy.Store{DB: f.store.svc.db.DB}
	p, err := store.Policy(t.Context(), c.Store.EnvironmentID, c.Store.RegistrationID, c.Target.ID)
	require.NoError(t, err)
	p.Mode = "apply"
	p.ActorID = c.Actor.ID
	p.TargetIdentity = c.Options.Lifecycle.TargetIdentity
	require.NoError(t, store.SavePolicy(t.Context(), p, 0))
	p.Revision = 1
	child, fresh, err := store.PrepareChild(t.Context(), p, "example-job-run")
	require.NoError(t, err)
	require.True(t, fresh)
	c.ProposedRunID = child.ID
	c.Key = child.ID
	c.Options.Lifecycle.Policy = &dto.PolicyBinding{PolicyID: p.ID, Revision: 1, JobRunID: child.JobRunID, ChildID: child.ID}
	return c, p
}
func TestRuntimePolicyAdmissionRevisionAndAtomicChild(t *testing.T) {
	for _, boundary := range []string{"stale", "commit", "child-write-failure"} {
		t.Run(boundary, func(t *testing.T) {
			f := b1Fixture(t)
			c, p := policyCommand(t, f)
			if boundary == "stale" {
				f.resolver.resolve = func(_ context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
					p.Mode = "off"
					require.NoError(t, (policy.Store{DB: f.store.svc.db.DB}).SavePolicy(t.Context(), p, 1))
					return RuntimeResolvedTarget{Target: target}, nil
				}
			}
			if boundary == "child-write-failure" {
				require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER synthetic_child_failure BEFORE UPDATE ON image_policy_children BEGIN SELECT RAISE(ABORT,'synthetic child failure'); END").Error)
			}
			admitted, err := f.service.Admit(t.Context(), c)
			var child policy.Child
			require.NoError(t, f.store.svc.db.First(&child, "id = ?", c.ProposedRunID).Error)
			if boundary != "commit" {
				require.Error(t, err)
				b1Zero(t, admitted)
				require.Equal(t, "prepared", child.Status)
				var count int64
				require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", c.ProposedRunID).Count(&count).Error)
				require.Zero(t, count)
				require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", c.ProposedRunID).Count(&count).Error)
				require.Zero(t, count)
				if boundary == "stale" {
					require.Equal(t, "update.policy-changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.ProposedRunID, admitted.ActivityID)
			require.Equal(t, "admitted", child.Status)
			require.Equal(t, admitted.ActivityID, child.RuntimeID)
			p.Mode = "off"
			require.NoError(t, (policy.Store{DB: f.store.svc.db.DB}).SavePolicy(t.Context(), p, 1))
			retained, found, err := f.service.RetainedCommand(t.Context(), c)
			require.NoError(t, err)
			require.True(t, found)
			require.EqualValues(t, 1, retained.Options.Lifecycle.Policy.Revision)
			again, err := f.service.Admit(t.Context(), retained)
			require.NoError(t, err)
			require.Equal(t, admitted.ActivityID, again.ActivityID)
			c.Options.Lifecycle.Policy.Revision = 99
			require.EqualValues(t, 1, retained.Options.Lifecycle.Policy.Revision, "owned intent cannot alias caller binding")
		})
	}
}
