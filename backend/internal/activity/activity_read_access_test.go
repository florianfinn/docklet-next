// Source: docklet-next/issues/364
// Revision: e683d402f358e87b8eef5b81a71b530fd70cc0fe
// Retrieved: 2026-11-10
// License: Apache-2.0
// Deviations: Original receipt identity, missing target and scoped grant read guards.

package activity

import (
	"context"
	"errors"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestSingleActivityReadReceiptTargetGuards(t *testing.T) {
	f := b1Fixture(t)
	resolved := RuntimeResolvedTarget{Target: RuntimeTarget{Kind: "stack", ID: "assigned"}}
	var resolveErr error
	reads := 0
	_, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) { return f.command().Store, nil }, PrepareTarget: func(context.Context, string, RuntimeTarget) (RuntimeResolvedTarget, error) {
		reads++
		return resolved, resolveErr
	}})
	require.NoError(t, err)
	h := NewHandler(f.store.svc, EnvironmentDependencies{})
	receipt := &RuntimeReceipt{CallerID: new("owner"), TargetKind: new("stack"), TargetID: new("assigned")}
	ps := access.Synthesize("reader", []usertypes.AppAssignment{{EnvironmentID: "0", ProjectID: "assigned", Level: "restricted"}})
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, ps)
	require.True(t, h.canReadRuntimeActivityInternal(ctx, "0", receipt))
	resolved.Target.ID = "other"
	require.False(t, h.canReadRuntimeActivityInternal(ctx, "0", receipt), "a resolver cannot substitute another target")
	resolved.Target.ID, resolved.Target.Kind = "assigned", "container"
	require.False(t, h.canReadRuntimeActivityInternal(ctx, "0", receipt), "a resolver cannot substitute another kind")
	resolveErr = errors.New("target unavailable")
	require.False(t, h.canReadRuntimeActivityInternal(ctx, "0", receipt), "a failed current target read is denied")
	resolveErr = nil
	receipt.TargetKind = new("image")
	require.False(t, h.canReadRuntimeActivityInternal(ctx, "0", receipt), "unassigned resource types remain denied")
	receipt.TargetID = nil
	require.False(t, h.canReadRuntimeActivityInternal(ctx, "0", receipt), "missing targets remain denied")
	owner := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, access.Synthesize("owner", nil))
	before := reads
	require.True(t, h.canReadRuntimeActivityInternal(owner, "0", receipt), "persisted actor does not depend on target presence")
	require.Equal(t, before, reads)
	empty := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.NewPermissionSet())
	require.False(t, h.canReadRuntimeActivityInternal(empty, "0", receipt), "no user or grant cannot read")
	privileged := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
	require.True(t, h.canReadRuntimeActivityInternal(privileged, "0", receipt))
}
