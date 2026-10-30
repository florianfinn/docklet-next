// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original real admission/worker exclusion proof with controlled trusted parent metadata.

package activity

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRecreateAdmissionExcludesContainerParentAndPolicy(t *testing.T) {
	for _, operation := range []RuntimeOperation{RuntimeRecreateContainer, RuntimeRedeployContainer} {
		t.Run(string(operation), func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			var calls atomic.Int32
			f := b2Fixture(t, t.Context(), func(ctx context.Context, command RuntimeCommand, _ io.Writer) error {
				if calls.Add(1) == 1 {
					close(entered)
				}
				<-release
				containerDTO.RecordRecreateResult(ctx, containerDTO.RecreateResult{OldID: command.Target.ID, NewID: strings.Repeat("b", 64), OldName: "app", NewName: "app"})
				return nil
			})
			command := f.command()
			command.Operation = operation
			command.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64), Name: new("app"), ParentProjectID: new("parent-stack")}
			command.Options = RuntimeOptions{}
			if operation == RuntimeRecreateContainer {
				command.Options.Recreate = &containerDTO.Edit{}
			}
			// Parent metadata is a trusted resolver fixture. Ordinary M14 origin rules
			// refuse Compose edits; this exercises the persisted scope invariant itself.
			intake, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) { return command.Store, nil }, PrepareTarget: func(_ context.Context, _ string, target RuntimeTarget) (RuntimeResolvedTarget, error) {
				return RuntimeResolvedTarget{Target: target}, nil
			}, GuardContainer: func(context.Context, string, string) error { return nil }})
			require.NoError(t, err)
			intake.SetReady(true)
			admitted, err := f.service.Admit(t.Context(), command)
			require.NoError(t, err)
			b1Await(t, entered)
			restart := command
			restart.Key = uuid.NewString()
			restart.Operation = RuntimeRestartContainer
			restart.Options = RuntimeOptions{}
			_, err = f.service.Admit(t.Context(), restart)
			var busy *GateConflict
			require.ErrorAs(t, err, &busy)
			parent := command
			parent.Key = uuid.NewString()
			parent.Operation = RuntimeRestartProject
			parent.Target = RuntimeTarget{Kind: "stack", ID: "parent-stack"}
			parent.Options = RuntimeOptions{}
			_, err = f.service.Admit(t.Context(), parent)
			require.ErrorAs(t, err, &busy)
			ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
			ctx = context.WithValue(ctx, userDTO.CurrentUserContextKey{}, &command.Actor)
			_, err = f.store.svc.AdmitPolicyUpdate(ctx, "0", "parent-stack", uuid.NewString(), updaterDTO.LifecycleRequest{ExpectedRevision: strings.Repeat("a", 64)}, updaterDTO.PolicyBinding{PolicyID: "policy", ChildID: uuid.NewString(), Revision: 1, JobRunID: "job"})
			require.ErrorAs(t, err, &busy)
			require.EqualValues(t, 1, calls.Load())
			row := b3Receipt(t, f, admitted.ActivityID)
			require.Nil(t, row.SettledAt)
			require.Equal(t, "container:"+command.Target.ID, *row.GateScope)
			b2Owner(t, f, admitted.ActivityID)
			unblock()
			snapshot, err := b2Wait(t, f, command, admitted.ActivityID)
			require.NoError(t, err)
			require.Equal(t, "success", string(snapshot.Status))
		})
	}
}
