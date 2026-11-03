// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original success-only Compose stop/down intent producer assertions.

package project

import (
	"context"
	"encoding/json"
	"errors"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/runtimeintent"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHB5ProjectStopAndDownPersistActiveMembersOnly(t *testing.T) {
	for _, operation := range []string{"stop", "down"} {
		for _, fail := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "-success", true: "-failed"}[fail], func(t *testing.T) {
				svc, root := runtimeIdentityFixture(t)
				row := runtimeIdentityProject(t, svc, root, "example-project", "example", "services:\n  app:\n    image: registry.example.com/app:1\n")
				daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if strings.HasSuffix(r.URL.Path, "/containers/json") {
						_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "running", "Names": []string{"/example-app"}, "State": "running"}, map[string]any{"Id": "looping", "Names": []string{"/example-loop"}, "State": "restarting"}, map[string]any{"Id": "stopped", "Names": []string{"/example-old"}, "State": "exited"}})
						return
					}
					w.WriteHeader(200)
				}))
				defer daemon.Close()
				svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, daemon)}
				svc.initChildren(nil, nil)
				previousStop, previousDown := composeStopProjectServices, composeDownProject
				defer func() { composeStopProjectServices, composeDownProject = previousStop, previousDown }()
				effect := func() error {
					if fail {
						return errors.New("synthetic stop failure")
					}
					return nil
				}
				composeStopProjectServices = func(context.Context, *compose.Project, []string) error { return effect() }
				composeDownProject = func(context.Context, *compose.Project, bool) error { return effect() }
				var err error
				if operation == "stop" {
					err = svc.StopProject(t.Context(), row.ID, userDTO.SystemUser)
				} else {
					err = svc.DownProject(t.Context(), row.ID, userDTO.SystemUser)
				}
				var rows []runtimeintent.Row
				require.NoError(t, svc.db.Find(&rows).Error)
				if fail {
					require.Error(t, err)
					require.Empty(t, rows)
				} else {
					require.NoError(t, err)
					require.Len(t, rows, 2)
					ids := []string{rows[0].ContainerID, rows[1].ContainerID}
					require.ElementsMatch(t, []string{"running", "looping"}, ids)
					require.Equal(t, row.ID, rows[0].ProjectID)
					require.False(t, rows[0].RecordedAt.IsZero())
				}
			})
		}
	}
}

func TestHB5ContainerIntentAfterCancellationAndWithoutProject(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	row := runtimeIdentityProject(t, svc, root, "example-project", "example", "services:\n  app:\n    image: registry.example.com/app:1\n")
	require.NoError(t, svc.db.Model(&Project{}).Where("id=?", row.ID).Update("compose_project_name", "").Error)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, svc.RecordContainerStopIntent(ctx, "managed", "/example-app", "example"))
	require.NoError(t, svc.RecordContainerStopIntent(t.Context(), "unmanaged", "/standalone", ""))
	var rows []runtimeintent.Row
	require.NoError(t, svc.db.Order("container_id").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, row.ID, rows[0].ProjectID)
	require.Empty(t, rows[1].ProjectID)
}
