// Source: docklet-next/issues/289
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backup

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBindBackupTypedRefusalThroughHubDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newRemoteFenceHost(t, mode, func(api huma.API, agent *runtimePlan, cfg *config.Config) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					next(huma.WithContext(ctx, context.WithValue(ctx.Context(), user.CurrentUserContextKey{}, &user.SystemUser)))
				})
				h := NewHandler(agent.service, nil, nil, nil, t.Context(), cfg)
				middleware.RegisterWithPermission(api, huma.Operation{OperationID: "create-volume-backup", Method: "POST", Path: "/environments/{id}/volumes/{volumeName}/backups"}, authz.PermVolumesBackup, h.CreateBackup)
			})
			engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/info") {
					_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
					return
				}
				f.agentDB.serve(w, r)
			}))
			defer engine.Close()
			cli, err := client.New(client.WithHost(engine.URL), client.WithAPIVersion("1.41"))
			require.NoError(t, err)
			defer cli.Close()
			f.agentDB.service.deps.Docker = f.agentDB.service.deps.Docker.WithClient(cli)
			ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.SystemUser)
			h := NewHandler(f.hub, nil, f.environments, nil, t.Context(), &config.Config{})
			_, err = h.CreateBackup(ctx, &CreateBackupInput{EnvironmentID: f.id, VolumeName: volume.BindKey("/etc/config")})
			var p *dto.Problem
			require.ErrorAs(t, err, &p)
			require.Equal(t, 422, p.Status, "%+v", p)
			require.Equal(t, "volume.bind.path-denied", p.Code)
			require.Equal(t, "system", p.Reason)
			require.Empty(t, f.agentDB.creates, "denied backup created no source engine")
		})
	}
}
