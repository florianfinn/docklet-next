// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original router-level bounded event query and SQLite query-plan tests.

package event

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/base"
	eventdto "github.com/florianfinn/docklet-next/types/event"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func windowFixture(t *testing.T, old, recent int) (*echo.Echo, *database.DB, time.Time) {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "events.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	boundary := time.Date(2026, 11, 10, 12, 0, 0, 123000000, time.UTC)
	rows := make([]Event, 0, old+recent)
	for i := range old + recent {
		at := boundary.Add(-time.Hour)
		if i >= old {
			at = boundary
		}
		rows = append(rows, Event{ID: fmt.Sprintf("event-%05d", i), Type: EventTypeContainerStart, MessageCode: "container.start", Title: "Example", Timestamp: at, EnvironmentID: new("example")})
	}
	require.NoError(t, db.CreateInBatches(rows, 100).Error)
	router := echo.New()
	api := humaecho.New(router, huma.DefaultConfig("test", "1.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	RegisterEvents(api, NewEventService(db, nil, nil))
	return router, db, boundary
}

func getWindow(t *testing.T, router *echo.Echo, path string) base.Paginated[eventdto.Event] {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var out base.Paginated[eventdto.Event]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

func TestWindowFiltersCountAndPageThroughRouter(t *testing.T) {
	router, db, since := windowFixture(t, 20000, 10)
	for _, route := range []string{"/events", "/events/environment/example"} {
		out := getWindow(t, router, route+"?since="+url.QueryEscape(since.Format(time.RFC3339Nano))+"&limit=500&sort=timestamp&order=desc")
		require.Len(t, out.Data, 10)
		require.EqualValues(t, 10, out.Pagination.TotalItems)
		for i, row := range out.Data {
			require.Equal(t, fmt.Sprintf("event-%05d", 20009-i), row.ID)
			require.True(t, row.Timestamp.Equal(since), "inclusive boundary")
		}
	}
	offsetTime := since.In(time.FixedZone("example", 3600))
	require.Len(t, getWindow(t, router, "/events?since="+url.QueryEscape(offsetTime.Format(time.RFC3339Nano))).Data, 10)
	for _, sql := range []string{
		"SELECT count(*) FROM events WHERE timestamp >= ?",
		"SELECT * FROM events WHERE timestamp >= ? ORDER BY timestamp DESC,id DESC LIMIT 500 OFFSET 0",
	} {
		var plans []struct {
			ID, Parent, Notused int
			Detail              string
		}
		require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+sql, since).Scan(&plans).Error)
		require.NotEmpty(t, plans)
		for _, plan := range plans {
			t.Logf("EXPLAIN %s: %s", sql, plan.Detail)
		}
		require.Contains(t, fmt.Sprint(plans), "idx_events_timestamp")
	}
}

func TestWindowValidationAndLegacyClampThroughRouter(t *testing.T) {
	router, db, since := windowFixture(t, 1, 501)
	for _, route := range []string{"/events", "/events/environment/example"} {
		t.Run(route, func(t *testing.T) {
			query := "?since=" + url.QueryEscape(since.Format(time.RFC3339Nano))
			for _, suffix := range []string{"", "&limit=500"} {
				out := getWindow(t, router, route+query+suffix)
				require.Len(t, out.Data, 500)
				require.EqualValues(t, 501, out.Pagination.TotalItems)
				require.EqualValues(t, 500, out.Pagination.ItemsPerPage)
			}
			one := getWindow(t, router, route+query+"&limit=1&start=25&sort=timestamp&order=desc")
			require.Len(t, one.Data, 1)
			require.Equal(t, "event-00476", one.Data[0].ID)
			for _, invalid := range []string{"?since=invalid", "?since=2026-11-10", "?since=", query + "&limit=-1", query + "&limit=0", query + "&limit=501", query + "&since=invalid"} {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest("GET", route+invalid, http.NoBody))
				require.Equal(t, 400, rec.Code, invalid+": "+rec.Body.String())
				require.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
			}
			require.Len(t, getWindow(t, router, route+"?limit=500").Data, 100)
			require.Len(t, getWindow(t, router, route).Data, 20)
			require.Len(t, getWindow(t, router, route+"?limit=-1").Data, 502)
		})
	}
	// A neighbour using the shared paginator keeps its original cap.
	var result []Event
	page, err := pagination.PaginateAndSortDB(pagination.QueryParams{Limit: 500}, db.Model(&Event{}), &result)
	require.NoError(t, err)
	require.Len(t, result, 100)
	require.Equal(t, 100, page.ItemsPerPage)
}

func TestWindowSubmicrosecondSinceThroughRouter(t *testing.T) {
	router, _, boundary := windowFixture(t, 1, 1)
	for _, route := range []string{"/events", "/events/environment/example"} {
		before := getWindow(t, router, route+"?since="+url.QueryEscape(boundary.Add(-time.Nanosecond).Format(time.RFC3339Nano)))
		require.Len(t, before.Data, 1)
		after := getWindow(t, router, route+"?since="+url.QueryEscape(boundary.Add(time.Nanosecond).Format(time.RFC3339Nano)))
		require.EqualValues(t, 0, after.Pagination.TotalItems, "a lower bound after the persisted instant excludes that event")
		require.Empty(t, after.Data)
	}
}
