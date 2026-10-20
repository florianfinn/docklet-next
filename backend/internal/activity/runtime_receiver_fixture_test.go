// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Core-only migrated SQLite fixtures, shared limiter and bounded joins; no HTTP claims.

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

type receiverFixture struct {
	*b1FixtureInternal
	receiver *RuntimeReceiver
}

func receiverNew(t *testing.T, executor b2ExecutorFunc) *receiverFixture {
	t.Helper()
	f := slotsFixture(t, t.Context(), new(1), executor)
	slotsCleanup(t, f)
	r, err := NewRuntimeReceiver(f.service)
	require.NoError(t, err)
	return &receiverFixture{f, r}
}
func receiverProbeFixture(t *testing.T) (*receiverFixture, *slotsProbe) {
	t.Helper()
	p := slotsNewProbe(t)
	f := receiverNew(t, func(ctx context.Context, _ RuntimeCommand, _ io.Writer) error { return p.run(ctx) })
	// Unblock before the fixture's join on failures, too.
	t.Cleanup(p.unblock)
	return f, p
}
func receiverEnvelope(t *testing.T, c RuntimeCommand) activitytypes.RuntimeExecutionEnvelope {
	t.Helper()
	c.Key = ""
	if c.ProposedRunID == "" {
		c.ProposedRunID = uuid.NewString()
	}
	canonical, err := runtimeCanonicalizeSemanticInternal(c)
	require.NoError(t, err)
	return activitytypes.RuntimeExecutionEnvelope{Identity: activitytypes.RuntimeRunIdentity{
		RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: c.Store.ManagerID,
			RegistrationID: c.Store.RegistrationID, ExecutorGeneration: c.Store.ExecutorGeneration},
		RunID: c.ProposedRunID, RequestDigest: canonical.digest}, Operation: c.Operation, Target: c.Target, Options: c.Options,
		Attribution: activitytypes.RuntimeAttribution{ID: c.Actor.ID, Username: c.Actor.Username, DisplayName: c.Actor.DisplayName}}
}
func receiverExecute(t *testing.T, f *receiverFixture, e activitytypes.RuntimeExecutionEnvelope) RuntimeReceiverResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	result, err := f.receiver.Execute(ctx, e)
	require.NoError(t, err)
	require.Equal(t, e.Identity, result.Identity)
	require.NotNil(t, result.Snapshot)
	require.Equal(t, e.Identity.RunID, result.Snapshot.ID)
	require.Equal(t, "0", result.Snapshot.EnvironmentID)
	return result
}
func receiverWait(t *testing.T, f *receiverFixture, e activitytypes.RuntimeExecutionEnvelope) activitytypes.RuntimeSnapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	snapshot, err := f.service.WaitLocal(ctx, RuntimeRunReference{Store: f.receiver.store, RunID: e.Identity.RunID})
	require.NoError(t, err)
	return snapshot
}
func receiverZero(t *testing.T, result RuntimeReceiverResult) {
	t.Helper()
	require.Equal(t, RuntimeReceiverResult{}, result, "failed transaction exposes no identity/evidence")
}
func receiverMismatch(t *testing.T, result RuntimeReceiverResult, err error, hidden string) {
	t.Helper()
	receiverZero(t, result)
	var conflict *runtimeConflictInternal
	require.ErrorAs(t, err, &conflict)
	require.NotContains(t, err.Error(), hidden)
}
func receiverCounts(t *testing.T, f *receiverFixture, receipts, activities, starts int64) {
	t.Helper()
	runtimeCount(t, f.store.svc.db.DB, &RuntimeReceipt{}, receipts)
	runtimeCount(t, f.store.svc.db.DB, &Activity{}, activities)
	runtimeCount(t, f.store.svc.db.DB, &RuntimeKeyBinding{}, 0)
	var count int64
	require.NoError(t, f.store.svc.db.Model(&ActivityMessage{}).Where("code = 'run.started'").Count(&count).Error)
	require.Equal(t, starts, count)
}
