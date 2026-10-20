// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Real handler/SQLite ownership and joined settlement activation regressions.

package activity

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationGenericOwnership(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(map[bool]string{false: "executing", true: "queued"}[queued], func(t *testing.T) {
			probe := slotsNewProbe(t)
			f := slotsFixture(t, t.Context(), new(1), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error { return probe.run(ctx) })
			slotsCleanup(t, f, probe)
			require.NoError(t, f.service.BindActivityOwner())
			var unblock func()
			if queued {
				var err error
				unblock, err = f.store.svc.limiter.acquireInternal(t.Context(), "0")
				require.NoError(t, err)
				defer unblock()
			}
			c := f.command()
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			if !queued {
				b1Await(t, probe.entered)
			}
			owner := b2Owner(t, f, admitted.ActivityID)
			// Even an owner token or startup-repair token cannot use generic writers.
			_, err = f.store.svc.CompleteActivity(owner.ctx, admitted.ActivityID, activitytypes.StatusSuccess, "false terminal", nil)
			require.ErrorIs(t, err, ErrRunNotOwned)
			require.ErrorIs(t, f.store.svc.AwaitActivitySlot(t.Context(), admitted.ActivityID, "0"), ErrRunNotOwned)
			_, err = f.store.svc.CancelActivity(t.Context(), "0", admitted.ActivityID, "untrusted name")
			require.ErrorIs(t, err, ErrRunNotOwned)

			mux := http.NewServeMux()
			api := humago.New(mux, huma.DefaultConfig("Activity ownership", "0.0.0"))
			api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				ps := authz.NewPermissionSet()
				ps.AddEnv("0", authz.PermActivitiesRead, authz.PermActivitiesCancel)
				request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, ps)
				request = context.WithValue(request, usertypes.CurrentUserContextKey{}, &c.Actor)
				next(huma.WithContext(ctx, request))
			})
			RegisterActivities(api, NewHandler(f.store.svc, EnvironmentDependencies{}))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/environments/0/activities/"+admitted.ActivityID+"/cancel", nil))
			require.Equal(t, 200, response.Code, response.Body.String())
			if !queued {
				// Executor observes intent but deliberately keeps its child alive.
				select {
				case <-owner.ctx.Done():
				case <-time.After(5 * time.Second):
					t.Fatal("runtime owner was not cancelled")
				}
				require.False(t, isTerminalActivityStatusInternal(runtimeBoundarySnapshot(t, f.store).Activities[0].Status))
				competing := c
				competing.Key, competing.Operation, competing.Options = uuid.NewString(), RuntimeDownProject, RuntimeOptions{}
				_, err = f.service.Admit(t.Context(), competing)
				var busy *GateConflict
				require.ErrorAs(t, err, &busy)
				f.store.svc.slotMu.Lock()
				_, held := f.store.svc.slotReleases[admitted.ActivityID]
				f.store.svc.slotMu.Unlock()
				require.True(t, held, "generic completion must not release a live executor slot")
			}
			probe.unblock()
			if queued {
				unblock()
			}
			snapshot, err := b2Wait(t, f, c, admitted.ActivityID)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusCancelled, snapshot.Status)
			if queued {
				require.Zero(t, probe.calls.Load(), "cancelled queue must never execute")
			} else {
				require.EqualValues(t, 1, probe.calls.Load())
			}
		})
	}
}

func TestRuntimeActivationLocalPendingKnowledgeAndRetainedParent(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	c.Operation, c.Options = RuntimeStartContainer, RuntimeOptions{}
	c.Target = RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-stack")}
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, activitytypes.RuntimeOutcomeKnown, admitted.Activity.OutcomeState)
	f.resolver.resolve = func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error) {
		t.Fatal("retained key inspected Docker")
		return RuntimeResolvedTarget{}, nil
	}
	request := c
	request.Target.ParentProjectID = nil
	retained, found, err := f.service.RetainedCommand(t.Context(), request)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, c.Target.ParentProjectID, retained.Target.ParentProjectID)
	joined, err := f.service.Admit(t.Context(), retained)
	require.NoError(t, err)
	require.Equal(t, admitted.ActivityID, joined.ActivityID)
	request.Actor.ID = "another-caller"
	_, _, err = f.service.RetainedCommand(t.Context(), request)
	var mismatch *runtimeConflictInternal
	require.ErrorAs(t, err, &mismatch)
	require.NotContains(t, err.Error(), admitted.ActivityID)
}

func TestRuntimeActivationMaintenanceRetriesFrozenResult(t *testing.T) {
	probe := slotsNewProbe(t)
	f := slotsFixture(t, t.Context(), new(1), func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error { return probe.run(ctx) })
	slotsCleanup(t, f, probe)
	c := f.command()
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	b1Await(t, probe.entered)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fixture_activation_settlement BEFORE UPDATE OF settled_at ON runtime_receipts WHEN NEW.settled_at IS NOT NULL BEGIN SELECT RAISE(ABORT, 'synthetic settlement fault'); END").Error)
	probe.unblock()
	_, err = b2Wait(t, f, c, admitted.ActivityID)
	require.Error(t, err)
	owner := b2Owner(t, f, admitted.ActivityID)
	require.NotNil(t, owner.result)
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER fixture_activation_settlement").Error)
	app, stop := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- f.service.runMaintenanceInternal(app, time.Millisecond, time.Hour) }()
	defer func() {
		stop()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("maintenance did not join")
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	// A terminal activity event is the commit barrier, not an immediate counter read.
	events, _, unsubscribe := f.store.svc.Subscribe("0")
	defer unsubscribe()
	for {
		f.store.svc.runningMu.Lock()
		_, retained := f.service.owners[admitted.ActivityID]
		f.store.svc.runningMu.Unlock()
		if !retained {
			break
		}
		select {
		case <-events:
		case <-ctx.Done():
			t.Fatal("retained result was not retried")
		}
	}
	snapshot, err := f.service.WaitLocal(ctx, b2Ref(c, admitted.ActivityID))
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
	require.EqualValues(t, 1, probe.calls.Load())
	b2FinalCount(t, f, admitted.ActivityID, 1)
}
