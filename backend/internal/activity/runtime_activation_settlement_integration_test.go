// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Real SQLite writer contention and actual Fx settlement retry driver.

package activity_test

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Observe errors from the real pool without changing its SQLite behavior. The
// third Busy return is the initial settlement attempt's retry-exhaustion barrier.
type activationObservedPool struct {
	*sql.DB
	armed     atomic.Bool
	busy      atomic.Int32
	exhausted chan struct{}
}

func (p *activationObservedPool) GetDBConn() (*sql.DB, error) { return p.DB, nil }
func (p *activationObservedPool) BeginTx(ctx context.Context, options *sql.TxOptions) (*sql.Tx, error) {
	tx, err := p.DB.BeginTx(ctx, options)
	if p.armed.Load() && dbutil.IsRetryableWriteError(err) && p.busy.Add(1) == 3 {
		close(p.exhausted)
	}
	return tx, err
}

func TestRuntimeActivationFxRetriesSettlementAfterSQLiteContention(t *testing.T) {
	var observed *activationObservedPool
	hub := activationApp(t, false, "", activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
		pool, err := f.db.SQLDB()
		require.NoError(t, err)
		observed = &activationObservedPool{DB: pool, exhausted: make(chan struct{})}
		f.db.Config.ConnPool = observed
		f.db.Statement.ConnPool = observed
	}})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	hub.services.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start") {
			once.Do(func() { close(entered) })
			<-release
		}
		return false
	}
	status, body := activationCall(t, hub, "/api/environments/0/containers/"+runtimeContainerID+"/start", uuid.NewString(), "respond-async", nil)
	require.Equal(t, 202, status, string(body))
	run := activationRunID(t, body)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual SDK executor did not enter")
	}
	contender, err := sqlitestore.Open(t.Context(), hub.services.db.Path(), false, false)
	require.NoError(t, err)
	defer contender.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	connection, err := contender.Conn(ctx)
	require.NoError(t, err)
	defer connection.Close()
	_, err = connection.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.NoError(t, err)
	locked := true
	defer func() {
		if locked {
			_, _ = connection.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()
	observed.armed.Store(true)
	events, _, unsubscribe := hub.services.activity.Subscribe("0")
	defer unsubscribe()
	unblock()
	select {
	case <-observed.exhausted:
	case <-ctx.Done():
		t.Fatal("real initial settlement did not exhaust three SQLite Busy attempts")
	}
	var settled sql.NullString
	require.NoError(t, connection.QueryRowContext(ctx, "SELECT settled_at FROM runtime_receipts WHERE run_id = ?", run).Scan(&settled))
	require.False(t, settled.Valid, "database contention must retain exclusion")
	before := runtimeEffectEvents(hub.services.http.snapshot())
	require.Len(t, before, 1)
	_, err = connection.ExecContext(ctx, "COMMIT")
	require.NoError(t, err)
	locked = false
	for {
		select {
		case event := <-events:
			if event.Activity != nil && event.Activity.ID == run && event.Activity.Status == activitytypes.StatusSuccess {
				activationJoinRun(t, hub, hub, "0", run)
				require.Equal(t, before, runtimeEffectEvents(hub.services.http.snapshot()))
				return
			}
		case <-ctx.Done():
			t.Fatal("actual Fx maintenance driver did not settle the frozen result")
		}
	}
}
