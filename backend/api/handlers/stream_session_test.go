// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

package handlers

import (
	"bufio"
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSessionSSEExplicitAndDurableCadence(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("active=%v/explicit=%v", active, explicit), func(t *testing.T) {
				db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"))
				require.NoError(t, err)
				require.NoError(t, db.AutoMigrate(&access.Assignment{}, &user.User{}, &session.UserSession{}))
				u := &user.User{Username: "example-sse"}
				require.NoError(t, db.Create(u).Error)
				service := session.NewSessionService(&database.DB{DB: db})
				record, _, err := service.CreateSession(t.Context(), u.ID, time.Now().Add(time.Hour), auth.SessionMeta{})
				require.NoError(t, err)
				ticks, ready, done := make(chan time.Time), make(chan struct{}), make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer close(done)
					ctx := service.ViewerContext(r.Context(), record.ID, u.ID)
					ctx = sessionlife.WithCadence(ctx, func(period time.Duration, once bool) sessionlife.Cadence {
						if period != 15*time.Second || once {
							t.Error("wrong SSE cadence")
						}
						close(ready)
						return sessionlife.Cadence{C: ticks, Stop: func() {}}
					})
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
					h := &StreamHandler{}
					h.streamClientInternal(ctx, nil, &StreamClientInput{}, sse.Sender(func(message sse.Message) error {
						_, err := fmt.Fprintf(w, "data: %v\n\n", message.Data)
						w.(http.Flusher).Flush()
						return err
					}))
				}))
				defer server.Close()
				response, err := server.Client().Get(server.URL)
				require.NoError(t, err)
				defer response.Body.Close()
				<-ready
				if active {
					ticks <- time.Now()
					_, err := bufio.NewReader(response.Body).ReadString('\n')
					require.NoError(t, err)
				}
				if explicit {
					require.NoError(t, session.NewSessionService(&database.DB{DB: db}).RevokeSession(t.Context(), record.ID))
				} else {
					require.NoError(t, db.Model(record).Update("expires_at", time.Now().Add(-time.Hour)).Error)
					ticks <- time.Now()
				}
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("SSE send loop did not join after revoke")
				}
				_, err = io.ReadAll(response.Body)
				require.NoError(t, err)
			})
		}
	}
}

func TestSessionSSERegistrationRejectsBeforeSending(t *testing.T) {
	sent := false
	ctx := sessionlife.WithSession(t.Context(), "example-rejected-sse", "example-user", func(context.Context) error { return io.EOF })
	(&StreamHandler{}).streamClientInternal(ctx, nil, &StreamClientInput{}, sse.Sender(func(sse.Message) error { sent = true; return nil }))
	require.False(t, sent)
}
