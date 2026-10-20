// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original real WebSocket cadence-owner regression tests.

package ws

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/types/auth"
	systemtypes "github.com/florianfinn/docklet-next/types/system"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSessionAPICadenceOwners(t *testing.T) {
	for _, owner := range []string{"stats-exec-keepalive", "diagnostics-snapshots", "diagnostics-server-logs"} {
		t.Run(owner, func(t *testing.T) {
			db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"))
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&user.User{}, &session.UserSession{}))
			u := &user.User{Username: "example-cadence"}
			require.NoError(t, db.Create(u).Error)
			service := session.NewSessionService(&database.DB{DB: db})
			record, _, err := service.CreateSession(t.Context(), u.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
			require.NoError(t, err)
			ticks, ready, joined := make(chan time.Time), make(chan struct{}), make(chan struct{})
			h := &WebSocketHandler{wsMetrics: wshub.NewWebSocketMetrics()}
			router := echo.New()
			router.GET("/viewer", func(c *echo.Context) error {
				defer close(joined)
				ctx := service.ViewerContext(c.Request().Context(), record.ID, u.ID)
				ctx = sessionlife.WithCadence(ctx, func(period time.Duration, _ bool) sessionlife.Cadence {
					if period != 54*time.Second {
						t.Error("wrong API viewer cadence")
					}
					close(ready)
					return sessionlife.Cadence{C: ticks, Stop: func() {}}
				})
				c.SetRequest(c.Request().WithContext(ctx))
				switch owner {
				case "diagnostics-snapshots":
					return h.streamSnapshotsInternal(c, time.Hour, func(context.Context) any { return map[string]bool{"ready": true} }, nil)
				case "diagnostics-server-logs":
					return h.ServerLogsStream(c)
				default:
					conn, release, ok := h.acceptWSInternal(c, systemtypes.WSKindSystemStats, "")
					if !ok {
						return nil
					}
					defer release()
					defer conn.CloseNow()
					ctx, cancel := context.WithCancel(ctx)
					defer cancel()
					defer startWSKeepaliveInternal(ctx, cancel, conn, 54*time.Second)()
					_, _, _ = conn.Read(ctx)
					return nil
				}
			})
			server := httptest.NewServer(router)
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/viewer", nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			<-ready
			if owner == "diagnostics-snapshots" {
				_, _, err = conn.Read(ctx)
				require.NoError(t, err)
			}
			require.NoError(t, db.Model(record).Update("expires_at", time.Now().Add(-time.Hour)).Error)
			ticks <- time.Now()
			for {
				_, _, err = conn.Read(ctx)
				if err != nil {
					break
				}
			}
			require.NoError(t, ctx.Err(), "durable cadence must close before the test deadline")
			select {
			case <-joined:
			case <-ctx.Done():
				t.Fatal("API cadence helper did not join")
			}
			require.Empty(t, h.wsMetrics.Connections())
		})
	}
}
