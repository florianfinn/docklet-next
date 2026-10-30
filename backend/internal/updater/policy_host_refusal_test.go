// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original policy child through real admission to durable host refusal.

package updater

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	user "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type refusalResolver struct{}

func (refusalResolver) ResolveRuntimeTarget(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
	return activity.RuntimeResolvedTarget{Target: target}, nil
}

func TestImagePolicyChildPersistsRestartRefusal(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "example-stack", "apply")
	service := activity.NewRuntimeService(activity.NewActivityService(f.db, nil), refusalResolver{})
	generation, err := service.InitializeGeneration(t.Context())
	require.NoError(t, err)
	now := time.Now().UTC()
	require.NoError(t, f.db.Create(&environment.AgentRestartRequest{RunID: "example-restart", EnvironmentID: "0", RegistrationID: "example-registration", State: "reserved", CreatedAt: now, UpdatedAt: now, Deadline: now.Add(time.Minute)}).Error)
	f.p.ports.admit = func(ctx context.Context, env, target, key string, request dto.LifecycleRequest, b dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
		return service.Admit(ctx, activity.RuntimeCommand{Store: activity.RuntimeStoreBinding{EnvironmentID: env, RegistrationID: "example-registration", ManagerID: "example-manager", StoreGeneration: generation, ExecutorGeneration: generation}, Key: key, Actor: user.Actor{ID: "example-actor", Username: "Operator"}, Operation: activity.RuntimeUpdateProject, Target: activity.RuntimeTarget{Kind: "stack", ID: target}, Options: activity.RuntimeOptions{Lifecycle: &dto.LifecycleIntent{Request: request, Policy: &b, TargetIdentity: f.baseline.TargetIdentity, SavedRevision: f.baseline.SavedRevision, ObservedRevision: f.baseline.ObservedRevision, BaselineRevision: f.baseline.SavedRevision}}})
	}
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, "host.restarting", out.Targets[0].Code)
	var child policy.Child
	require.NoError(t, f.db.First(&child, "policy_id=?", one.ID).Error)
	require.Equal(t, "refused", child.Status)
	require.Equal(t, "host.restarting", child.Code)
	var n int64
	require.NoError(t, f.db.Table("runtime_receipts").Count(&n).Error)
	require.Zero(t, n)
	f.reopen(t)
	require.NoError(t, f.db.First(&child, "policy_id=?", one.ID).Error)
	require.Equal(t, "host.restarting", child.Code)
}
