// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	dto "github.com/florianfinn/docklet-next/types/updater"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
)

func TestFullUserUpdateAdmissionUsesSupportingGrants(t *testing.T) {
	f := b2Fixture(t, t.Context(), func(context.Context, RuntimeCommand, io.Writer) error { return nil })
	c := imageCommand(f.command(), RuntimeUpdateProject)
	h, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) { return c.Store, nil }, PrepareTarget: func(context.Context, string, RuntimeTarget) (RuntimeResolvedTarget, error) {
		return RuntimeResolvedTarget{Target: c.Target}, nil
	}, PrepareLifecycle: func(context.Context, string, RuntimeTarget, RuntimeStoreBinding, dto.LifecycleRequest) (*dto.LifecycleIntent, error) {
		return c.Options.Lifecycle, nil
	}})
	require.NoError(t, err)
	h.SetReady(true)
	ps := access.Synthesize(c.Actor.ID, []userDTO.AppAssignment{{EnvironmentID: "0", ProjectID: c.Target.ID, Level: "full"}})
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, ps)
	ctx = context.WithValue(ctx, userDTO.CurrentUserContextKey{}, &c.Actor)
	accepted, _, err := h.admit(ctx, "0", c.Key, c.Operation, c.Target, RuntimeOptions{Lifecycle: &dto.LifecycleIntent{Request: c.Options.Lifecycle.Request}})
	require.NoError(t, err)
	require.NotEmpty(t, accepted.ActivityID)
}

func TestOwnRunUsesPersistedCallerAfterRestart(t *testing.T) {
	f := b1Fixture(t)
	command := f.command()
	admitted, err := f.service.Admit(t.Context(), command)
	require.NoError(t, err)
	// A new service reads the persisted initiating actor, without assignment or Docker reads.
	restarted := NewRuntimeService(f.store.svc, f.resolver)
	ctx := access.WithController(t.Context(), &access.Controller{RunOwner: func(ctx context.Context, env, run string) (string, error) {
		ref, found, err := restarted.Reference(ctx, env, run)
		require.True(t, found)
		if err != nil {
			return "", err
		}
		detail, err := restarted.GetRuntimeDetail(ctx, ref)
		if err != nil {
			return "", err
		}
		return detail.Activity.StartedBy.UserID, nil
	}})
	for _, id := range []string{command.Actor.ID, "different-viewer"} {
		ps := access.Synthesize(id, nil)
		userCtx := context.WithValue(ctx, middleware.ContextKeyUserPermissions, ps)
		_, err := runtimeHubReference(userCtx, restarted, "0", admitted.ActivityID, false)
		if id == command.Actor.ID {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, "access.not-found")
		}
	}
}
