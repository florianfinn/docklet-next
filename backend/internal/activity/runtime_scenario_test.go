// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Composed local lifecycle with repair, two-pool conflict and history regression.

package activity

import (
	"context"
	"io"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeScenarioCancelRepairTwoPoolsAndCompactRetry(t *testing.T) {
	entered, childDone, releaseChild := make(chan struct{}), make(chan struct{}), make(chan struct{})
	calls := 0
	f := b2Fixture(t, t.Context(), func(ctx context.Context, _ RuntimeCommand, output io.Writer) error {
		calls++ // only this controlled executor writes the test-owned counter
		_, err := output.Write([]byte("example progress\n"))
		if err != nil {
			return err
		}
		close(entered)
		<-ctx.Done()
		close(childDone) // a known child can still complete after cancellation
		<-releaseChild
		return ctx.Err()
	})
	c := f.command()
	admitted := b3Admit(t, f, c)
	b1Await(t, entered)
	original := b3Receipt(t, f, admitted)
	require.Equal(t, "executing", *original.ExecutorState)
	genericCount, err := f.store.svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.Zero(t, genericCount)
	require.Equal(t, original, b3Receipt(t, f, admitted))
	contender := f.contender(t, &b1ResolverInternal{})
	child := c
	child.Target = RuntimeTarget{Kind: "container", ID: "example-child", ParentProjectID: new(c.Target.ID)}
	child.Operation = RuntimeStartContainer
	child.Key = uuid.NewString()
	result, err := contender.Admit(t.Context(), child)
	b1Zero(t, result)
	var busy *GateConflict
	require.ErrorAs(t, err, &busy)
	cancelled, err := f.service.Cancel(t.Context(), b2Ref(c, admitted), c.Actor)
	require.NoError(t, err)
	require.True(t, cancelled.CancelRequested)
	b1Await(t, childDone)
	// Neither cancellation nor a generic repair attempt frees the receipt gate.
	result, err = contender.Admit(t.Context(), child)
	b1Zero(t, result)
	require.ErrorAs(t, err, &busy)
	require.Nil(t, b3Receipt(t, f, admitted).SettledAt)
	close(releaseChild)
	final, err := b2Wait(t, f, c, admitted)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusCancelled, final.Status)
	require.Equal(t, activitytypes.RuntimeOutcomeKnown, final.OutcomeState)
	require.True(t, final.CancelRequested)
	require.Equal(t, string(activitytypes.CodeRuntimeCancelled), final.MessageCode)
	b2FinalCount(t, f, admitted, 1)
	require.Equal(t, 1, calls)
	after := runtimeBoundarySnapshot(t, f.store)
	genericCount, err = f.store.svc.RepairStartup(t.Context())
	require.NoError(t, err)
	require.Zero(t, genericCount)
	require.Equal(t, after, runtimeBoundarySnapshot(t, f.store))
	_, err = f.store.svc.DeleteHistory(t.Context(), "0")
	require.NoError(t, err)
	f.store.now = f.store.now.Add(11 * time.Minute)
	f.resolver.resolve = func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error) {
		return RuntimeResolvedTarget{}, context.DeadlineExceeded
	}
	retained, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, admitted, retained.ActivityID)
	require.Equal(t, activitytypes.StatusCancelled, retained.Activity.Status)
	require.Equal(t, 1, calls)
	var activityCount int64
	require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", admitted).Count(&activityCount).Error)
	require.Zero(t, activityCount)
}
