// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original import route authorisation and per-principal concurrency guards.

package stackimport

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type resolverFunc func(context.Context, dto.Request) (dto.Result, error)

func (f resolverFunc) Resolve(ctx context.Context, r dto.Request) (dto.Result, error) {
	return f(ctx, r)
}
func TestM19ImportRightsStubAndConcurrentGate(t *testing.T) {
	entered, release := make(chan string, 2), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var calls atomic.Int32
	module := NewWithResolver(resolverFunc(func(ctx context.Context, r dto.Request) (dto.Result, error) {
		calls.Add(1)
		entered <- r.Ref
		<-release
		return dto.Result{}, dto.Failure("import.source-unavailable", 502)
	}))
	router, api := humatest.New(t)
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		ps := authz.SudoPermissionSet()
		if c.Header("X-Test-Role") == "user" {
			ps = authz.NewPermissionSet()
			ps.UserID = "reader"
		}
		ctx := context.WithValue(c.Context(), middleware.ContextKeyUserPermissions, ps)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, c.Header("X-Test-User"))
		next(huma.WithContext(c, ctx))
	})
	module.RegisterRoutes(api)
	request := func(user string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/stack-import", strings.NewReader(`{"kind":"github","ref":"owner/repo"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Test-User", user)
		router.ServeHTTP(rec, r)
		return rec
	}
	done := make(chan *httptest.ResponseRecorder, 2)
	go func() { done <- request("admin-a") }()
	<-entered
	busy := request("admin-a")
	require.Equal(t, 409, busy.Code)
	require.Contains(t, busy.Body.String(), "import.busy")
	go func() { done <- request("admin-b") }()
	select {
	case <-entered:
	case rec := <-done:
		require.Equal(t, http.StatusBadGateway, rec.Code)
		t.Fatal("other principal never entered resolver")
	}
	close(release)
	for range 2 {
		rec := <-done
		require.Equal(t, 502, rec.Code)
		require.Contains(t, rec.Body.String(), "import.source-unavailable")
	}
	// Failure releases the gate; the S1 resolver remains the replaceable boundary.
	_, err := New().importSource(context.WithValue(t.Context(), middleware.ContextKeyUserID, "admin-a"), &input{Body: dto.Request{Kind: "github", Ref: "owner/repo"}})
	require.EqualError(t, err, "import.source-unavailable")
}

func TestM19ImportRightsRealHandler(t *testing.T) {
	_, api := humatest.New(t)
	api.UseMiddleware(func(c huma.Context, next func(huma.Context)) {
		ps := authz.NewPermissionSet()
		ps.UserID = "reader"
		ctx := context.WithValue(c.Context(), middleware.ContextKeyUserPermissions, ps)
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "reader")
		next(huma.WithContext(c, ctx))
	})
	New().RegisterRoutes(api)
	response := api.Post("/stack-import", map[string]any{"kind": "github", "ref": "owner/repo"})
	require.Equal(t, 403, response.Code)
	require.Contains(t, response.Body.String(), "access.admin-required")
}
