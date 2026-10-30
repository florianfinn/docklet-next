// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/autoheal"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	nt "github.com/florianfinn/docklet-next/types/notification"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAutoHealDeterminedAgentOutcomeReachesTheHubOverHTTP(t *testing.T) {
	for _, stage := range []string{"settle", "projection"} {
		for _, failed := range []bool{false, true} {
			for _, late := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/failed=%t/late=%t", stage, failed, late), func(t *testing.T) {
					j, db := autoHealFixture(t)
					var received atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						received.Add(1)
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"data":{"message":"accepted"}}`))
					}))
					defer server.Close()
					token := "example"
					cfg := &config.Config{AgentMode: true, ManagerApiUrl: server.URL, AgentToken: token}
					n := notification.NewNotificationService(db, cfg, nil, nil)
					j.store.Config = cfg
					attempts := 0
					var dispatchErr error
					var dispatchCtxErr error
					j.store.Dispatch = func(ctx context.Context, p nt.DispatchRequest) error {
						attempts++
						dispatchCtxErr = ctx.Err()
						dispatchErr = n.SendHostEvent(ctx, p)
						return dispatchErr
					}
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					j.restartContainer = func(context.Context, *client.Client, string) error {
						if failed {
							return errors.New("controlled Docker failure")
						}
						return nil
					}
					injected := false
					if stage == "settle" {
						require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:cancel", func(tx *gorm.DB) {
							if tx.Statement.Table == "auto_heal_restarts" {
								if m, ok := tx.Statement.Dest.(map[string]any); ok && m["outcome"] != nil {
									injected = true
									if late {
										cancel()
									}
								}
							}
						}))
					} else {
						require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:cancel", func(tx *gorm.DB) {
							if _, ok := tx.Statement.Dest.(*activity.Activity); ok {
								injected = true
								if late {
									cancel()
								}
							}
						}))
					}
					_, _ = j.Run(ctx)
					require.True(t, injected)
					var row autoheal.Restart
					require.NoError(t, db.First(&row).Error)
					require.NotNil(t, row.ProjectedAt)
					require.Equal(t, 1, attempts)
					t.Logf("outcome=%s dispatchCtxErr=%v dispatchErr=%v received=%d", row.Outcome, dispatchCtxErr, dispatchErr, received.Load())
					require.EqualValues(t, 1, received.Load(), "available HTTP hub must receive the determined outcome once")
					require.NoError(t, dispatchErr)
				})
			}
		}
	}
}
