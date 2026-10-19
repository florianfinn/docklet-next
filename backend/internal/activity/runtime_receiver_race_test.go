// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Deterministic core races through two real SQLite pools and one runtime lifetime owner.

package activity

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Both pools use the real driver/file. Alternating transaction origins exercises
// storage serialization without constructing a second RuntimeService/owner map.
type receiverPools struct {
	*sql.DB
	other *sql.DB
	next  atomic.Int64
}

func (p *receiverPools) GetDBConn() (*sql.DB, error) { return p.DB, nil }
func (p *receiverPools) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if p.next.Add(1)%2 == 0 {
		return p.other.BeginTx(ctx, opts)
	}
	return p.DB.BeginTx(ctx, opts)
}
func receiverTwoPools(t *testing.T, f *receiverFixture) *receiverPools {
	t.Helper()
	other := runtimeContender(t, f.store)
	second, err := other.DB()
	require.NoError(t, err)
	first, err := f.store.svc.db.DB.DB()
	require.NoError(t, err)
	pools := &receiverPools{DB: first, other: second}
	session := f.store.svc.db.Session(&gorm.Session{NewDB: true})
	session.Config.ConnPool = pools
	session.Statement.ConnPool = pools
	f.store.svc.db = &database.DB{DB: session}
	slotsCleanup(t, f.b1FixtureInternal)
	return pools
}

type receiverAnswer struct {
	result RuntimeReceiverResult
	err    error
}

func receiverAnswerWait(t *testing.T, done <-chan receiverAnswer) receiverAnswer {
	t.Helper()
	select {
	case got := <-done:
		return got
	case <-time.After(8 * time.Second):
		t.Fatal("receiver call did not join")
		return receiverAnswer{}
	}
}

func TestRuntimeReceiverSimultaneousSameUUIDTwoPools(t *testing.T) {
	f, p := receiverProbeFixture(t)
	pools := receiverTwoPools(t, f)
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	f.resolver.resolve = func(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return RuntimeResolvedTarget{}, ctx.Err()
		}
		return RuntimeResolvedTarget{Target: target}, nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	e := receiverEnvelope(t, f.command())
	done := make(chan receiverAnswer, 2)
	for range 2 {
		go func() { r, err := f.receiver.Execute(ctx, e); done <- receiverAnswer{r, err} }()
	}
	b1Await(t, entered)
	b1Await(t, entered) // Both passed real run-ID absence before resolution.
	unblock()
	for range 2 {
		got := receiverAnswerWait(t, done)
		require.NoError(t, got.err)
		require.Equal(t, e.Identity, got.result.Identity)
		require.Equal(t, e.Identity.RunID, got.result.Snapshot.ID)
	}
	b1Await(t, p.entered)
	require.GreaterOrEqual(t, pools.next.Load(), int64(4))
	require.EqualValues(t, 1, p.calls.Load())
	receiverCounts(t, f, 1, 1, 1)
	f.store.svc.runningMu.Lock()
	require.Len(t, f.service.owners, 1)
	f.store.svc.runningMu.Unlock()
	p.unblock()
	receiverWait(t, f, e)
	b2FinalCount(t, f.b1FixtureInternal, e.Identity.RunID, 1)
}

func TestRuntimeReceiverQueryExecuteDeterministicWinners(t *testing.T) {
	for _, winner := range []string{"query", "execute"} {
		t.Run(winner, func(t *testing.T) {
			f, p := receiverProbeFixture(t)
			receiverTwoPools(t, f)
			e := receiverEnvelope(t, f.command())
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			f.resolver.resolve = func(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return RuntimeResolvedTarget{}, ctx.Err()
				}
				return RuntimeResolvedTarget{Target: target}, nil
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			done := make(chan receiverAnswer, 1)
			go func() { r, err := f.receiver.Execute(ctx, e); done <- receiverAnswer{r, err} }()
			b1Await(t, entered)
			if winner == "execute" {
				unblock()
				got := receiverAnswerWait(t, done)
				require.NoError(t, got.err)
				b1Await(t, p.entered)
			}
			result, err := f.receiver.Query(ctx, e.Identity)
			require.NoError(t, err)
			require.Equal(t, e.Identity, result.Identity)
			if winner == "query" {
				require.Equal(t, activitytypes.RuntimeAnswerNotReceived, result.State)
				require.Nil(t, result.Snapshot)
				before := runtimeBoundarySnapshot(t, f.store)
				require.Equal(t, "not_received", before.Receipts[0].Kind)
				require.Equal(t, e.Identity.RequestDigest, before.Receipts[0].RequestDigest)
				unblock()
				got := receiverAnswerWait(t, done)
				receiverMismatch(t, got.result, got.err, e.Identity.RunID)
				require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
				again, err := f.receiver.Query(ctx, e.Identity)
				require.NoError(t, err)
				require.Equal(t, result, again)
				receiverCounts(t, f, 1, 0, 0)
				require.Zero(t, p.calls.Load())
			} else {
				require.Equal(t, activitytypes.RuntimeAnswerActive, result.State)
				require.NotNil(t, result.Snapshot)
				require.Equal(t, e.Identity.RunID, result.Snapshot.ID)
				p.unblock()
				receiverWait(t, f, e)
				result, err = f.receiver.Query(ctx, e.Identity)
				require.NoError(t, err)
				require.Equal(t, activitytypes.RuntimeAnswerTerminal, result.State)
				require.EqualValues(t, 1, p.calls.Load())
				receiverCounts(t, f, 1, 1, 1)
			}
		})
	}
}

func TestRuntimeReceiverCancelExecutingAndRepeated(t *testing.T) {
	f, p := receiverProbeFixture(t)
	receiverTwoPools(t, f)
	e := receiverEnvelope(t, f.command())
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	for range 2 {
		result, err := f.receiver.Cancel(t.Context(), e.Identity, activitytypes.RuntimeAttribution{ID: "example-canceller"})
		require.NoError(t, err)
		require.True(t, result.Snapshot.CancelRequested)
		require.Equal(t, activitytypes.RuntimeAnswerActive, result.State)
		slotsHeld(t, f.b1FixtureInternal, 1)
	}
	before := runtimeBoundarySnapshot(t, f.store)
	require.True(t, before.Receipts[0].CancelRequested)
	require.Nil(t, before.Receipts[0].SettledAt)
	require.Equal(t, activitytypes.StatusRunning, before.Activities[0].Status)
	p.unblock()
	require.Equal(t, activitytypes.StatusCancelled, receiverWait(t, f, e).Status)
	slotsHeld(t, f.b1FixtureInternal, 0)
	require.EqualValues(t, 1, p.calls.Load())
}
