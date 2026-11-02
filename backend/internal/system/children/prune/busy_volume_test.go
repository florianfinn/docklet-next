// Source: docklet-next/issues/202
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package prune

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	volumeservice "github.com/florianfinn/docklet-next/backend/internal/volume"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScheduledBusyVolumeIsSkippedInUse(t *testing.T) {
	held, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/volumes/busy"):
			close(held)
			<-release
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"removed externally"}`))
		case strings.HasSuffix(r.URL.Path, "/system/df"):
			_, _ = w.Write([]byte(`{"Volumes":[{"Name":"busy","Driver":"local","UsageData":{"RefCount":0,"Size":1}}]}`))
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	defer cli.Close()
	ds := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)
	vs := volumeservice.NewVolumeService(nil, ds, nil, nil, nil, nil, nil, nil, nil, nil)
	done := make(chan error, 1)
	go func() {
		_, err := vs.ExecuteVolumePrune(t.Context(), []volume.PruneCandidate{{Name: "busy", CreatedAt: "2026-11-01T00:00:00Z"}})
		done <- err
	}()
	<-held
	svc := NewService(time.UTC, func(context.Context) (*client.Client, error) { return cli, nil }, nil, nil, vs, nil)
	targets := []scheduler.TargetOutcome{}
	ctx := jobcontext.WithExecution(t.Context(), scheduler.Run{}, func(target scheduler.TargetOutcome) error { targets = append(targets, target); return nil })
	result, started, err := svc.PruneScheduled(ctx, "0", system.PruneAllRequest{Volumes: &system.PruneVolumesOptions{Mode: system.PruneVolumeModeAll}})
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, err)
	require.True(t, started)
	require.True(t, result.Success)
	require.Empty(t, result.Errors)
	require.Empty(t, result.VolumesDeleted)
	found := false
	for _, target := range targets {
		if target.ID == "prune:volume:busy" && target.Status != scheduler.Running {
			found = true
			require.Equal(t, scheduler.Skipped, target.Status)
			require.Equal(t, "in-use", target.Message)
			require.EqualValues(t, "volume.workspace-busy", target.Code)
		}
	}
	require.True(t, found)
}
