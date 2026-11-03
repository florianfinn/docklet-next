// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original real session-to-widget rights and durable activity integration.

package widget_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/internal/widget"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitydto "github.com/florianfinn/docklet-next/types/activity"
	authdto "github.com/florianfinn/docklet-next/types/auth"
	dto "github.com/florianfinn/docklet-next/types/widget"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestSW9RealSessionAndSW13DurableActivity(t *testing.T) {
	ctx := t.Context()
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "widget-session-test-key-with-32-plus-bytes"})
	db, err := database.Initialize(ctx, sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	roles := role.NewRoleService(db)
	require.NoError(t, roles.EnsureBuiltInRoles(ctx))
	users := user.NewUserService(db, roles, session.RevokeAllUserSessionsExceptInDB)
	admin, err := users.CreateUser(ctx, &user.User{Username: "widget-admin"})
	require.NoError(t, err)
	require.NoError(t, roles.SetUserAssignments(ctx, admin.ID, []role.UserRoleAssignment{{RoleID: authz.BuiltInRoleAdmin}}))
	viewer, err := users.CreateUser(ctx, &user.User{Username: "widget-viewer"})
	require.NoError(t, err)
	preferences, err := settings.NewSettingsService(ctx, db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, preferences.Stop(context.WithoutCancel(ctx))) })
	cfg := &config.Config{AuthMode: "local", AppUrl: "https://hub.example.com", Environment: "production", JWTRefreshExpiry: time.Hour}
	identity := auth.NewAuthService(users, preferences, nil, session.NewSessionService(db), cfg)
	require.NoError(t, identity.InitializeSignIn(ctx))
	adminToken, err := identity.CompleteLogin(ctx, admin, authdto.SessionMeta{}, session.UserSessionSourceLocal)
	require.NoError(t, err)
	viewerToken, err := identity.CompleteLogin(ctx, viewer, authdto.SessionMeta{}, session.UserSessionSourceLocal)
	require.NoError(t, err)
	var posts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/api/v1/request/12/approve", r.URL.Path)
		posts.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	service := widget.NewService(db, activity.NewActivityService(db, preferences))
	_, err = service.Save(ctx, dto.Overseerr, dto.ConfigUpdate{Enabled: true, Address: upstream.URL})
	require.NoError(t, err)
	router := echo.New()
	api.SetupAPI(router, router.Group("/api"), handlerutil.NewActivityAppContext(ctx), cfg, api.HandlerDeps{Widget: widget.New(service), Auth: auth.New(identity, users, preferences), Role: role.New(roles), ApiKey: apikey.New(apikey.NewApiKeyService(db, users, roles))})
	request := func(token, method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/service-widgets"+path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, 401, request("", "GET", "").Code)
	for _, path := range []string{"", "/plex"} {
		w := request(viewerToken.AccessToken, "GET", path)
		require.Equal(t, 200, w.Code, w.Body.String())
	}
	for _, op := range []struct{ method, path string }{{"GET", "/plex/config"}, {"PUT", "/plex/config"}, {"POST", "/plex/test"}, {"POST", "/overseerr/requests/12/approve"}, {"POST", "/overseerr/requests/12/decline"}} {
		w := request(viewerToken.AccessToken, op.method, op.path)
		require.Equal(t, 403, w.Code, w.Body.String())
	}
	require.Zero(t, posts.Load())
	w := request(adminToken.AccessToken, "POST", "/overseerr/requests/12/approve")
	require.Equal(t, 200, w.Code, w.Body.String())
	var action dto.ActionResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &action))
	require.NotEmpty(t, action.ActivityID)
	var recorded activity.Activity
	require.NoError(t, db.First(&recorded, "id = ?", action.ActivityID).Error)
	require.Equal(t, activitydto.StatusSuccess, recorded.Status)
	require.NotNil(t, recorded.StartedByUserID)
	require.Equal(t, admin.ID, *recorded.StartedByUserID)
	require.Equal(t, "approve", recorded.Metadata["action"])
	require.Equal(t, "overseerr", recorded.Metadata["widget"])
	require.EqualValues(t, 1, posts.Load())
}
