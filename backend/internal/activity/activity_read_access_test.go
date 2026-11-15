// Source: docklet-next/issues/364
// Revision: e683d402f358e87b8eef5b81a71b530fd70cc0fe
// Retrieved: 2026-11-10
// License: Apache-2.0
// Deviations: Original receipt identity, missing target and scoped grant read guards; non-disclosing receipt-storage failures with warning evidence.

package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

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

// Other fixture workers can log while the package runs; capture complete writes.
type activityReadLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *activityReadLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}
func (b *activityReadLogBuffer) snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.Buffer.Bytes())
}

func TestSingleActivityReceiptStorageFailureDoesNotDiscloseRun(t *testing.T) {
	f := b1Fixture(t)
	require.NoError(t, f.service.BindActivityOwner())
	command := f.command()
	admitted, err := f.service.Admit(t.Context(), command)
	require.NoError(t, err)
	h := NewHandler(f.store.svc, EnvironmentDependencies{})
	userCtx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, access.Synthesize("reader", nil))
	ownerCtx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, access.Synthesize(command.Actor.ID, nil))
	adminCtx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
	input := &GetActivityInput{EnvironmentID: "0", ActivityID: admitted.ActivityID, Limit: 500}
	// Both an existing denied run and an absent run have the same public error.
	readError := func(t *testing.T, ctx context.Context, input *GetActivityInput, status int) []byte {
		t.Helper()
		out, err := h.GetActivity(ctx, input)
		require.Nil(t, out)
		var problem huma.StatusError
		require.ErrorAs(t, err, &problem)
		require.Equal(t, status, problem.GetStatus(), "receipt read status must not disclose a run")
		encoded, marshalErr := json.Marshal(err)
		require.NoError(t, marshalErr)
		return encoded
	}
	unknown := readError(t, userCtx, &GetActivityInput{EnvironmentID: "0", ActivityID: uuid.NewString()}, 404)
	denied := readError(t, userCtx, input, 404)
	require.JSONEq(t, string(unknown), string(denied))
	detail, err := h.GetActivity(ownerCtx, input)
	require.NoError(t, err, "the healthy stored run remains readable by its owner")
	require.NotNil(t, detail)

	// Reference reads only the receipt; the subsequent transaction validates the
	// current store generation, as it would after replacement of the store.
	require.NoError(t, f.store.svc.db.Model(&RuntimeStoreGeneration{}).Where("id = 1").Update("generation", uuid.NewString()).Error)
	ref, found, err := f.service.Reference(t.Context(), "0", admitted.ActivityID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, admitted.ActivityID, ref.RunID)
	var logs activityReadLogBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		status int
	}{
		{"simple user", userCtx, 404}, {"owner", ownerCtx, 404}, {"administrator", adminCtx, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := len(logs.snapshot())
			problem := readError(t, tc.ctx, input, tc.status)
			if tc.status == 404 {
				require.JSONEq(t, string(unknown), string(problem), "storage failures, unknown runs and denied runs must be indistinguishable")
			} else {
				require.Contains(t, string(problem), "Runtime evidence unavailable")
			}
			type record struct {
				Level string
				Msg   string
				Error string
			}
			var warnings []record
			for _, line := range strings.Split(strings.TrimSpace(string(logs.snapshot()[before:])), "\n") {
				var entry record
				if json.Unmarshal([]byte(line), &entry) == nil && entry.Msg == "failed to load runtime activity receipt" {
					warnings = append(warnings, entry)
				}
			}
			require.Len(t, warnings, 1)
			require.Equal(t, "WARN", warnings[0].Level)
			require.NotEmpty(t, warnings[0].Error)
		})
	}
	// Failure before Reference establishes existence keeps its existing status.
	const callback = "test:runtime-reference-unavailable"
	require.NoError(t, f.store.svc.db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		tx.AddError(errors.New("reference storage unavailable"))
	}))
	defer func() { require.NoError(t, f.store.svc.db.Callback().Query().Remove(callback)) }()
	for _, ctx := range []context.Context{userCtx, adminCtx} {
		readError(t, ctx, input, 503)
	}
}
