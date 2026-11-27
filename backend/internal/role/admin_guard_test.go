// Source: docklet-next/issues/669
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original transactional role removal and HTTP conflict regression.

package role_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestRoleMutationKeepsEnabledGlobalAdmin(t *testing.T) {
	for _, op := range []string{"update", "delete"} {
		t.Run(op, func(t *testing.T) {
			db, svc := setupUserAndRoleServices(t)
			admin := createTestUser(t, db, "admin", "admin")
			disabled := createTestUser(t, db, "disabled", "disabled")
			r, err := svc.CreateRole(t.Context(), "Custom admin", nil, authz.AllPermissions())
			require.NoError(t, err)
			require.NoError(t, svc.SetUserAssignments(t.Context(), admin.ID, []role.UserRoleAssignment{{RoleID: r.ID}}))
			grantGlobalAdmin(t, svc, disabled.ID)
			require.NoError(t, db.Model(disabled).Update("disabled_at", time.Now()).Error)
			if op == "update" {
				_, err = svc.UpdateRole(t.Context(), r.ID, "changed", nil, []string{authz.PermContainersList})
			} else {
				err = svc.DeleteRole(t.Context(), r.ID)
			}
			require.ErrorIs(t, err, common.ErrNoGlobalAdminRemains)
			persisted, err := svc.GetRole(t.Context(), r.ID)
			require.NoError(t, err)
			require.Equal(t, "Custom admin", persisted.Name)
			ps, err := svc.ResolvePermissions(t.Context(), admin.ID)
			require.NoError(t, err)
			require.True(t, ps.IsGlobalAdmin())
			// The registered HTTP receiver uses the same conflict status as assignment updates.
			e := echo.New()
			e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c *echo.Context) error {
					c.SetRequest(c.Request().WithContext(context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
					return next(c)
				}
			})
			api := humaecho.New(e, huma.DefaultConfig("test", "1.0.0"))
			role.RegisterRoles(api, svc)
			method, body := http.MethodDelete, ""
			if op == "update" {
				method = http.MethodPut
				body = `{"name":"changed","permissions":["containers:list"]}`
			}
			req := httptest.NewRequest(method, "/roles/"+r.ID, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			require.NoError(t, db.Model(disabled).Update("disabled_at", nil).Error)
			if op == "update" {
				_, err = svc.UpdateRole(t.Context(), r.ID, "changed", nil, []string{authz.PermContainersList})
			} else {
				err = svc.DeleteRole(t.Context(), r.ID)
			}
			require.NoError(t, err)
		})
	}
}
