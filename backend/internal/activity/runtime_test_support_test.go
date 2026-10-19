// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Real migrated SQLite service fixtures and phase barriers, no executor stub.

package activity

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type b1ResolverInternal struct {
	calls   atomic.Int64
	resolve func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error)
}

func (r *b1ResolverInternal) ResolveRuntimeTarget(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
	r.calls.Add(1)
	if r.resolve != nil {
		return r.resolve(ctx, target)
	}
	return RuntimeResolvedTarget{Target: target, ComposeManaged: target.ParentProjectID != nil}, nil
}

type b1FixtureInternal struct {
	store    *runtimeFixtureInternal
	service  *RuntimeService
	resolver *b1ResolverInternal
}

func b1Fixture(t *testing.T) *b1FixtureInternal {
	t.Helper()
	f := runtimeFixture(t) // Production migration and lifetime owner, not AutoMigrate.
	resolver := &b1ResolverInternal{}
	service := NewRuntimeService(f.svc, resolver)
	generation, err := service.InitializeGeneration(t.Context())
	require.NoError(t, err)
	require.Equal(t, f.generation, generation)
	return &b1FixtureInternal{f, service, resolver}
}
func (f *b1FixtureInternal) command() RuntimeCommand {
	return RuntimeCommand{Store: RuntimeStoreBinding{StoreGeneration: f.store.generation,
		ExecutorGeneration: f.store.generation, ManagerID: "example-manager", RegistrationID: "example-registration", EnvironmentID: "0"},
		Key: uuid.NewString(), Actor: usertypes.Actor{ID: "example-owner", Username: "Operator", DisplayName: new("Original operator")},
		Operation: RuntimeDeployProject, Target: RuntimeTarget{Kind: "stack", ID: "example-stack", Name: new("Example stack")}}
}
func (f *b1FixtureInternal) contender(t *testing.T, resolver RuntimeTargetResolver) *RuntimeService {
	t.Helper()
	// Keep the original fixture/lifetime owner alive while another real pool and
	// ActivityService contend on the same file. No second lifetime lock is bypassed.
	svc := NewActivityService(&database.DB{DB: runtimeContender(t, f.store)}, nil)
	svc.clock = f.store.svc.clock
	return NewRuntimeService(svc, resolver)
}
func b1Zero(t *testing.T, result RuntimeAdmissionResult) {
	t.Helper()
	require.Equal(t, RuntimeAdmissionResult{}, result, "failed admission must return no identity or projection")
}
func b1Mismatch(t *testing.T, result RuntimeAdmissionResult, err error, hiddenID string) {
	t.Helper()
	b1Zero(t, result)
	var conflict *runtimeConflictInternal
	require.ErrorAs(t, err, &conflict)
	require.NotContains(t, err.Error(), hiddenID)
}
func b1Await(t *testing.T, channel <-chan struct{}) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(10 * time.Second):
		t.Fatal("phase barrier not reached")
	}
}
func b1Identity(t *testing.T, command RuntimeCommand, runID string) runtimeIdentityInternal {
	t.Helper()
	c, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	return c.identity(runID)
}
func b1Tx(t *testing.T, f *b1FixtureInternal, fn func(*gorm.DB, time.Time) error) {
	t.Helper()
	require.NoError(t, f.store.tx(fn))
}
