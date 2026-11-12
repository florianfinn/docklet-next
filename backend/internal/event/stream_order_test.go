// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original real stream router ordering and post-subscription commit test.

package event_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/api/handlers"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	streamdto "github.com/florianfinn/docklet-next/types/stream"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type frameWriter struct {
	header http.Header
	mu     sync.Mutex
	body   strings.Builder
	frames chan streamdto.Event
}

func (w *frameWriter) Header() http.Header { return w.header }
func (w *frameWriter) WriteHeader(int)     {}
func (w *frameWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(b)
}
func (w *frameWriter) Flush() {
	w.mu.Lock()
	body := w.body.String()
	w.body.Reset()
	w.mu.Unlock()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			var e streamdto.Event
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e) == nil && e.EventLog != nil {
				w.frames <- e
			}
		}
	}
}

func TestEventStreamInitialFrameAfterSubscriptionThroughRouter(t *testing.T) {
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&event.Event{}))
	svc := event.NewEventService(&database.DB{DB: db}, nil, nil)
	synctest.Test(t, func(t *testing.T) {
		subscribing, release := make(chan struct{}), make(chan struct{})
		releaseOnce := sync.OnceFunc(func() { close(release) })
		defer releaseOnce()
		svc.SetSubscriptionBarrierForTest(func() { close(subscribing); <-release })
		router := echo.New()
		cfg := huma.DefaultConfig("test", "1.0.0")
		cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(typ reflect.Type, hint string) string {
			for typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			return strings.ReplaceAll(typ.PkgPath(), "/", "_") + huma.DefaultSchemaNamer(typ, hint)
		})
		api := humaecho.New(router, cfg)
		api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
			next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
		})
		handlers.RegisterStream(api, nil, nil, nil, svc, nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		writer := &frameWriter{header: http.Header{}, frames: make(chan streamdto.Event, 10)}
		done := make(chan struct{})
		go func() {
			defer close(done)
			router.ServeHTTP(writer, httptest.NewRequest("GET", "/stream?channels=events", http.NoBody).WithContext(ctx))
		}()
		<-subscribing
		synctest.Wait()
		require.Empty(t, writer.frames, "initial eventLog frame must never precede subscription")
		releaseOnce()
		synctest.Wait()
		require.Len(t, writer.frames, 1, "subscribed stream sends its initial eventLog frame")
		require.Equal(t, "changed", (<-writer.frames).EventLog.Type)
		_, err := svc.CreateEvent(ctx, event.CreateEventRequest{Type: event.EventTypeContainerStart, Title: "Example", EnvironmentID: new("0")})
		require.NoError(t, err)
		synctest.Wait()
		require.Len(t, writer.frames, 1, "a committed change after subscription is signalled")
		require.Equal(t, "changed", (<-writer.frames).EventLog.Type)
		cancel()
		<-done
	})
}
