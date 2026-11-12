// Source: docklet-next/issues/441
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original real admission and terminal transactions across suppression windows and reconciliation.

package updater

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	a "github.com/florianfinn/docklet-next/types/activity"
	image "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

type suppressionExecutor struct {
	outcome, code string
	applied       []dto.AppliedCandidate
	calls         int
	// refuseSuppressed mirrors the container executor's scheduled candidate check.
	refuseSuppressed bool
}

func (e *suppressionExecutor) ExecuteRuntime(ctx context.Context, c activity.RuntimeCommand, _ io.Writer) error {
	e.calls++
	if e.refuseSuppressed && c.Options.ContainerImage != nil && c.Options.ContainerImage.Policy != nil && slices.Contains(c.Options.ContainerImage.Policy.Suppressed, e.applied[0]) {
		dto.RecordLifecycleResult(ctx, dto.LifecycleResult{RunID: c.ProposedRunID, Code: "update.candidate-failed", HTTPStatus: 409, Recovery: &dto.RecoveryResult{Outcome: "not-needed"}})
		return image.Failure("update.candidate-failed", 409)
	}
	result := dto.LifecycleResult{RunID: c.ProposedRunID, Code: e.code, HTTPStatus: 500, Applied: e.applied, Recovery: &dto.RecoveryResult{Outcome: e.outcome}}
	if e.code == "" {
		result.Verified, result.HTTPStatus, result.Recovery.Outcome = true, 0, "not-needed"
	}
	dto.RecordLifecycleResult(ctx, result)
	if e.code != "" {
		return image.Failure(e.code, 500)
	}
	return nil
}

type suppressionRuntime struct {
	f        *policyFixture
	service  *activity.RuntimeService
	store    activity.RuntimeStoreBinding
	executor *suppressionExecutor
	bindings []dto.PolicyBinding
}

func newSuppressionRuntime(t *testing.T, f *policyFixture) *suppressionRuntime {
	t.Helper()
	as := activity.NewActivityService(f.db, nil)
	setup := activity.NewRuntimeService(as, refusalResolver{})
	generation, err := setup.InitializeGeneration(t.Context())
	require.NoError(t, err)
	store := activity.RuntimeStoreBinding{EnvironmentID: "0", RegistrationID: "example-registration", ManagerID: "example-manager", StoreGeneration: generation, ExecutorGeneration: generation}
	executor := &suppressionExecutor{code: "update.unstable", outcome: "restored", applied: []dto.AppliedCandidate{{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("f", 64)}}}
	service := activity.NewRuntimeService(as, refusalResolver{}, activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: store, Executor: executor})
	_, err = activity.NewRuntimeHTTPService(service, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return store, nil }, GuardContainer: func(context.Context, string, string) error { return nil }, PrepareTarget: func(_ context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
		return activity.RuntimeResolvedTarget{Target: target}, nil
	}})
	require.NoError(t, err)
	r := &suppressionRuntime{f: f, service: service, store: store, executor: executor}
	f.p.ports.admit = func(ctx context.Context, env, target, key string, request dto.LifecycleRequest, binding dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
		r.bindings = append(r.bindings, binding)
		// Stack executor preflight double: resolve the final manifest, never the
		// native comparison digest reported by the earlier update check.
		for _, suppressed := range binding.Suppressed {
			if suppressed == executor.applied[0] {
				return activity.RuntimeAdmissionResult{}, image.Failure("update.candidate-failed", 409)
			}
		}
		return service.Admit(ctx, activity.RuntimeCommand{Store: store, Key: key, ProposedRunID: binding.ChildID, Actor: user.Actor{ID: "example-actor", Username: "Operator"}, Operation: activity.RuntimeUpdateProject, Target: activity.RuntimeTarget{Kind: "stack", ID: target, Name: new(target)}, Options: activity.RuntimeOptions{Lifecycle: &dto.LifecycleIntent{Request: request, Policy: &binding, TargetIdentity: f.baseline.TargetIdentity, SavedRevision: f.baseline.SavedRevision, ObservedRevision: f.baseline.ObservedRevision, BaselineRevision: f.baseline.SavedRevision}}})
	}
	f.p.ports.wait = func(ctx context.Context, _, id string) (a.RuntimeSnapshot, error) {
		return service.WaitLocal(ctx, activity.RuntimeRunReference{Store: store, RunID: id})
	}
	f.p.ports.snapshot = func(ctx context.Context, env, id string) (a.RuntimeSnapshot, bool, error) {
		return as.PolicyUpdateSnapshot(ctx, env, id)
	}
	return r
}

func TestPolicySuppressionThreeWindowsAndRestart(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "example-stack", "scheduled")
	r := newSuppressionRuntime(t, f)
	first := f.run(t, "auto-update", st.Run{ID: "window-one"})
	require.Len(t, first.Targets, 1)
	require.Equal(t, "update.auto-restored", first.Targets[0].Code)
	failed, err := f.p.store.FailedCandidates(t.Context(), one.ID, one.TargetIdentity)
	require.NoError(t, err)
	require.Equal(t, r.executor.applied, failed)
	require.NotEqual(t, f.check.ServiceUpdates["app"].UpdateInfo.LatestDigest, failed[0].Manifest, "classic comparison key is not the pullable manifest")
	for _, window := range []string{"window-two", "window-three"} {
		f.reopen(t)
		r = newSuppressionRuntime(t, f)
		out := f.run(t, "auto-update", st.Run{ID: window})
		require.Len(t, out.Targets, 1)
		require.Equal(t, st.Succeeded, out.Status)
		require.Equal(t, st.Skipped, out.Targets[0].Status)
		require.Equal(t, "update.candidate-failed", out.Targets[0].Code)
		require.Equal(t, failed, r.bindings[0].Suppressed)
		require.Zero(t, r.executor.calls)
	}
	var notices []notificationoutbox.Row
	require.NoError(t, f.db.Find(&notices).Error)
	require.Len(t, notices, 1, "skipped windows do not notify")
	require.Equal(t, "auto-restored", notices[0].Payload["reason"])
}

func TestPolicySuppressionEverySettlementCaller(t *testing.T) {
	for _, path := range []string{"wait", "host-hold", "restart-reconcile", "lost-terminal-response"} {
		t.Run(path, func(t *testing.T) {
			f := newPolicyFixture(t)
			one := f.save(t, "example-stack", "scheduled")
			r := newSuppressionRuntime(t, f)
			if path == "lost-terminal-response" {
				admit := f.p.ports.admit
				f.p.ports.admit = func(ctx context.Context, env, target, key string, req dto.LifecycleRequest, b dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
					accepted, err := admit(ctx, env, target, key, req, b)
					require.NoError(t, err)
					_, err = r.service.WaitLocal(ctx, activity.RuntimeRunReference{Store: r.store, RunID: accepted.ActivityID})
					require.NoError(t, err)
					return accepted, errors.New("synthetic lost response")
				}
			} else if path != "wait" {
				wait := f.p.ports.wait
				f.p.ports.wait = func(ctx context.Context, env, id string) (a.RuntimeSnapshot, error) {
					_, err := wait(ctx, env, id)
					require.NoError(t, err)
					return a.RuntimeSnapshot{}, errors.New("synthetic loss after terminal before child settlement")
				}
			}
			out := f.run(t, "auto-update", st.Run{ID: "initial-window"})
			if path == "host-hold" || path == "restart-reconcile" {
				require.Equal(t, "runtime.outcome-unknown", out.Targets[0].Code)
				f.reopen(t)
				r = newSuppressionRuntime(t, f)
				// A later policy rebind must not change the admitted intent's identity.
				require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", one.ID).Update("target_identity", strings.Repeat("b", 64)).Error)
				if path == "host-hold" {
					hold, err := f.s.imagePolicyHostHold(t.Context(), f.p, "0", "example-registration")
					require.NoError(t, err)
					require.Nil(t, hold)
				} else {
					_, err := f.s.ReconcileImagePolicyHost(t.Context(), "0", st.Run{ID: "initial-window"})
					require.NoError(t, err)
				}
			}
			rows, err := f.p.store.FailedCandidates(t.Context(), one.ID, one.TargetIdentity)
			require.NoError(t, err)
			require.Equal(t, r.executor.applied, rows)
			var child policy.Child
			require.NoError(t, f.db.First(&child, "policy_id = ?", one.ID).Error)
			require.Equal(t, "settled", child.Status)
			require.NotNil(t, child.SettledAt)
			require.Equal(t, "update.auto-restored", child.Code)
			// Reconciliation is idempotent after settlement.
			_, err = f.s.ReconcileImagePolicyHost(t.Context(), "0", st.Run{ID: "initial-window"})
			require.NoError(t, err)
			rows, err = f.p.store.FailedCandidates(t.Context(), one.ID, one.TargetIdentity)
			require.NoError(t, err)
			require.Equal(t, r.executor.applied, rows)
		})
	}
}

func TestPolicySuppressionWriteFailureRetainsAdmission(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "example-stack", "scheduled")
	r := newSuppressionRuntime(t, f)
	require.NoError(t, f.db.Exec("CREATE TRIGGER failed_candidate_fault BEFORE INSERT ON image_policy_failed_candidates BEGIN SELECT RAISE(ABORT,'synthetic candidate write failure'); END").Error)
	out := f.run(t, "auto-update", st.Run{ID: "window"})
	require.Equal(t, "update.persistence-failed", out.Targets[0].Code)
	var child policy.Child
	require.NoError(t, f.db.First(&child, "policy_id = ?", one.ID).Error)
	require.Equal(t, "admitted", child.Status)
	var count int64
	require.NoError(t, f.db.Model(&policy.FailedCandidate{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, f.db.Exec("DROP TRIGGER failed_candidate_fault").Error)
	hold, err := f.s.imagePolicyHostHold(t.Context(), f.p, "0", "example-registration")
	require.NoError(t, err)
	require.Nil(t, hold)
	rows, err := f.p.store.FailedCandidates(t.Context(), one.ID, one.TargetIdentity)
	require.NoError(t, err)
	require.Equal(t, r.executor.applied, rows)
}

func TestPolicySuppressionCurrentIdentityAndFullBinding(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "example-stack", "scheduled")
	r := newSuppressionRuntime(t, f)
	for i := 0; i < dto.MaxSuppressedCandidates; i++ {
		tuple := r.executor.applied[0]
		tuple.Service = strings.Repeat("s", i+1)
		require.NoError(t, f.db.Create(&policy.FailedCandidate{PolicyID: one.ID, TargetIdentity: one.TargetIdentity, Service: tuple.Service, Repository: tuple.Repository, Platform: tuple.Platform, Manifest: tuple.Manifest, RunID: "earlier", CreatedAt: *policyNow()}).Error)
	}
	other := r.executor.applied[0]
	require.NoError(t, f.db.Create(&policy.FailedCandidate{PolicyID: one.ID, TargetIdentity: "other-identity", Service: other.Service, Repository: other.Repository, Platform: other.Platform, Manifest: other.Manifest, RunID: "earlier-other", CreatedAt: *policyNow()}).Error)
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, "update.auto-restored", out.Targets[0].Code)
	require.Len(t, r.bindings[0].Suppressed, dto.MaxSuppressedCandidates)
	for _, tuple := range r.bindings[0].Suppressed {
		require.NotEqual(t, "app", tuple.Service)
	}
	var retained activity.RuntimeImageIntent
	require.NoError(t, f.db.First(&retained, "run_id = ?", out.Targets[0].ActivityID).Error)
	var intent dto.LifecycleIntent
	require.NoError(t, json.Unmarshal([]byte(retained.Payload), &intent))
	require.Len(t, intent.Policy.Suppressed, dto.MaxSuppressedCandidates)
}

func TestPolicyContainerSuppressionRecordThenSkip(t *testing.T) {
	f := newPolicyFixture(t)
	one, err := f.p.store.Policy(t.Context(), "0", "example-registration", "app", "container")
	require.NoError(t, err)
	one.Mode, one.ActorID, one.TargetIdentity = "scheduled", "example-actor", "app"
	one.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
	require.NoError(t, f.p.store.SavePolicy(t.Context(), one, 0))
	one.Revision = 1
	r := newSuppressionRuntime(t, f)
	r.executor.applied[0].Service = ""
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "container", ID: "app", Name: "app", ContainerID: "docker-id", Eligible: true}}, nil
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) { return f.check, nil }
	f.p.ports.admitContainer = func(ctx context.Context, env, target, key string, b dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
		r.bindings = append(r.bindings, b)
		for _, tuple := range b.Suppressed {
			if tuple == r.executor.applied[0] {
				return activity.RuntimeAdmissionResult{}, image.Failure("update.candidate-failed", 409)
			}
		}
		return r.service.Admit(ctx, activity.RuntimeCommand{Store: r.store, Key: key, ProposedRunID: b.ChildID, Actor: user.Actor{ID: "example-actor", Username: "Operator"}, Operation: activity.RuntimeUpdateContainer, Target: activity.RuntimeTarget{Kind: "container", ID: target, Name: new("app")}, Options: activity.RuntimeOptions{ContainerImage: &dto.ContainerImageRequest{Policy: &b}}})
	}
	first := f.run(t, "auto-update", st.Run{ID: "container-one"})
	require.Equal(t, "update.auto-restored", first.Targets[0].Code)
	rows, err := f.p.store.FailedCandidates(t.Context(), one.ID, "app")
	require.NoError(t, err)
	require.Equal(t, r.executor.applied, rows)
	second := f.run(t, "auto-update", st.Run{ID: "container-two"})
	require.Equal(t, st.Succeeded, second.Status)
	require.Equal(t, st.Skipped, second.Targets[0].Status)
	require.Equal(t, "update.candidate-failed", second.Targets[0].Code)
	require.Equal(t, 1, r.executor.calls)
	require.Equal(t, rows, r.bindings[1].Suppressed)
}

// The real container skip is an executor refusal after admission: a terminal
// failed run that settles Skipped without a notice or a new failed row.
func TestPolicyContainerExecutorSkipSettlesEndToEnd(t *testing.T) {
	f := newPolicyFixture(t)
	one, err := f.p.store.Policy(t.Context(), "0", "example-registration", "app", "container")
	require.NoError(t, err)
	one.Mode, one.ActorID, one.TargetIdentity = "scheduled", "example-actor", "app"
	one.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
	require.NoError(t, f.p.store.SavePolicy(t.Context(), one, 0))
	r := newSuppressionRuntime(t, f)
	r.executor.applied[0].Service = ""
	r.executor.refuseSuppressed = true
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "container", ID: "app", Name: "app", ContainerID: "docker-id", Eligible: true}}, nil
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) { return f.check, nil }
	f.p.ports.admitContainer = func(ctx context.Context, env, target, key string, b dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
		r.bindings = append(r.bindings, b)
		return r.service.Admit(ctx, activity.RuntimeCommand{Store: r.store, Key: key, ProposedRunID: b.ChildID, Actor: user.Actor{ID: "example-actor", Username: "Operator"}, Operation: activity.RuntimeUpdateContainer, Target: activity.RuntimeTarget{Kind: "container", ID: target, Name: new("app")}, Options: activity.RuntimeOptions{ContainerImage: &dto.ContainerImageRequest{Policy: &b}}})
	}
	first := f.run(t, "auto-update", st.Run{ID: "container-one"})
	require.Equal(t, "update.auto-restored", first.Targets[0].Code)
	second := f.run(t, "auto-update", st.Run{ID: "container-two"})
	require.Equal(t, 2, r.executor.calls, "the skip is decided by the executor, not admission")
	require.Equal(t, st.Succeeded, second.Status)
	require.Equal(t, st.Skipped, second.Targets[0].Status)
	require.Equal(t, "update.candidate-failed", second.Targets[0].Code)
	var children []policy.Child
	require.NoError(t, f.db.Find(&children, "policy_id = ?", one.ID).Error)
	require.Len(t, children, 2)
	codes := map[string]string{}
	for _, child := range children {
		require.Equal(t, "settled", child.Status)
		codes[child.Code] = child.RuntimeID
	}
	require.Equal(t, second.Targets[0].ActivityID, codes["update.candidate-failed"])
	rows, err := f.p.store.FailedCandidates(t.Context(), one.ID, "app")
	require.NoError(t, err)
	require.Equal(t, r.executor.applied, rows)
	var notices []notificationoutbox.Row
	require.NoError(t, f.db.Find(&notices).Error)
	require.Len(t, notices, 1, "the executor skip does not notify")
	require.Equal(t, "auto-restored", notices[0].Payload["reason"])
}

func TestPolicySuppressionReadFailureLeavesNoChild(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "example-stack", "scheduled")
	newSuppressionRuntime(t, f)
	require.NoError(t, f.db.Exec("DROP TABLE image_policy_failed_candidates").Error)
	out := f.run(t, "auto-update", st.Run{ID: "window"})
	require.Equal(t, "update.persistence-failed", out.Targets[0].Code)
	var count int64
	require.NoError(t, f.db.Model(&policy.Child{}).Count(&count).Error)
	require.Zero(t, count, "no prepared child outlives a failed suppression read")
}

func TestPolicyManualCheckDoesNotClearFailedCandidate(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "example-stack", "check")
	tuple := policy.FailedCandidate{PolicyID: one.ID, TargetIdentity: one.TargetIdentity, Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64), RunID: "previous", CreatedAt: *policyNow()}
	require.NoError(t, f.db.Create(&tuple).Error)
	_, err := f.s.CheckPolicyProject(t.Context(), "0", one.TargetID)
	require.NoError(t, err)
	rows, err := f.p.store.FailedCandidates(t.Context(), one.ID, one.TargetIdentity)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Zero(t, f.admissions)
}
