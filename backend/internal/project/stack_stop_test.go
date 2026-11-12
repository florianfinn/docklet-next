// Source: docklet-next/issues/316
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original stop/down effect barrier, full-service selection and failure status proof.

package project

import (
	"context"
	"errors"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestStackStopAndDownWriteStatusOnlyAfterSuccess(t *testing.T) {
	for _, action := range []string{"stop", "down"} {
		for _, fail := range []bool{false, true} {
			t.Run(action+map[bool]string{false: "-success", true: "-failure"}[fail], func(t *testing.T) {
				svc, root := runtimeIdentityFixture(t)
				row := runtimeIdentityProject(t, svc, root, "example-project", "alpha", "services:\n  app:\n    image: registry.example.com/app:1\n  worker:\n    image: registry.example.com/worker:1\n")
				require.NoError(t, svc.db.Model(row).Update("status", ProjectStatusRunning).Error)
				server := newProjectRuntimeDockerServer(t, nil)
				svc.dockerService = &docker.DockerClientService{Client: newTestDockerClient(t, server)}
				svc.initChildren(nil, nil)
				oldStop, oldDown := composeStopProjectServices, composeDownProject
				t.Cleanup(func() { composeStopProjectServices = oldStop; composeDownProject = oldDown })
				entered, release := make(chan struct{}), make(chan struct{})
				unblock := sync.OnceFunc(func() { close(release) })
				defer unblock()
				effect := func() error {
					close(entered)
					<-release
					if fail {
						return errors.New("synthetic Compose failure")
					}
					return nil
				}
				var selected []string
				composeStopProjectServices = func(_ context.Context, model *composetypes.Project, services []string) error {
					selected = services
					return effect()
				}
				composeDownProject = func(context.Context, *composetypes.Project, bool) error { return effect() }
				done := make(chan error, 1)
				joined := make(chan struct{})
				defer func() { unblock(); <-joined }()
				go func() {
					defer close(joined)
					if action == "stop" {
						done <- svc.StopProject(t.Context(), row.ID, usertypes.Actor{})
					} else {
						done <- svc.DownProject(t.Context(), row.ID, usertypes.Actor{})
					}
				}()
				<-entered
				var observed Project
				require.NoError(t, svc.db.First(&observed, "id=?", row.ID).Error)
				require.Equal(t, ProjectStatusRunning, observed.Status)
				unblock()
				err := <-done
				if fail {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				require.NoError(t, svc.db.First(&observed, "id=?", row.ID).Error)
				if fail {
					require.Equal(t, ProjectStatusRunning, observed.Status)
				} else {
					require.Equal(t, ProjectStatusStopped, observed.Status)
				}
				if action == "stop" {
					require.Equal(t, []string{"app", "worker"}, selected)
				}
			})
		}
	}
}
