// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

func requestFixture(t *testing.T) (string, Request, []byte) {
	t.Helper()
	r, old := replayFixture(t)
	data := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(data, "self-update"), 0750))
	request := Request{OpID: "helper-operation", Role: "hub", OldContainerID: "old-container", Replay: r, TargetReference: "example.com/docklet-hub:1.0.1@sha256:" + digestHex, TargetVersion: "1.0.1", Deadline: time.Now().Add(OperationTimeout), AppUID: os.Geteuid(), AppGID: os.Getegid()}
	require.NoError(t, WriteJSON(RequestPath(data, request.OpID), request))
	return data, request, old
}
func TestHelperFailedHealthKeepsPinWithoutRollback(t *testing.T) {
	data, request, old := requestFixture(t)
	ups := 0
	helper := Helper{Up: func(_ context.Context, p *composetypes.Project, service string) error {
		ups++
		require.Equal(t, request.TargetReference, p.Services[service].Image)
		return nil
	}, Healthy: func(context.Context, Request) error { return errors.New("unhealthy") }}
	require.Error(t, helper.Run(t.Context(), data, request.OpID))
	require.Equal(t, 1, ups, "failed health must not apply the previous image")
	require.False(t, envBytesEqual(request.Replay.EnvFile, old))
	backup, err := os.ReadFile(request.Replay.EnvFile + ".docklet-previous." + request.OpID)
	require.NoError(t, err)
	require.Equal(t, old, backup)
	var result Result
	require.NoError(t, ReadJSON(ResultPath(data, request.OpID), &result))
	require.Equal(t, "failed-health", result.Phase)
	require.Error(t, helper.Run(t.Context(), data, request.OpID))
	require.Equal(t, 1, ups, "an existing operation cannot rerun")
}
func TestHelperPreEffectRestore(t *testing.T) {
	data, request, old := requestFixture(t)
	ups := 0
	helper := Helper{Up: func(context.Context, *composetypes.Project, string) error { ups++; return nil }, Healthy: func(context.Context, Request) error { return nil }, Phase: func(phase string) {
		if phase == "env-written" {
			path := ResultPath(data, request.OpID)
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0750))
		}
	}}
	require.Error(t, helper.Run(t.Context(), data, request.OpID))
	require.Zero(t, ups)
	require.True(t, envBytesEqual(request.Replay.EnvFile, old), "env must be restored before any Compose effect")
	info, err := os.Stat(request.Replay.EnvFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0640), info.Mode().Perm())
}
func TestHelperMountsAndCreateContract(t *testing.T) {
	for _, dataType := range []mount.Type{mount.TypeBind, mount.TypeVolume} {
		t.Run(string(dataType), func(t *testing.T) {
			current := container.InspectResponse{Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: "/host/run/docker.sock", Destination: "/var/run/docker.sock", RW: true}, {Type: dataType, Source: "/host/data", Name: "hub-data", Destination: "/app/data", RW: true}}}
			replay := Replay{WorkingDir: "/opt/example"}
			mounts, err := HelperMounts(current, "unix:///var/run/docker.sock", "/app/data", replay)
			require.NoError(t, err)
			require.Equal(t, "/host/run/docker.sock", mounts[0].Source)
			require.Equal(t, "/var/run/docker.sock", mounts[0].Target)
			require.Equal(t, replay.WorkingDir, mounts[1].Source)
			require.Equal(t, replay.WorkingDir, mounts[1].Target)
			wantData := "/host/data"
			if dataType == mount.TypeVolume {
				wantData = "hub-data"
			}
			require.Equal(t, wantData, mounts[2].Source)
			require.Equal(t, HelperData, mounts[2].Target)
			create := HelperCreate(Request{OpID: "operation-1", TargetReference: "example.com/hub:1.0.1@sha256:" + digestHex}, mounts)
			require.Empty(t, create.Config.Entrypoint)
			require.Equal(t, []string{"self-update", "--op", "operation-1"}, []string(create.Config.Cmd))
			require.Equal(t, []string{"DOCKER_HOST=unix:///var/run/docker.sock"}, create.Config.Env)
			require.Equal(t, []string{"NONE"}, create.Config.Healthcheck.Test)
			require.Equal(t, "self-update", create.Config.Labels["io.docklet.helper"])
			require.NotContains(t, create.Config.Labels, "com.docker.compose.project")
			require.False(t, create.HostConfig.AutoRemove)
			require.Equal(t, "0:0", create.Config.User, "the helper must reach operator-owned project files")
			require.Empty(t, create.HostConfig.GroupAdd)
			_, err = HelperMounts(current, "tcp://example.com:2375", "/app/data", replay)
			require.Error(t, err)
			current.Mounts[0].Type = mount.TypeVolume
			_, err = HelperMounts(current, "unix:///var/run/docker.sock", "/app/data", replay)
			require.Error(t, err)
			current.Mounts = nil
			_, err = HelperMounts(current, "unix:///var/run/docker.sock", "/app/data", replay)
			require.Error(t, err)
		})
	}
}

func TestManualRecoveryUsesRecordedInputs(t *testing.T) {
	replay := Replay{Project: "explicit-project", Service: "hub", ConfigFiles: []string{"/opt/example/compose.yaml", "/opt/example/override.yaml"}, EnvFile: "/opt/example/.env"}
	commands := ManualCommands(replay, &Operation{OpID: "failed-op", Phase: "settled", Outcome: "failed-health"})
	require.Equal(t, []string{
		"cp -- '/opt/example/.env.docklet-previous.failed-op' '/opt/example/.env'",
		"docker compose -p 'explicit-project' -f '/opt/example/compose.yaml' -f '/opt/example/override.yaml' --env-file '/opt/example/.env' pull 'hub'",
		"docker compose -p 'explicit-project' -f '/opt/example/compose.yaml' -f '/opt/example/override.yaml' --env-file '/opt/example/.env' up -d --no-deps 'hub'",
	}, commands)
}
