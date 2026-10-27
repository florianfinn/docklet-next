// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Image-intent digest, parent exclusion, permission, settlement and no-replay barriers.

package activity

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func imageCommand(c RuntimeCommand, op RuntimeOperation) RuntimeCommand {
	c.Operation = op
	revision := strings.Repeat("a", 64)
	intent := &dto.LifecycleIntent{Request: dto.LifecycleRequest{ExpectedRevision: revision}, TargetIdentity: strings.Repeat("b", 64), SavedRevision: revision, ObservedRevision: strings.Repeat("c", 64), BaselineRevision: strings.Repeat("d", 64), Selections: []dto.ImageSelection{{Service: "app", SourceReference: "registry.example.com/app:1.0.0", Reference: "registry.example.com/app:1.1.0", Digest: "sha256:" + strings.Repeat("e", 64), ImageID: "sha256:" + strings.Repeat("f", 64), Platform: "linux/amd64", Kind: "version"}}}
	if op == RuntimeRestoreProject {
		intent.Request.PointID = uuid.NewString()
		intent.PointRevision = strings.Repeat("d", 64)
		intent.Selections[0].Kind = "restore"
	}
	c.Options = RuntimeOptions{Lifecycle: intent}
	return c
}
func TestRuntimeImageIntentFreezesDigestAndOwnedSelections(t *testing.T) {
	f := b1Fixture(t)
	for _, op := range []RuntimeOperation{RuntimeUpdateProject, RuntimeRestoreProject} {
		t.Run(string(op), func(t *testing.T) {
			c := imageCommand(f.command(), op)
			canonical, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			c.Options.Lifecycle.Selections[0].ImageID = "sha256:" + strings.Repeat("1", 64)
			require.NotEqual(t, c.Options.Lifecycle.Selections[0].ImageID, canonical.command.Options.Lifecycle.Selections[0].ImageID)
			changed, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			require.NotEqual(t, canonical.digest, changed.digest)
			c.Target.Kind = "container"
			_, err = runtimeCanonicalizeInternal(c)
			require.Error(t, err)
		})
	}
}
func TestRuntimeImageParentRestoreAndReceiptFirstRetry(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int32
	f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error {
		calls.Add(1)
		close(entered)
		<-release
		return nil
	})
	c := imageCommand(f.command(), RuntimeUpdateProject)
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	b1Await(t, entered)
	other := imageCommand(c, RuntimeRestoreProject)
	other.Key = uuid.NewString()
	_, err = f.service.Admit(t.Context(), other)
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)
	require.Equal(t, admitted.ActivityID, busy.RunID)
	child := c
	child.Key = uuid.NewString()
	child.Operation = RuntimeStartContainer
	child.Options = RuntimeOptions{}
	child.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64), ParentProjectID: &c.Target.ID}
	_, err = f.service.Admit(t.Context(), child)
	require.ErrorAs(t, err, &busy)
	retry := c
	retry.Options = RuntimeOptions{Lifecycle: &dto.LifecycleIntent{Request: c.Options.Lifecycle.Request}}
	retained, found, err := f.service.RetainedCommand(t.Context(), retry)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, c.Options, retained.Options)
	again, err := f.service.Admit(t.Context(), retained)
	require.NoError(t, err)
	require.Equal(t, admitted.ActivityID, again.ActivityID)
	require.EqualValues(t, 1, calls.Load())
	retry.Options.Lifecycle.Request.ReplaceConfiguration = true
	_, _, err = f.service.RetainedCommand(t.Context(), retry)
	require.Error(t, err)
	unblock()
	settled, err := b2Wait(t, f, c, admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusSuccess, settled.Status)
	_, err = f.store.svc.DeleteHistory(t.Context(), "0")
	require.NoError(t, err)
	retained, found, err = f.service.RetainedCommand(t.Context(), c)
	require.NoError(t, err)
	require.True(t, found)
	again, err = f.service.Admit(t.Context(), retained)
	require.NoError(t, err)
	require.Equal(t, admitted.ActivityID, again.ActivityID)
	require.EqualValues(t, 1, calls.Load())
}
func TestRuntimeImageStartupInterruptsWithoutReplay(t *testing.T) {
	for _, op := range []RuntimeOperation{RuntimeUpdateProject, RuntimeRestoreProject} {
		t.Run(string(op), func(t *testing.T) {
			f := b1Fixture(t)
			c := imageCommand(f.command(), op)
			run := b3Admit(t, f, c)
			b3Phase(t, f, c, run, "executing")
			require.NoError(t, f.store.svc.db.Exec("INSERT INTO image_lifecycle_runs(id,point_id,selected_point_id,had_effect,settled) VALUES(?,?,?,1,0)", run, "example-point", "").Error)
			var calls atomic.Int32
			require.NoError(t, f.service.ConfigureLocal(RuntimeLocalConfig{ApplicationContext: t.Context(), Store: c.Store, Executor: b2ExecutorFunc(func(context.Context, RuntimeCommand, io.Writer) error { calls.Add(1); return nil })}))
			require.NoError(t, f.service.InitializeLocalStartup(t.Context(), c.Store))
			receipt := b3Receipt(t, f, run)
			require.Equal(t, "failed", *receipt.ExecutorState)
			require.Equal(t, "runtime.interrupted", *receipt.ExecutorResultCode)
			require.NotNil(t, receipt.SettledAt)
			var row struct{ Settled bool }
			require.NoError(t, f.store.svc.db.Table("image_lifecycle_runs").Where("id = ?", run).Take(&row).Error)
			require.True(t, row.Settled)
			same, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			require.Equal(t, run, same.ActivityID)
			require.Zero(t, calls.Load(), "startup and retry may not replay image effects")
		})
	}
}
func imageGrantContext(actor user.Actor, missing string) context.Context {
	ps := authz.NewPermissionSet()
	for _, grant := range []string{authz.PermProjectsDeploy, authz.PermProjectsUpdate, authz.PermImagesPull, authz.PermImageUpdatesCheck, authz.PermActivitiesRead} {
		if grant != missing {
			ps.AddEnv("0", grant)
		}
	}
	ctx := context.WithValue(context.Background(), middleware.ContextKeyUserPermissions, ps)
	return context.WithValue(ctx, user.CurrentUserContextKey{}, &actor)
}
func TestRuntimeImagePermissionsPrecedeFreezeAndRetainedJoins(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var effects, freezes atomic.Int32
	f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error {
		effects.Add(1)
		close(entered)
		<-release
		return nil
	})
	c := imageCommand(f.command(), RuntimeUpdateProject)
	h, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) { return c.Store, nil }, PrepareTarget: func(context.Context, string, RuntimeTarget) (RuntimeResolvedTarget, error) {
		return RuntimeResolvedTarget{Target: c.Target}, nil
	}, PrepareLifecycle: func(context.Context, string, RuntimeTarget, RuntimeStoreBinding, dto.LifecycleRequest) (*dto.LifecycleIntent, error) {
		freezes.Add(1)
		return c.Options.Lifecycle, nil
	}})
	require.NoError(t, err)
	h.SetReady(true)
	seed := RuntimeOptions{Lifecycle: &dto.LifecycleIntent{Request: c.Options.Lifecycle.Request}}
	for _, missing := range []string{authz.PermProjectsDeploy, authz.PermProjectsUpdate, authz.PermImagesPull, authz.PermImageUpdatesCheck, authz.PermActivitiesRead} {
		_, _, err = h.admit(imageGrantContext(c.Actor, missing), "0", c.Key, c.Operation, c.Target, seed)
		var denied huma.StatusError
		require.ErrorAs(t, err, &denied)
		require.Equal(t, 403, denied.GetStatus())
		require.Zero(t, freezes.Load())
		require.Zero(t, effects.Load())
	}
	for _, key := range []string{"", "invalid-key"} {
		_, _, err = h.admit(imageGrantContext(c.Actor, ""), "0", key, c.Operation, c.Target, seed)
		require.Error(t, err)
		require.Zero(t, freezes.Load())
		require.Zero(t, effects.Load())
	}
	accepted, _, err := h.admit(imageGrantContext(c.Actor, ""), "0", c.Key, c.Operation, c.Target, seed)
	require.NoError(t, err)
	b1Await(t, entered)
	require.EqualValues(t, 1, freezes.Load())
	_, _, err = h.admit(imageGrantContext(c.Actor, authz.PermImagesPull), "0", c.Key, c.Operation, c.Target, seed)
	require.Error(t, err)
	require.EqualValues(t, 1, freezes.Load())
	replacement := c.Actor
	replacement.ID = "different-actor"
	_, _, err = h.admit(imageGrantContext(replacement, ""), "0", c.Key, c.Operation, c.Target, seed)
	require.Error(t, err)
	require.EqualValues(t, 1, freezes.Load())
	unblock()
	result, err := b2Wait(t, f, c, accepted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusSuccess, result.Status)
	require.EqualValues(t, 1, effects.Load())
}
func TestRuntimeImageResultSettlementIsAtomicAndRetryNeverExecutes(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	f := b2Fixture(t, t.Context(), func(ctx context.Context, c RuntimeCommand, _ io.Writer) error {
		calls.Add(1)
		dto.RecordLifecycleResult(ctx, dto.LifecycleResult{RunID: c.ProposedRunID, RestorePointID: "example-point", Verified: true})
		close(entered)
		<-release
		return nil
	})
	c := imageCommand(f.command(), RuntimeUpdateProject)
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	b1Await(t, entered)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER image_result_fault BEFORE UPDATE OF metadata ON activities BEGIN SELECT RAISE(ABORT,'synthetic image result write'); END").Error)
	close(release)
	_, err = b2Wait(t, f, c, admitted.ActivityID)
	require.Error(t, err)
	receipt := b3Receipt(t, f, admitted.ActivityID)
	require.Nil(t, receipt.SettledAt)
	require.Nil(t, receipt.ExecutorSettledAt)
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER image_result_fault").Error)
	result, err := f.service.RetrySettlement(t.Context(), b2Ref(c, admitted.ActivityID))
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusSuccess, result.Status)
	require.NotNil(t, result.ImageLifecycle)
	require.True(t, result.ImageLifecycle.Verified)
	require.EqualValues(t, 1, calls.Load())
}

func TestRuntimeImageParentExclusionPreventsExtraEffects(t *testing.T) {
	for _, direction := range []string{"update-restore", "stack-child", "child-stack"} {
		t.Run(direction, func(t *testing.T) {
			first, second, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var calls atomic.Int32
			f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error {
				switch calls.Add(1) {
				case 1:
					close(first)
				case 2:
					close(second)
				}
				<-release
				return nil
			})
			base := f.command()
			a := imageCommand(base, RuntimeUpdateProject)
			b := imageCommand(base, RuntimeRestoreProject)
			b.Key = uuid.NewString()
			child := base
			child.Key = uuid.NewString()
			child.Operation = RuntimeStartContainer
			child.Options = RuntimeOptions{}
			child.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64), ParentProjectID: &base.Target.ID}
			if direction == "stack-child" {
				b = child
			}
			if direction == "child-stack" {
				a = child
			}
			admitted, err := f.service.Admit(t.Context(), a)
			require.NoError(t, err)
			b1Await(t, first)
			_, err = f.service.Admit(t.Context(), b)
			if err == nil {
				b1Await(t, second)
			}
			require.EqualValues(t, 1, calls.Load(), "parent exclusion must prevent a second execution effect")
			var busy *GateConflict
			require.ErrorAs(t, err, &busy)
			require.Equal(t, admitted.ActivityID, busy.RunID)
			unblock()
			_, err = b2Wait(t, f, a, admitted.ActivityID)
			require.NoError(t, err)
		})
	}
}
