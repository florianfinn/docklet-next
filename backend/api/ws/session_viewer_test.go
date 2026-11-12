// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

package ws

import (
	"context"
	"fmt"
	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSessionViewerRealLocalDirectEdgeActiveIdle(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		for _, active := range []bool{false, true} {
			for _, explicit := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/active=%v/explicit=%v", mode, active, explicit), func(t *testing.T) {
					db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"))
					require.NoError(t, err)
					require.NoError(t, db.AutoMigrate(&access.Assignment{}, &user.User{}, &session.UserSession{}))
					u := &user.User{Username: "example-viewer"}
					require.NoError(t, db.Create(u).Error)
					service := session.NewSessionService(&database.DB{DB: db})
					record, _, err := service.CreateSession(t.Context(), u.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
					require.NoError(t, err)
					ticks, ready := make(chan time.Time), make(chan struct{})
					decorate := func(r *http.Request) *http.Request {
						ctx := service.ViewerContext(r.Context(), record.ID, u.ID)
						ctx = sessionlife.WithCadence(ctx, func(period time.Duration, _ bool) sessionlife.Cadence {
							if period != 54*time.Second {
								t.Error("wrong viewer cadence")
							}
							close(ready)
							return sessionlife.Cadence{C: ticks, Stop: func() {}, Reset: func(time.Duration) {}}
						})
						return r.WithContext(ctx)
					}
					joined, entered := make(chan struct{}), make(chan struct{})
					h := &WebSocketHandler{containerLogStreamer: func(ctx context.Context, _ string, out chan<- string, _ bool, _, _ string, _ bool) error {
						defer close(joined)
						close(entered)
						if active {
							select {
							case out <- "example live output":
							case <-ctx.Done():
								return ctx.Err()
							}
						}
						<-ctx.Done()
						return ctx.Err()
					}}
					target := ""
					if mode == "local" {
						target = logServer(t, h, true, decorate) + "containers/example/logs?format=json"
					} else {
						target = logServer(t, h, true) + "containers/example/logs?format=json"
					}
					viewer := target
					if mode == "edge" {
						viewer = edgeLogViewer(t, target, decorate)
					}
					if mode == "direct" {
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							_ = wshub.ProxyHTTP(w, decorate(r), target, nil, func(*http.Request) bool { return true })
						}))
						defer server.Close()
						viewer = "ws" + strings.TrimPrefix(server.URL, "http")
					}
					conn := dialLog(t, viewer)
					<-ready
					<-entered
					if active {
						require.Contains(t, string(readLog(t, conn)), "example live output")
					}
					if explicit {
						require.NoError(t, session.NewSessionService(&database.DB{DB: db}).RevokeSession(t.Context(), record.ID))
					} else {
						require.NoError(t, db.Model(record).Update("expires_at", time.Now().Add(-time.Hour)).Error)
						ticks <- time.Now()
					}
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					defer cancel()
					_, _, err = conn.Read(ctx)
					require.Error(t, err)
					require.ErrorIs(t, ctx.Err(), nil, "viewer must close without consuming the test deadline")
					waitLogSignal(t, joined)
					h.logStreamsMu.Lock()
					remaining := len(h.logStreams)
					h.logStreamsMu.Unlock()
					require.Zero(t, remaining, "last viewer must release source references")
				})
			}
		}
	}
}

func TestSessionViewerTwoTabsOtherSessionAndSharedTunnelSurvive(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"))
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&user.User{}, &session.UserSession{}))
			u := &user.User{Username: "example-tabs"}
			require.NoError(t, db.Create(u).Error)
			service := session.NewSessionService(&database.DB{DB: db})
			revoked, _, err := service.CreateSession(t.Context(), u.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
			require.NoError(t, err)
			survivor, _, err := service.CreateSession(t.Context(), u.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
			require.NoError(t, err)
			ready := make(chan struct{}, 3)
			cadence := func(context.Context) sessionlife.CadenceFactory {
				return func(time.Duration, bool) sessionlife.Cadence {
					ready <- struct{}{}
					return sessionlife.Cadence{C: make(chan time.Time), Stop: func() {}, Reset: func(time.Duration) {}}
				}
			}
			bind := func(r *http.Request) *http.Request {
				return r.WithContext(service.ViewerContext(r.Context(), r.Header.Get("X-Example-Session"), u.ID))
			}
			localBind := func(r *http.Request) *http.Request {
				if mode == "local" {
					r = bind(r)
				}
				return r.WithContext(sessionlife.WithCadence(r.Context(), cadence(r.Context())))
			}
			produce, joined := make(chan string, 1), make(chan struct{})
			h := &WebSocketHandler{containerLogStreamer: func(ctx context.Context, _ string, out chan<- string, _ bool, _, _ string, _ bool) error {
				defer close(joined)
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case line := <-produce:
						select {
						case out <- line:
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
			}}
			target := logServer(t, h, true, localBind) + "containers/example/logs?format=json"
			viewer := target
			if mode == "edge" {
				viewer = edgeLogViewer(t, target, bind)
			}
			if mode == "direct" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = wshub.ProxyHTTP(w, bind(r), target, nil, func(*http.Request) bool { return true })
				}))
				defer server.Close()
				viewer = "ws" + strings.TrimPrefix(server.URL, "http")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			connect := func(id string) *websocket.Conn {
				conn, _, err := websocket.Dial(ctx, viewer, &websocket.DialOptions{HTTPHeader: http.Header{"X-Example-Session": []string{id}}})
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.CloseNow() })
				return conn
			}
			first, second, other := connect(revoked.ID), connect(revoked.ID), connect(survivor.ID)
			for range 3 {
				select {
				case <-ready:
				case <-ctx.Done():
					t.Fatal("viewer pumps not ready")
				}
			}
			select {
			case produce <- "before revoke":
			case <-ctx.Done():
				t.Fatal("shared producer stopped before revocation")
			}
			for _, conn := range []*websocket.Conn{first, second, other} {
				require.Contains(t, string(readLog(t, conn)), "before revoke")
			}
			require.NoError(t, session.NewSessionService(&database.DB{DB: db}).RevokeSession(t.Context(), revoked.ID))
			for _, conn := range []*websocket.Conn{first, second} {
				_, _, err := conn.Read(ctx)
				require.Error(t, err)
				require.NoError(t, ctx.Err())
			}
			select {
			case produce <- "other session still live":
			case <-ctx.Done():
				t.Fatal("revoking one session stopped the other session producer")
			}
			require.Contains(t, string(readLog(t, other)), "other session still live")
			select {
			case <-joined:
				t.Fatal("one session ended shared producer/tunnel")
			default:
			}
			_ = other.CloseNow()
			waitLogSignal(t, joined)
		})
	}
}
