// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original registered-widget HTTP authorization and transient-test checks.

package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/user"
	dto "github.com/florianfinn/docklet-next/types/widget"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func widgetRouter(s *Service) *echo.Echo {
	router := echo.New()
	cfg := huma.DefaultConfig("Widgets", "test")
	cfg.Transformers = append(cfg.Transformers, ErrorTransformer)
	api := humaecho.NewWithGroup(router, router.Group("/api"), cfg)
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		var ps *authz.PermissionSet
		if identity := ctx.Header("X-Test-Identity"); identity != "" {
			ps = access.Synthesize(identity, nil)
			if identity == "admin" {
				ps.Sudo = true
			}
		}
		c := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, ps)
		if ps != nil {
			c = context.WithValue(c, user.CurrentUserContextKey{}, &user.Actor{ID: ps.UserID, Username: ps.UserID})
		}
		next(huma.WithContext(ctx, c))
	})
	api.UseMiddleware(middleware.AccessMiddleware(api))
	New(s).RegisterRoutes(api)
	return router
}
func call(t *testing.T, router *echo.Echo, identity, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/service-widgets"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Identity", identity)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestSW9RightsThroughRegisteredHTTP(t *testing.T) {
	s := testService(t)
	router := widgetRouter(s)
	for _, path := range []string{"", "/plex"} {
		require.Equal(t, 401, call(t, router, "", "GET", path, "").Code)
		w := call(t, router, "viewer", "GET", path, "")
		require.Equal(t, 200, w.Code, w.Body.String())
	}
	for _, op := range []struct{ method, path string }{{"GET", "/plex/config"}, {"PUT", "/plex/config"}, {"POST", "/plex/test"}, {"POST", "/overseerr/requests/1/approve"}, {"POST", "/overseerr/requests/1/decline"}} {
		w := call(t, router, "viewer", op.method, op.path, `{}`)
		require.Equal(t, 403, w.Code, op.path)
		require.Equal(t, 401, call(t, router, "", op.method, op.path, `{}`).Code)
	}
	for _, id := range []string{"list-service-widgets", "get-service-widget"} {
		require.Equal(t, access.Self, access.Rules[id].Kind)
	}
	for _, id := range []string{"get-service-widget-config", "put-service-widget-config", "test-service-widget", "approve-service-widget-request", "decline-service-widget-request"} {
		require.Equal(t, access.Admin, access.Rules[id].Kind)
	}
}

func snapshot(s *Service) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(struct {
		Generation map[dto.Kind]uint64
		Cache      map[dto.Kind]cacheEntry
	}{s.generation, s.cache})
	s.titles.mu.Lock()
	defer s.titles.mu.Unlock()
	return string(b) + fmt.Sprint(s.titles.entries)
}

func TestSW8TransientHTTPDoesNotTouchLiveState(t *testing.T) {
	s := testService(t)
	var liveCalls atomic.Int32
	var fail atomic.Bool
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		liveCalls.Add(1)
		if fail.Load() {
			w.WriteHeader(502)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/count") {
			respond(w, `{"pending":1,"approved":0,"available":0}`)
		} else if r.URL.Path == "/api/v1/request" {
			respond(w, overseerrList("movie", 1))
		} else {
			respond(w, `{"title":"Live title"}`)
		}
	}))
	defer live.Close()
	_, err := s.Save(t.Context(), dto.Overseerr, dto.ConfigUpdate{Enabled: true, Address: live.URL, CredentialAction: "replace", Credential: "stored-synthetic-key"})
	require.NoError(t, err)
	require.Nil(t, s.Get(t.Context(), dto.Overseerr).Error)
	draft := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "stored-synthetic-key", r.Header.Get("X-Api-Key"))
		if fail.Load() {
			w.WriteHeader(401)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/count") {
			respond(w, `{"pending":1,"approved":0,"available":0}`)
		} else if r.URL.Path == "/api/v1/request" {
			respond(w, overseerrList("movie", 1))
		} else {
			respond(w, `{"title":"Draft title"}`)
		}
	}))
	defer draft.Close()
	before, err := load(t.Context(), s.db, dto.Overseerr)
	require.NoError(t, err)
	router := widgetRouter(s)
	for _, failed := range []bool{false, true} {
		fail.Store(failed)
		if failed {
			s.mu.Lock()
			e := s.cache[dto.Overseerr]
			e.Next = s.now()
			s.cache[dto.Overseerr] = e
			s.mu.Unlock()
			require.True(t, s.Get(t.Context(), dto.Overseerr).Stale)
		}
		state := snapshot(s)
		body, _ := json.Marshal(dto.ConfigUpdate{Enabled: true, Address: draft.URL, CredentialAction: "keep"})
		w := call(t, router, "admin", "POST", "/overseerr/test", string(body))
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "stored-synthetic-key")
		var result dto.Result
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		if failed {
			require.Equal(t, dto.AuthFailed, result.Error.Code)
		} else {
			require.Nil(t, result.Error)
			require.Equal(t, "Draft title", result.Overseerr.Requests[0].Title, "transient lookup must not reuse the live title")
		}
		after, err := load(t.Context(), s.db, dto.Overseerr)
		require.NoError(t, err)
		require.Equal(t, before, after)
		require.Equal(t, state, snapshot(s), "test action must preserve live generation, cache, backoff and derived titles")
	}
}

func TestSW7HTTPValidationDoesNotEchoSecrets(t *testing.T) {
	router := widgetRouter(testService(t))
	for _, body := range []string{`{"credentialAction":"invalid","credential":"synthetic-secret-value"}`, `{"address":3,"credential":"synthetic-secret-value"}`, `{"credential":"synthetic-secret-value",`, strings.Repeat(" ", 17000) + `{"credential":"synthetic-secret-value"}`} {
		w := call(t, router, "admin", "PUT", "/plex/config", body)
		require.Equal(t, 400, w.Code)
		require.NotContains(t, w.Body.String(), "synthetic-secret-value")
		require.Contains(t, w.Body.String(), string(dto.InvalidConfig))
	}
}

func TestSW13AdminActionThroughHTTP(t *testing.T) {
	s := testService(t)
	var posts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/api/v1/request/9/decline", r.URL.Path)
		posts.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	configure(t, s, dto.Overseerr, upstream.URL)
	router := widgetRouter(s)
	denied := call(t, router, "viewer", "POST", "/overseerr/requests/9/decline", "")
	require.Equal(t, 403, denied.Code)
	require.Zero(t, posts.Load())
	w := call(t, router, "admin", "POST", "/overseerr/requests/9/decline", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.EqualValues(t, 1, posts.Load())
	require.Len(t, s.activities.(*auditStub).starts, 1)
}

func TestSW11CodesMatchSchemaAndConstants(t *testing.T) {
	field, _ := reflect.TypeFor[dto.Problem]().FieldByName("Code")
	codes := strings.Split(field.Tag.Get("enum"), ",")
	want := []dto.Code{dto.NotConfigured, dto.Disabled, dto.Unreachable, dto.Timeout, dto.AuthFailed, dto.BadResponse, dto.RedirectRefused, dto.TLSFailed, dto.CredentialUnavailable, dto.InvalidConfig, dto.PersistenceFailed}
	actual := make([]string, len(want))
	for i, c := range want {
		actual[i] = string(c)
	}
	require.Equal(t, actual, codes)
	for _, k := range []dto.Kind{dto.Plex, dto.Overseerr, dto.Minecraft} {
		require.True(t, k.Valid())
	}
	require.False(t, dto.Kind("unknown").Valid())
}
