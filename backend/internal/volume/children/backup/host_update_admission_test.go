// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Injected host-admission errors through actual backup consumers and HTTP.

package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	dto "github.com/florianfinn/docklet-next/types/backup"
	host "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestBackupHostAdmissionConsumersAndHTTP(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, busy := range []bool{true, false} {
			name := mode + map[bool]string{true: "-host-busy", false: "-engine-error"}[busy]
			t.Run(name, func(t *testing.T) {
				f := newRuntimePlan(t)
				require.NoError(t, f.service.deps.DB.AutoMigrate(&policies.VolumeBackupPolicy{}))
				problem := &host.HostProblem{Code: "host.busy", Status: 409}
				refusal := error(problem)
				if !busy {
					refusal = errors.New("synthetic engine failure")
				}
				f.service.deps.Engine.SetHostAdmission(func(context.Context) error { return refusal })
				ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.SystemUser)
				// Both actual consumers run before any helper or backup row is written.
				_, _, err := f.service.acquireOperation(ctx, "data", "")
				checkAdmissionRefusal(t, err, problem, busy)
				_, err = f.service.startRepositoryCheck(ctx, "local")
				checkAdmissionRefusal(t, err, problem, busy)
				router, base := humatest.New(t)
				api := huma.NewGroup(base, "/api")
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
					request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
					next(huma.WithContext(ctx, request))
				})
				RegisterRoutes(api, NewHandler(f.service, nil, nil, nil, t.Context(), &config.Config{AgentMode: true}))
				backuptarget.RegisterHostRoutes(api, TargetHandler(f.service, nil, &config.Config{AgentMode: true}, t.Context()), true)
				status, _, body := backupRouteTransport(t, mode, router)(t.Context(), "POST", "/api/environments/0/volumes/data/backups", []byte(`{}`))
				want, code := 503, "volume.backup.engine-unavailable"
				if busy {
					want, code = 409, "host.busy"
				}
				require.Equal(t, want, status, string(body))
				require.Contains(t, string(body), `"code":"`+code+`"`)
				require.Empty(t, f.creates)
				status, _, body = backupRouteTransport(t, mode, router)(t.Context(), "POST", "/api/environments/0/volumes/backups/targets/local/test", nil)
				require.Equal(t, want, status, string(body))
				require.Contains(t, string(body), `"code":"`+code+`"`)
				require.Empty(t, f.creates)
				// The actual remote JSON consumer receives bytes from the real transport.
				call := backupRouteTransport(t, mode, router)
				proxy := func(ctx context.Context, _ string, method, path string, _ []byte, _ any) error {
					status, _, body := call(ctx, method, path, nil)
					return &remenv.StatusError{StatusCode: status, Body: body}
				}
				_, err = backuptarget.RemoteJSON[any](t.Context(), proxy, "0", "POST", "/api/environments/0/volumes/backups/targets/local/test", nil)
				require.Error(t, err)
				if busy {
					var remote *host.HostProblem
					require.ErrorAs(t, err, &remote)
					require.Equal(t, 409, remote.Status)
					require.Equal(t, "host.busy", remote.Code)
				} else {
					var remote *dto.Problem
					require.ErrorAs(t, err, &remote)
					require.Equal(t, 503, remote.Status)
					require.Equal(t, "volume.backup.engine-unavailable", remote.Code)
				}
			})
		}
	}
}
func checkAdmissionRefusal(t *testing.T, err error, original *host.HostProblem, busy bool) {
	t.Helper()
	require.Error(t, err)
	if busy {
		require.Same(t, original, err)
		return
	}
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, 503, problem.Status)
	require.Equal(t, "volume.backup.engine-unavailable", problem.Code)
}
